package operations

import (
	"context"
	"encoding/json"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestProjectRevenueExpenseWIPCostingAuditAndReopen(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	if _, err := f.ops.SaveItem(ctx, f.cmd("goods-item"), Item{ID: "item", Code: "VT", Name: "Vật tư công trình", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}

	projects := []ledger.Project{
		{ID: "ct-1", Code: " ct001 ", Name: " Công trình Nhà máy A ", Description: " Giai đoạn 1 ", Active: true},
		{ID: "ct-2", Code: "CT002", Name: "Công trình Văn phòng B", Description: "Cho thuê nhân sự", Active: true},
		{ID: "ct-inactive", Code: "CT-OLD", Name: "Công trình đã đóng", Active: false},
	}
	if _, err := f.ledger.SaveProjects(ctx, f.cmd("projects-create"), projects); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ledger.SaveProjects(ctx, f.cmd("projects-dup"), []ledger.Project{{ID: "other", Code: "ct001", Name: "Trùng mã", Active: true}}); accounting.Code(err) != "DUPLICATE_CODE" {
		t.Fatalf("expected DUPLICATE_CODE, got %v", err)
	}

	// Inactive project must be rejected on purchase/sale/payment/costing.
	badPurchase := tradeInput("bad-proj", "purchase", "1000")
	badPurchase.TradeMode = "service"
	badPurchase.Lines[0].ProjectID = "ct-inactive"
	if _, err := f.ops.Execute(ctx, f.cmd("bad-proj"), badPurchase); accounting.Code(err) != "INVALID_PROJECT" {
		t.Fatalf("expected INVALID_PROJECT, got %v", err)
	}

	// Audit undo/redo on project.save.
	if _, err := f.ledger.SaveProjects(ctx, f.cmd("project-edit"), []ledger.Project{{ID: "ct-2", Code: "CT002", Name: "Tên tạm sửa", Description: "Sửa thử", Active: true}}); err != nil {
		t.Fatal(err)
	}
	undoID, err := auditRestore(t, f, lastAudit(t, f, "project.save"), "project-undo")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := f.ledger.Snapshot(ctx, f.actor, "owner", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range snap.Projects {
		if p.ID == "ct-2" && (p.Name != "Công trình Văn phòng B" || p.Description != "Cho thuê nhân sự") {
			t.Fatalf("unexpected undone project: %+v", p)
		}
	}
	if _, err = auditRestore(t, f, undoID, "project-redo"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ledger.SaveProjects(ctx, f.cmd("project-restore-name"), []ledger.Project{{ID: "ct-2", Code: "CT002", Name: "Công trình Văn phòng B", Description: "Cho thuê nhân sự", Active: true}}); err != nil {
		t.Fatal(err)
	}

	// 1. Purchase service > 2^53 into 642 and WIP into 154 for ct-1; also WIP into 154 for ct-2.
	deductible := true
	if _, err = f.ops.Execute(ctx, f.cmd("ct1-exp"), Input{
		Kind: "purchase", SourceID: "ct1-exp", Date: "2026-10-05", Description: "Thuê nhân công ngoài CT001", PartnerID: "supplier", TradeMode: "service", PriceMode: "exclusive",
		Lines: []TradeLine{
			{ItemID: "service", ProjectID: "ct-1", AccountID: "642", Quantity: "100", UnitPrice: "90071992547410", VATRate: "0", Deductible: &deductible},
			{ItemID: "service", ProjectID: "ct-1", AccountID: "154", Quantity: "1", UnitPrice: "5000000", VATRate: "10", Deductible: &deductible},
			{ItemID: "service", ProjectID: "ct-2", AccountID: "154", Quantity: "1", UnitPrice: "4000000", VATRate: "0", Deductible: &deductible},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Direct cash payment expense for ct-1 via TreasuryLines.
	if _, err = f.ops.Execute(ctx, f.cmd("ct1-cash-seed"), Input{
		Kind: "receipt", SourceID: "ct1-cash-seed", Date: "2026-10-01", Description: "Thu tiền mặt khởi tạo", MoneyAccount: "111", CounterAccount: "711", Amount: "10000000",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ops.Execute(ctx, f.cmd("ct1-payment"), Input{
		Kind: "payment", SourceID: "ct1-payment", Date: "2026-10-06", Description: "Chi công tác phí CT001", MoneyAccount: "111", Amount: "1200000",
		TreasuryLines: []TreasuryLine{{CounterAccount: "642", ProjectID: "ct-1", Description: "Phụ cấp công trình", Amount: "1200000"}},
	}); err != nil {
		t.Fatal(err)
	}

	// 3. Sale service for ct-1 (> 2^53).
	if _, err = f.ops.Execute(ctx, f.cmd("ct1-sale"), Input{
		Kind: "sale", SourceID: "ct1-sale", Date: "2026-10-10", Description: "Doanh thu cho thuê lao động CT001", PartnerID: "customer", TradeMode: "service", PriceMode: "exclusive",
		Lines: []TradeLine{{ItemID: "service", ProjectID: "ct-1", AccountID: "511", Quantity: "100", UnitPrice: "90071992647410", VATRate: "0"}},
	}); err != nil {
		t.Fatal(err)
	}

	// 4. Stocked goods purchase -> sale on ct-2 attaches both 511 revenue and 632 COGS to ct-2.
	if _, err = f.ops.Execute(ctx, f.cmd("goods-in"), Input{
		Kind: "purchase", SourceID: "goods-in", Date: "2026-10-07", Description: "Nhập vật tư kho", PartnerID: "supplier", TradeMode: "goods", PriceMode: "exclusive",
		Lines: []TradeLine{{ItemID: "item", WarehouseID: "default", AccountID: "156", Quantity: "2", UnitPrice: "1500000", VATRate: "0", Deductible: &deductible}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.ops.Execute(ctx, f.cmd("ct2-goods-sale"), Input{
		Kind: "sale", SourceID: "ct2-goods-sale", Date: "2026-10-12", Description: "Xuất bán vật tư CT002", PartnerID: "customer", TradeMode: "goods", PriceMode: "exclusive",
		Lines: []TradeLine{{ItemID: "item", WarehouseID: "default", ProjectID: "ct-2", Quantity: "1", UnitPrice: "3500000", VATRate: "10"}},
	}); err != nil {
		t.Fatal(err)
	}

	// 5. Draft voucher on ct-1 must NOT affect report totals.
	if _, err = f.ops.SaveVoucher(ctx, f.cmd("ct1-draft"), Input{
		Kind: "sale", SourceID: "ct1-draft", Date: "2026-10-15", Description: "Nháp chưa ghi sổ", PartnerID: "customer", TradeMode: "service", PriceMode: "exclusive",
		Lines: []TradeLine{{ItemID: "service", ProjectID: "ct-1", Quantity: "1", UnitPrice: "99999999", VATRate: "0"}},
	}); err != nil {
		t.Fatal(err)
	}

	// 6. Costing 154 -> 632 for ct-1: cannot exceed ct-1's own 154 balance (5,000,000), even though total 154 is 9,000,000.
	if _, err = f.ops.Execute(ctx, f.cmd("ct1-costing-over"), Input{
		Kind: "costing", SourceID: "ct1-costing-over", Date: "2026-10-20", Description: "Vượt dở dang CT001", ProjectID: "ct-1", Amount: "6000000",
	}); accounting.Code(err) != "INSUFFICIENT_COST" {
		t.Fatalf("expected INSUFFICIENT_COST for project WIP, got %v", err)
	}
	if _, err = f.ops.Execute(ctx, f.cmd("ct1-costing"), Input{
		Kind: "costing", SourceID: "ct1-costing", Date: "2026-10-20", Description: "Kết chuyển giá thành CT001", ProjectID: "ct-1", Amount: "3000000",
	}); err != nil {
		t.Fatal(err)
	}

	// Expected for ct-1:
	// Revenue = 9007199264741000
	// Expense (6xx) = 9007199254741000 + 1200000 + 3000000 = 9007199258941000
	// WIP (154) = 5000000 - 3000000 = 2000000
	// TotalCost = Expense + WIP = 9007199260941000 (no double count of 3,000,000!)
	// Profit = 9007199264741000 - 9007199260941000 = 3800000
	//
	// Expected for ct-2:
	// Revenue = 3500000
	// Expense (632 COGS) = 1500000
	// WIP (154) = 4000000
	// TotalCost = 5500000
	// Profit = -2000000
	rep, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "project", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Total != 2 || len(rep.Rows) != 2 {
		t.Fatalf("expected 2 active projects (inactive zero hidden), got %+v", rep)
	}
	r1, r2 := rep.Rows[0], rep.Rows[1]
	if r1.Code != "CT001" || r1.Revenue != "9007199264741000" || r1.Expense != "9007199258941000" || r1.WIP != "2000000" || r1.TotalCost != "9007199260941000" || r1.Profit != "3800000" {
		t.Fatalf("unexpected CT001 summary: %+v", r1)
	}
	if r2.Code != "CT002" || r2.Revenue != "3500000" || r2.Expense != "1500000" || r2.WIP != "4000000" || r2.TotalCost != "5500000" || r2.Profit != "-2000000" {
		t.Fatalf("unexpected CT002 summary: %+v", r2)
	}
	if rep.Revenue != "9007199268241000" || rep.TotalCost != "9007199266441000" || rep.Profit != "1800000" {
		t.Fatalf("unexpected aggregate project totals: %+v", rep)
	}

	// Single-project detail drilldown & document filter.
	detail, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "project", Project: "ct-1", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if detail.ProjectSummary == nil || detail.ProjectSummary.Profit != "3800000" || detail.Total != 6 {
		t.Fatalf("unexpected ct-1 detail report: %+v", detail)
	}
	docs, err := f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Project: "ct-1", Page: 1, PageSize: 20})
	if err != nil || docs.Total != 5 {
		t.Fatalf("expected 5 documents (4 posted + 1 draft) for ct-1, got %+v %v", docs, err)
	}

	// Period-end closing must NOT wipe out project revenue/expense.
	rawClose, err := f.ops.Execute(ctx, f.cmd("close-oct"), Input{Kind: "closing", SourceID: "close-2026-10", Date: "2026-10-31", Description: "Kết chuyển tháng 10"})
	if err != nil {
		t.Fatal(err)
	}
	var closedRes Result
	if err = json.Unmarshal(rawClose, &closedRes); err != nil {
		t.Fatal(err)
	}
	afterClose, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "project", Page: 1, PageSize: 10})
	if err != nil || afterClose.Revenue != "9007199268241000" || afterClose.TotalCost != "9007199266441000" || afterClose.Profit != "1800000" {
		t.Fatalf("closing altered project report: %+v %v", afterClose, err)
	}

	// Reopen SQLite and verify Check + persisted project report.
	f.store.Close()
	reopened, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	f.store, f.ledger.Store = reopened, reopened
	if err = sqlite.Check(ctx, reopened.Read); err != nil {
		t.Fatal(err)
	}
	afterReopen, err := f.ledger.Report(ctx, ledger.Filter{From: "2026-10-01", To: "2026-10-31", Kind: "project", Page: 1, PageSize: 10})
	if err != nil || afterReopen.Profit != "1800000" {
		t.Fatalf("unexpected project report after reopen: %+v %v", afterReopen, err)
	}
}
