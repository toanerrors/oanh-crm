package operations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestVoucherDraftPreviewPostingAndBackup(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	in := tradeInput("voucher-source", "sale", "900719925474099.3")
	in.Lines[0].Quantity = "10"
	in.TradeMode = "service"
	in.PaymentAccount = "1121"
	in.Voucher = VoucherInfo{DocumentDate: "2026-09-30", Contact: "Người kiểm thử", Address: "Địa chỉ giả lập", Notes: "Ghi chú"}
	b, err := f.ops.SaveVoucher(ctx, f.cmd("save-voucher"), in)
	if err != nil {
		t.Fatal(err)
	}
	var saved Voucher
	if err = json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Document.Status != "draft" || saved.Document.Version != 1 {
		t.Fatal(saved)
	}
	same, err := f.ops.SaveVoucher(ctx, f.cmd("save-voucher"), in)
	if err != nil || string(b) != string(same) {
		t.Fatal(err)
	}
	for _, table := range []string{"operations", "counters"} {
		var count int
		if err = f.store.Read.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	var posted int
	f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE status='posted'").Scan(&posted)
	if posted != 0 {
		t.Fatal("draft affected ledger")
	}
	if _, err = f.ledger.Post(ctx, f.cmd("bypass-post"), ledger.Post{ID: saved.Document.ID, Version: 1}); accounting.Code(err) != "SUBLEDGER_REQUIRED" {
		t.Fatal(err)
	}
	changed := saved.Document
	changed.PolicyID = ""
	changed.Lines[0].Debit = "1"
	if _, err = f.ledger.Save(ctx, f.cmd("bypass-edit"), changed); accounting.Code(err) != "SUBLEDGER_REQUIRED" {
		t.Fatal(err)
	}
	archive, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := f.store.Stage(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.store = db
	f.ledger.Store = db
	t.Cleanup(func() { db.Close() })
	loaded, err := f.ops.Voucher(ctx, saved.Document.ID)
	if err != nil || loaded.Input.Voucher.Contact != "Người kiểm thử" {
		t.Fatal(loaded, err)
	}
	preview, err := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, loaded.Input)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Input.ExpectedRevision = &preview.Revision
	loaded.Input.ExpectedPolicyID = preview.PolicyID
	result := executeTreasury(t, f, "post-voucher", loaded.Input)
	if result.Document.ID != saved.Document.ID || result.Document.Reference != "BH-2026-000001" || result.PaymentDocument == nil || result.PaymentDocument.Reference != "PT-2026-000001" {
		t.Fatal(result)
	}
	list, err := f.ops.Invoices(ctx, "2026-10-01", "sale", "customer", 1, 10)
	if err != nil || list.Remaining != "0" || list.Paid != "9907919180215092" {
		t.Fatal(list, err)
	}
	if _, err = f.store.Write.Exec("UPDATE voucher_inputs SET payload='{}' WHERE document_id=?", saved.Document.ID); err == nil {
		t.Fatal("posted input changed")
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Activate(ctx, stage.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err = f.ops.Voucher(ctx, saved.Document.ID)
	if err != nil || loaded.Document.Status != "draft" {
		t.Fatal(loaded, err)
	}
}

func TestVoucherStockDraftAndImmediatePaymentRollback(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	if _, err := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng hóa", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	in := tradeInput("stock-draft", "purchase", "1000")
	in.TradeMode = "goods"
	in.Lines[0].ItemID = "goods"
	in.PaymentAccount = "1121"
	if _, err := f.ops.SaveVoucher(ctx, f.cmd("draft-stock"), in); err != nil {
		t.Fatal(err)
	}
	var count int
	f.store.Read.QueryRow("SELECT count(*) FROM stock_movements").Scan(&count)
	if count != 0 {
		t.Fatal("draft moved stock")
	}
	if _, err := f.ledger.SaveAccount(ctx, f.cmd("disable-bank"), ledger.Account{ID: "1121", Code: "1121", Name: "Tiền gửi ngân hàng", Group: "cash", Active: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ops.Execute(ctx, f.cmd("failed-payment"), in); accounting.Code(err) != "DUPLICATE_SOURCE" {
		t.Fatal("saved source must remain reserved", err)
	}
	in.SourceID = "new-purchase"
	if _, err := f.ops.Execute(ctx, f.cmd("failed-payment-2"), in); accounting.Code(err) != "INVALID_ACCOUNT" {
		t.Fatal(err)
	}
	for _, table := range []string{"stock_movements", "stock_lots", "operations", "counters"} {
		f.store.Read.QueryRow("SELECT count(*) FROM " + table).Scan(&count)
		if count != 0 {
			t.Fatal("rollback", table, count)
		}
	}
}

func TestTreasuryMultipleRowsExactTotalAndVersion(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	in := Input{Kind: "payment", Date: "2026-10-01", SourceID: "multi-row", Description: "Chi nhiều dòng", MoneyAccount: "111", Amount: "9007199254741093", TreasuryLines: []TreasuryLine{{CounterAccount: "6427", Description: "Dịch vụ", Amount: "9007199254740993"}, {CounterAccount: "6428", Description: "Chi khác", Amount: "100"}}}
	b, err := f.ops.SaveVoucher(ctx, f.cmd("rows-save"), in)
	if err != nil {
		t.Fatal(err)
	}
	var v Voucher
	json.Unmarshal(b, &v)
	old := v.Input
	v.Input.Description = "Đã sửa"
	b, err = f.ops.SaveVoucher(ctx, f.cmd("rows-update"), v.Input)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(b, &v)
	if _, err = f.ops.SaveVoucher(ctx, f.cmd("stale"), old); accounting.Code(err) != "VERSION_CONFLICT" {
		t.Fatal(err)
	}
	v.Input.Amount = "1"
	if _, err = f.ops.Execute(ctx, f.cmd("bad-sum"), v.Input); accounting.Code(err) != "AMOUNT_MISMATCH" {
		t.Fatal(err)
	}
	v.Input.Amount = in.Amount
	result := executeTreasury(t, f, "rows-post", v.Input)
	if len(result.Document.Lines) != 4 || result.Amounts["total"] != in.Amount {
		t.Fatal(result)
	}
}

func TestReferenceChartRetainsExistingAccounts(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	chart := accounting.ReferenceChart()
	if len(chart.Accounts) != 184 {
		t.Fatal(len(chart.Accounts))
	}
	byCode := map[string]accounting.ChartAccount{}
	for _, a := range chart.Accounts {
		if _, ok := byCode[a.Code]; ok {
			t.Fatal("duplicate", a.Code)
		}
		if a.ParentCode != "" {
			if _, ok := byCode[a.ParentCode]; !ok {
				t.Fatal("missing parent", a)
			}
		}
		byCode[a.Code] = a
	}
	if byCode["3521"].ParentCode != "352" || byCode["511"].Level != 1 || byCode["215121"].ParentCode != "21512" {
		t.Fatal("normalized hierarchy")
	}
	old := f.save(t, "history")
	if _, err := f.ledger.ApplyReferenceChart(ctx, f.cmd("chart"), chart.Version); err != nil {
		t.Fatal(err)
	}
	var name string
	f.store.Read.QueryRow("SELECT name FROM accounts WHERE id='111'").Scan(&name)
	if name != "Tiền mặt" {
		t.Fatal(name)
	}
	read, err := ledger.ReadDocument(ctx, f.store.Read, old.ID)
	if err != nil || !strings.Contains(read.Lines[0].AccountName, "Tiền mặt") {
		t.Fatal(read, err)
	}
}

func TestReferenceProductionCostsDoNotCloseToProfit(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	doc := ledger.Document{ID: "production", Type: "journal", Date: "2026-10-01", Description: "Chi phí sản xuất chờ kết chuyển", Lines: []ledger.Line{{AccountID: "621", Debit: "100", Credit: "0"}, {AccountID: "6277", Debit: "50", Credit: "0"}, {AccountID: "111", Debit: "0", Credit: "150"}}}
	b, err := f.ledger.Save(ctx, f.cmd("production-save"), doc)
	d := decodeDocument(t, b, err)
	if _, err = f.ledger.Post(ctx, f.cmd("production-post"), ledger.Post{ID: d.ID, Version: d.Version}); err != nil {
		t.Fatal(err)
	}
	executeTreasury(t, f, "closing-sale", tradeInput("closing-sale", "sale", "1000"))
	close := executeTreasury(t, f, "closing-test", Input{Kind: "closing", SourceID: "closing-test", Date: "2026-10-31", Description: "Kết chuyển giả lập"})
	if close.Amounts["result"] != "1000" {
		t.Fatal(close.Amounts)
	}
	for _, line := range close.Document.Lines {
		if line.AccountID == "621" || line.AccountID == "6277" {
			t.Fatal("production cost closed without allocation", line)
		}
	}
}
