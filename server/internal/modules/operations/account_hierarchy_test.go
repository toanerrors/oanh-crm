package operations

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
	"sync"
	"testing"
)

func accountParent(value string) *string { return &value }
func saveHierarchyAccount(t *testing.T, f *fixture, key string, a ledger.Account) ledger.Account {
	t.Helper()
	raw, err := f.ledger.SaveAccount(context.Background(), f.cmd(key), a)
	if err != nil {
		t.Fatal(err)
	}
	var result ledger.Account
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestAccountHierarchyExplicitParentsCyclesVersionsAndHistory(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	w, err := f.ledger.Snapshot(ctx, f.actor, "owner", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	byCode := map[string]ledger.Account{}
	for _, a := range w.Accounts {
		byCode[a.Code] = a
	}
	for child, parent := range map[string]string{"1121": "112", "1281": "128", "1331": "133"} {
		if byCode[child].ParentID == nil || *byCode[child].ParentID != byCode[parent].ID {
			t.Fatal(child, byCode[child])
		}
	}
	a := saveHierarchyAccount(t, f, "tree-a", ledger.Account{ID: "tree-a", Code: "CUSTOM-A", Name: "Tiền chi tiết", Group: "cash", Active: true, ParentID: accountParent("111")})
	b := saveHierarchyAccount(t, f, "tree-b", ledger.Account{ID: "tree-b", Code: "OTHER-B", Name: "Chi tiết cấp ba", Group: "cash", Active: true, ParentID: accountParent(a.ID)})
	for _, parent := range []string{a.ID, b.ID, "missing"} {
		changed := a
		changed.ParentID = accountParent(parent)
		if _, err = f.ledger.SaveAccount(ctx, f.cmd("invalid-"+parent), changed); accounting.Code(err) != "INVALID_PARENT" {
			t.Fatal(parent, err)
		}
	}
	changed := a
	changed.ParentID = accountParent("112")
	current := saveHierarchyAccount(t, f, "move-a", changed)
	changed.ParentID = accountParent("128")
	if _, err = f.ledger.SaveAccount(ctx, f.cmd("stale-parent"), changed); accounting.Code(err) != "VERSION_CONFLICT" {
		t.Fatal(err)
	}
	legacy := current
	legacy.ParentID = nil
	legacy.Name = "Tên chưa có lịch sử"
	legacy = saveHierarchyAccount(t, f, "legacy-account-update", legacy)
	if legacy.ParentID == nil || *legacy.ParentID != "112" {
		t.Fatal("legacy API cleared parent")
	}
	var history int
	if err = f.store.Read.QueryRow("SELECT count(*) FROM account_hierarchy WHERE account_id=?", a.ID).Scan(&history); err != nil || history != 2 {
		t.Fatal(history, err)
	}
	raw, err := f.ledger.SaveAccount(ctx, f.cmd("legacy-account-update"), func() ledger.Account {
		in := current
		in.ParentID = nil
		in.Name = "Tên chưa có lịch sử"
		return in
	}())
	if err != nil || len(raw) == 0 {
		t.Fatal(err)
	}
	if err = f.store.Read.QueryRow("SELECT count(*) FROM account_hierarchy WHERE account_id=?", a.ID).Scan(&history); err != nil || history != 2 {
		t.Fatal(history, err)
	}
	doc := ledger.Document{ID: "child-history", Type: "journal", Date: "2026-10-22", Description: "Kiểm thử tài khoản con", SourceType: "journal", SourceID: "child-history", Lines: []ledger.Line{{AccountID: b.ID, Debit: "9007199254740993", Credit: "0"}, {AccountID: "411", Debit: "0", Credit: "9007199254740993"}}}
	saved, err := f.ledger.Save(ctx, f.cmd("tree-doc"), doc)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(saved, &doc)
	if _, err = f.ledger.Post(ctx, f.cmd("tree-post"), ledger.Post{ID: doc.ID, Version: doc.Version}); err != nil {
		t.Fatal(err)
	}
	legacy.ParentID = accountParent("128")
	if _, err = f.ledger.SaveAccount(ctx, f.cmd("move-used-branch"), legacy); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
	if _, err = f.store.Write.Exec("INSERT INTO account_hierarchy VALUES(?,999,?)", a.ID, b.ID); err == nil {
		t.Fatal("SQL accepted cycle")
	}
	if _, err = f.store.Write.Exec("UPDATE account_hierarchy SET parent_id=NULL WHERE account_id=?", b.ID); err == nil {
		t.Fatal("SQL changed history")
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
func TestAccountHierarchyBackupRestoreReopen(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	a := saveHierarchyAccount(t, f, "backup-tree", ledger.Account{ID: "backup-tree", Code: "CUSTOM-BACKUP", Name: "Chi tiết", Group: "cash", Active: true, ParentID: accountParent("111")})
	archive, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := f.store.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Activate(ctx, stage.ID); err != nil {
		t.Fatal(err)
	}
	root := f.root
	f.store.Close()
	store, err := sqlite.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	f.store = store
	f.ledger.Store = store
	w, err := f.ledger.Snapshot(ctx, f.actor, "owner", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range w.Accounts {
		if item.ID == a.ID {
			found = true
			if item.ParentID == nil || *item.ParentID != "111" || item.ParentRevision != a.ParentRevision {
				t.Fatal(item)
			}
		}
	}
	if !found {
		t.Fatal("missing restored child")
	}
	if err = sqlite.Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
}
func TestExcelAccountParentMappingAndUnmappedPreservation(t *testing.T) {
	f := openFixture(t)
	b, c := stageExcelTest(t, f, "parent-import", "accounts", []string{"code", "name", "group", "requiresPartner", "parentCode"}, [][]string{{"CUSTOM-EXCEL", "Con từ Excel", "cash", "false", "111"}})
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 {
		t.Fatal(p)
	}
	excelCommitTest(t, f, b, c, p, "parent-import")
	b, c = stageExcelTest(t, f, "parent-update", "accounts", []string{"code", "name", "group", "requiresPartner"}, [][]string{{"CUSTOM-EXCEL", "Con từ Excel", "cash", "false"}})
	c.Mode = "update"
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 {
		t.Fatal(p)
	}
	excelCommitTest(t, f, b, c, p, "parent-update")
	var parent string
	if err := f.store.Read.QueryRow("SELECT h.parent_id FROM account_current_parents h JOIN accounts a ON a.id=h.account_id WHERE a.code='CUSTOM-EXCEL'").Scan(&parent); err != nil || parent != "111" {
		t.Fatal(parent, err)
	}
}

func TestAccountHierarchyConcurrentReparentAndDepthLimit(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	a := saveHierarchyAccount(t, f, "concurrent-child", ledger.Account{ID: "concurrent-child", Code: "TREE-C", Name: "Con", Group: "cash", Active: true, ParentID: accountParent("111")})
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	for _, parent := range []string{"112", "128"} {
		wg.Add(1)
		go func(parent string) {
			defer wg.Done()
			copy := a
			copy.ParentID = accountParent(parent)
			_, err := f.ledger.SaveAccount(ctx, f.cmd("move-"+parent), copy)
			errors <- err
		}(parent)
	}
	wg.Wait()
	close(errors)
	success, conflict := 0, 0
	for err := range errors {
		if err == nil {
			success++
		} else if accounting.Code(err) == "VERSION_CONFLICT" {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	parent := "111"
	for depth := 2; depth <= 11; depth++ {
		id := fmt.Sprintf("depth-%d", depth)
		input := ledger.Account{ID: id, Code: id, Name: "Chi tiết", Group: "cash", Active: true, ParentID: accountParent(parent)}
		_, err := f.ledger.SaveAccount(ctx, f.cmd(id), input)
		if depth <= 10 && err != nil {
			t.Fatal(depth, err)
		}
		if depth == 11 && accounting.Code(err) != "INVALID_PARENT" {
			t.Fatal(depth, err)
		}
		parent = id
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestAccountHierarchyLegacyIdempotencyPayload(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	a := ledger.Account{ID: "legacy-idempotency", Code: "LEGACY-IDEM", Name: "Tài khoản cũ", Group: "cash", Active: true}
	old := struct {
		ID              string `json:"id"`
		Code            string `json:"code"`
		Name            string `json:"name"`
		Group           string `json:"group"`
		RequiresPartner bool   `json:"requiresPartner"`
		Active          bool   `json:"active"`
	}{a.ID, a.Code, a.Name, a.Group, a.RequiresPartner, a.Active}
	raw, err := f.ledger.Write(ctx, f.cmd("old-account-key"), "admin.account.save", old, func(tx *sql.Tx) (any, error) { return f.ledger.SaveAccountTx(ctx, tx, a) })
	if err != nil {
		t.Fatal(err)
	}
	retry, err := f.ledger.SaveAccount(ctx, f.cmd("old-account-key"), a)
	if err != nil || !bytes.Equal(raw, retry) {
		t.Fatal("legacy retry changed", err)
	}
}
