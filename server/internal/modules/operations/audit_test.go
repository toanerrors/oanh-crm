package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func lastAudit(t *testing.T, f *fixture, action string) string {
	t.Helper()
	var id string
	if err := f.store.Read.QueryRow("SELECT CAST(MAX(id) AS TEXT) FROM audit WHERE action=?", action).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func auditRestore(t *testing.T, f *fixture, id, key string) (string, error) {
	t.Helper()
	ctx := context.Background()
	in := AuditRestoreInput{AuditID: id, Reason: "Khôi phục để kiểm thử"}
	p, err := f.ops.PreviewAuditRestore(ctx, f.cmd("preview-"+key), in)
	if err != nil {
		return "", err
	}
	if len(p.Changes) == 0 {
		t.Fatal("empty restore preview")
	}
	in.ExpectedRevision = &p.Revision
	in.Fingerprint = p.Fingerprint
	b, err := f.ops.RestoreAudit(ctx, f.cmd(key), in)
	if err != nil {
		return "", err
	}
	var out struct {
		AuditID string `json:"auditId"`
	}
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out.AuditID, nil
}
func TestAuditSnapshotsUndoRedoDraftAndReopen(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	original := f.save(t, "audit-draft")
	creation := lastAudit(t, f, "document.save")
	original.Description = "Nội dung sửa"
	original.Lines[0].Debit = "1234567"
	original.Lines[1].Credit = "1234567"
	b, err := f.ledger.Save(ctx, f.cmd("audit-edit"), original)
	edited := decodeDocument(t, b, err)
	event := lastAudit(t, f, "document.save")
	revision, _ := ledger.ReadContext(ctx, f.store.Read)
	in := AuditRestoreInput{AuditID: event, Reason: "Sửa nhầm số tiền"}
	p, err := f.ops.PreviewAuditRestore(ctx, f.cmd("audit-preview"), in)
	if err != nil {
		t.Fatal(err)
	}
	current, err := ledger.ReadDocument(ctx, f.store.Read, original.ID)
	if err != nil || current.Version != edited.Version || current.Lines[0].Debit != "1234567" {
		t.Fatal("preview changed data", err)
	}
	afterPreview, _ := ledger.ReadContext(ctx, f.store.Read)
	if afterPreview.Revision != revision.Revision {
		t.Fatal("preview changed revision")
	}
	raw, _ := json.Marshal(p)
	if !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("preview lost exact amount")
	}
	in.ExpectedRevision = &p.Revision
	in.Fingerprint = p.Fingerprint
	response, err := f.ops.RestoreAudit(ctx, f.cmd("audit-confirm"), in)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := f.ops.RestoreAudit(ctx, f.cmd("audit-confirm"), in)
	if err != nil || string(replay) != string(response) {
		t.Fatal("restore retry duplicated", err)
	}
	var out struct {
		AuditID string `json:"auditId"`
	}
	json.Unmarshal(response, &out)
	got, err := ledger.ReadDocument(ctx, f.store.Read, original.ID)
	if err != nil || got.Description != "Chứng từ kiểm thử" || got.Lines[0].Debit != "9007199254740993" || got.Version != 3 || got.Status != "draft" {
		t.Fatal(got, err)
	}
	redo, err := auditRestore(t, f, out.AuditID, "audit-redo")
	if err != nil {
		t.Fatal(err)
	}
	got, err = ledger.ReadDocument(ctx, f.store.Read, original.ID)
	if err != nil || got.Description != "Nội dung sửa" || got.Version != 4 {
		t.Fatal(got, err)
	}
	if _, err = f.ops.PreviewAuditRestore(ctx, f.cmd("audit-old"), AuditRestoreInput{AuditID: creation, Reason: "Nhật ký cũ"}); err == nil {
		t.Fatal("old edit overwrote later version")
	}
	id, _ := ledger.AuditID(redo)
	detail, err := f.ledger.AuditDetail(ctx, id, 1, 20)
	if err != nil || detail.RestoredFrom != out.AuditID || detail.BaseAction != "document.save" {
		t.Fatal(detail, err)
	}
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
	metadata, _ := ledger.ReadContext(ctx, f.store.Read)
	f.epoch = metadata.Epoch
	detail, err = f.ledger.AuditDetail(ctx, id, 1, 20)
	if err != nil || detail.BaseAction != "document.save" {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, redo, "after-reopen"); err != nil {
		t.Fatal(err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
func TestAuditDraftCreationDiscardRecoveryVersionAndUnchangedLines(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	doc := f.save(t, "discard-audit")
	creation := lastAudit(t, f, "document.save")
	discarded, err := auditRestore(t, f, creation, "discard-created")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ledger.ReadDocument(ctx, f.store.Read, doc.ID); accounting.Code(err) != "NOT_FOUND" {
		t.Fatal(err)
	}
	recovered, err := auditRestore(t, f, discarded, "recover-created")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ledger.ReadDocument(ctx, f.store.Read, doc.ID)
	if err != nil || got.Version != 2 || got.Lines[0].Debit != "9007199254740993" {
		t.Fatal(got, err)
	}
	if _, err = f.ledger.Save(ctx, f.cmd("stale-before-trash"), doc); accounting.Code(err) != "VERSION_CONFLICT" {
		t.Fatal(err)
	}
	got.Description = "Chỉ đổi diễn giải"
	b, err := f.ledger.Save(ctx, f.cmd("desc-only"), got)
	got = decodeDocument(t, b, err)
	if _, err = auditRestore(t, f, lastAudit(t, f, "document.save"), "restore-desc"); err != nil {
		t.Fatal(err)
	}
	final, err := ledger.ReadDocument(ctx, f.store.Read, doc.ID)
	if err != nil || len(final.Lines) != 2 || final.Lines[0].Debit != "9007199254740993" {
		t.Fatal(final, err)
	}
	_ = recovered
}
func TestAuditVoucherCatalogAndOpeningRecovery(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	in := tradeInput("audit-voucher", "sale", "1000")
	in.TradeMode = "service"
	b, err := f.ops.SaveVoucher(ctx, f.cmd("audit-voucher-create"), in)
	if err != nil {
		t.Fatal(err)
	}
	var saved Voucher
	json.Unmarshal(b, &saved)
	in = saved.Input
	in.Description = "Sửa nháp nghiệp vụ"
	in.Lines[0].UnitPrice = "2000"
	b, err = f.ops.SaveVoucher(ctx, f.cmd("audit-voucher-edit"), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, lastAudit(t, f, "voucher.save"), "restore-voucher"); err != nil {
		t.Fatal(err)
	}
	v, err := f.ops.Voucher(ctx, saved.Document.ID)
	if err != nil || v.Input.Lines[0].UnitPrice != "1000" || v.Document.Version != 3 || v.Input.DraftVersion != 3 {
		t.Fatal(v, err)
	}
	var count int
	f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE status='posted'").Scan(&count)
	if count != 0 {
		t.Fatal("restore posted a voucher")
	}
	p := ledger.Partner{ID: "audit-partner", Code: "AUKH", Name: "Đối tượng mới", Role: "customer", Active: true}
	if _, err = f.ledger.SavePartners(ctx, f.cmd("audit-partner"), []ledger.Partner{p}); err != nil {
		t.Fatal(err)
	}
	deactivated, err := auditRestore(t, f, lastAudit(t, f, "partner.save"), "deactivate-new-partner")
	if err != nil {
		t.Fatal(err)
	}
	var active bool
	f.store.Read.QueryRow("SELECT active FROM partners WHERE id=?", p.ID).Scan(&active)
	if active {
		t.Fatal("catalog creation was not deactivated")
	}
	if _, err = auditRestore(t, f, deactivated, "reactivate-partner"); err != nil {
		t.Fatal(err)
	}
	f.store.Read.QueryRow("SELECT active FROM partners WHERE id=?", p.ID).Scan(&active)
	if !active {
		t.Fatal("catalog not reactivated")
	}
	opening := openingInput()
	b, err = f.ops.SaveOpening(ctx, f.cmd("audit-opening"), opening)
	state := decodeOpening(t, b, err)
	openingEvent := lastAudit(t, f, "admin.opening.save")
	discarded, err := auditRestore(t, f, openingEvent, "discard-opening")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, discarded, "recover-opening"); err != nil {
		t.Fatal(err)
	}
	got, err := ledger.ReadDocument(ctx, f.store.Read, state.Document.ID)
	if err != nil || got.Version != 2 {
		t.Fatal(got, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
func TestAuditRollbackRacePermissionsAndPostedGuard(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	doc := f.save(t, "protected-audit")
	event := lastAudit(t, f, "document.save")
	in := AuditRestoreInput{AuditID: event, Reason: "Kiểm thử đồng thời"}
	p, err := f.ops.PreviewAuditRestore(ctx, f.cmd("guard-preview"), in)
	if err != nil {
		t.Fatal(err)
	}
	in.ExpectedRevision = &p.Revision
	in.Fingerprint = p.Fingerprint
	if _, err = f.store.Write.Exec("CREATE TRIGGER fail_restore_audit BEFORE INSERT ON audit WHEN NEW.action='admin.audit.restore' BEGIN SELECT RAISE(ABORT,'synthetic late failure');END"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ops.RestoreAudit(ctx, f.cmd("rollback-restore"), in); err == nil {
		t.Fatal("expected rollback")
	}
	if _, err = ledger.ReadDocument(ctx, f.store.Read, doc.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	f.store.Read.QueryRow("SELECT count(*) FROM audit_context").Scan(&count)
	if count != 0 {
		t.Fatal("audit context survived rollback")
	}
	f.store.Write.Exec("DROP TRIGGER fail_restore_audit")
	var workers sync.WaitGroup
	success := make(chan string, 2)
	for i := 0; i < 2; i++ {
		workers.Go(func() {
			if _, err := f.ops.RestoreAudit(ctx, f.cmd(fmt.Sprintf("race-restore-%d", i)), in); err == nil {
				success <- "ok"
			}
		})
	}
	workers.Wait()
	close(success)
	if len(success) != 1 {
		t.Fatal("concurrent restore count", len(success))
	}
	latest := lastAudit(t, f, "admin.audit.restore")
	if _, err = auditRestore(t, f, latest, "put-back-draft"); err != nil {
		t.Fatal(err)
	}
	current, _ := ledger.ReadDocument(ctx, f.store.Read, doc.ID)
	if _, err = f.ledger.Post(ctx, f.cmd("post-guarded"), ledger.Post{ID: doc.ID, Version: current.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ops.PreviewAuditRestore(ctx, f.cmd("posted-guard"), AuditRestoreInput{AuditID: lastAudit(t, f, "admin.audit.restore"), Reason: "Không sửa sổ"}); err == nil {
		t.Fatal("posted draft restore allowed")
	}
	denied := f.cmd("viewer-restore")
	denied.Actor = "missing-user"
	if _, err = f.ops.PreviewAuditRestore(ctx, denied, in); accounting.Code(err) != "FORBIDDEN" {
		t.Fatal(err)
	}
	denied = f.cmd("stale-epoch")
	denied.Epoch = "old"
	if _, err = f.ops.PreviewAuditRestore(ctx, denied, in); accounting.Code(err) != "DATASET_CHANGED" {
		t.Fatal(err)
	}
	for _, q := range []string{"DELETE FROM audit", "UPDATE audit SET action='changed'", "DELETE FROM audit_changes", "UPDATE audit_details SET reason='changed'"} {
		if _, err = f.store.Write.Exec(q); err == nil {
			t.Fatal("audit changed", q)
		}
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestAuditCatalogCreationUndoRedoAndAccountParent(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	cases := []struct {
		action, table string
		save          func() error
	}{
		{"bank.save", "bank_accounts", func() error {
			_, err := f.ops.SaveBank(ctx, f.cmd("audit-bank"), BankAccount{ID: "audit-bank", Number: "0001123", BankName: "Ngân hàng kiểm thử", AccountID: "112", Active: true})
			return err
		}},
		{"warehouse.save", "warehouses", func() error {
			_, err := f.ops.SaveWarehouse(ctx, f.cmd("audit-warehouse"), Warehouse{ID: "audit-warehouse", Code: "AUDWH", Name: "Kho kiểm thử", Active: true})
			return err
		}},
		{"item.save", "items", func() error {
			_, err := f.ops.SaveItem(ctx, f.cmd("audit-item"), Item{ID: "audit-item", Code: "AUDITEM", Name: "Dịch vụ kiểm thử", Unit: "Lần", Active: true})
			return err
		}},
		{"admin.employee.save", "employees", func() error {
			_, err := f.ops.SaveEmployee(ctx, f.cmd("audit-employee"), Employee{ID: "audit-employee", Code: "AUDNV", Name: "Nhân viên kiểm thử", BaseSalary: "9007199254740993", InsuranceBase: "5000000", Active: true})
			return err
		}},
	}
	for i, c := range cases {
		t.Run(c.table, func(t *testing.T) {
			if err := c.save(); err != nil {
				t.Fatal(err)
			}
			undo, err := auditRestore(t, f, lastAudit(t, f, c.action), fmt.Sprintf("undo-catalog-%d", i))
			if err != nil {
				t.Fatal(err)
			}
			var active int
			if err = f.store.Read.QueryRow("SELECT active FROM " + c.table + " WHERE id LIKE 'audit-%'").Scan(&active); err != nil || active != 0 {
				t.Fatal(active, err)
			}
			if _, err = auditRestore(t, f, undo, fmt.Sprintf("redo-catalog-%d", i)); err != nil {
				t.Fatal(err)
			}
			if err = f.store.Read.QueryRow("SELECT active FROM " + c.table + " WHERE id LIKE 'audit-%'").Scan(&active); err != nil || active != 1 {
				t.Fatal(active, err)
			}
		})
	}
	var salary string
	if err := f.store.Read.QueryRow("SELECT CAST(base_salary AS TEXT) FROM employees WHERE id='audit-employee'").Scan(&salary); err != nil || salary != "9007199254740993" {
		t.Fatal(salary, err)
	}
	parent := "111"
	a := ledger.Account{ID: "audit-child", Code: "1119", Name: "Tiền chi tiết", Group: "cash", Active: true, ParentID: &parent}
	if _, err := f.ledger.SaveAccount(ctx, f.cmd("account-create"), a); err != nil {
		t.Fatal(err)
	}
	var rev int64
	if err := f.store.Read.QueryRow("SELECT MAX(revision) FROM account_hierarchy WHERE account_id=?", a.ID).Scan(&rev); err != nil {
		t.Fatal(err)
	}
	empty := ""
	a.ParentID = &empty
	a.ParentRevision = rev
	if _, err := f.ledger.SaveAccount(ctx, f.cmd("account-unparent"), a); err != nil {
		t.Fatal(err)
	}
	if _, err := auditRestore(t, f, lastAudit(t, f, "admin.account.save"), "restore-parent"); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := f.store.Read.QueryRow("SELECT parent_id FROM account_hierarchy WHERE account_id=? ORDER BY revision DESC LIMIT 1", a.ID).Scan(&got); err != nil || got != parent {
		t.Fatal(got, err)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestAuditFinancialDraftAndOpeningAdjustmentRecovery(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	in := ledger.FinancialInput{ID: "audit-report", Template: "tt99-2026-v1", From: "2026-01-01", To: "2026-12-31", PreviousFrom: "2025-01-01", PreviousTo: "2025-12-31", PreparedDate: "2026-10-23", Sheets: []string{"B01-DN"}, Values: map[string]map[string]ledger.FinancialValue{}, Notes: "Ghi chú gốc"}
	if _, err := f.ledger.SaveFinancial(ctx, f.cmd("report-create"), in); err != nil {
		t.Fatal(err)
	}
	creation := lastAudit(t, f, "financial.save")
	undo, err := auditRestore(t, f, creation, "report-discard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, undo, "report-recover"); err != nil {
		t.Fatal(err)
	}
	reports, err := f.ledger.FinancialReports(ctx, "", in.ID)
	if err != nil || len(reports) != 1 || reports[0].Input.Version != 2 {
		t.Fatal(reports, err)
	}
	if _, err = f.ledger.SaveFinancial(ctx, f.cmd("stale-report"), in); accounting.Code(err) != "VERSION_CONFLICT" {
		t.Fatal(err)
	}
	in = reports[0].Input
	in.Notes = "Sửa ghi chú"
	if _, err = f.ledger.SaveFinancial(ctx, f.cmd("report-edit"), in); err != nil {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, lastAudit(t, f, "financial.save"), "report-undo-edit"); err != nil {
		t.Fatal(err)
	}
	reports, err = f.ledger.FinancialReports(ctx, "", in.ID)
	if err != nil || reports[0].Input.Notes != "Ghi chú gốc" || reports[0].Input.Version != 4 {
		t.Fatal(reports, err)
	}
	state := confirmInitial(t, f, openingInput())
	adj := adjustmentInput(state, "audit-adjustment", "2000000")
	saveAdjustment(t, f, "adjust-create", adj)
	undo, err = auditRestore(t, f, lastAudit(t, f, "admin.opening.adjustment.save"), "adjust-discard")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auditRestore(t, f, undo, "adjust-recover"); err != nil {
		t.Fatal(err)
	}
	restored, err := f.ops.OpeningAdjustment(ctx, adj.Input.ID)
	if err != nil || restored.Document.Version != 2 {
		t.Fatal(restored, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestAuditRestoreRequiresUnchangedPreviewAndReason(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	doc := f.save(t, "preview-guard")
	event := lastAudit(t, f, "document.save")
	input := AuditRestoreInput{AuditID: event, Reason: "Xem trước có kiểm soát"}
	preview, err := f.ops.PreviewAuditRestore(ctx, f.cmd("guard-preview"), input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedRevision = &preview.Revision
	input.Fingerprint = preview.Fingerprint
	changed := input
	changed.Reason = "Lý do khác"
	if _, err = f.ops.RestoreAudit(ctx, f.cmd("reason-changed"), changed); accounting.Code(err) != "PREVIEW_REQUIRED" {
		t.Fatal(err)
	}
	changed = input
	changed.Fingerprint = "tampered"
	if _, err = f.ops.RestoreAudit(ctx, f.cmd("fingerprint-changed"), changed); accounting.Code(err) != "PREVIEW_REQUIRED" {
		t.Fatal(err)
	}
	if _, err = f.ledger.SavePartners(ctx, f.cmd("unrelated-later"), []ledger.Partner{{ID: "unrelated", Code: "OTHER", Name: "Thay đổi sau xem trước", Role: "customer", Active: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ops.RestoreAudit(ctx, f.cmd("revision-changed"), input); accounting.Code(err) != "PREVIEW_REQUIRED" {
		t.Fatal(err)
	}
	current, err := ledger.ReadDocument(ctx, f.store.Read, doc.ID)
	if err != nil || current.Version != doc.Version {
		t.Fatal(current, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
