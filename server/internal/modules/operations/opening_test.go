package operations

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func openingLine(a, p, d, c string) OpeningLine {
	return OpeningLine{Line: ledger.Line{AccountID: a, PartnerID: p, Debit: d, Credit: c}}
}
func decodeOpening(t *testing.T, b []byte, err error) OpeningState {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var state OpeningState
	if err = json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	return state
}
func openingInput() OpeningInput {
	return OpeningInput{ID: "opening", Cutoff: "2026-10-01", Source: "Sổ đối chiếu giả lập 30/09/2026", Lines: []OpeningLine{openingLine("111", "", "9007199254740993", "0"), openingLine("411", "", "0", "9007199254740993")}}
}
func TestOpeningReportsAndCarryForward(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	in := openingInput()
	in.Lines = append(in.Lines, openingLine("131", "customer", "100", "0"), openingLine("131", "supplier", "0", "40"), openingLine("411", "", "0", "60"))
	b, e := f.ops.SaveOpening(ctx, f.cmd("opening-save"), in)
	state := decodeOpening(t, b, e)
	if state.Document.Version != 1 || state.Document.Status != "draft" {
		t.Fatal(state)
	}
	before, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "trial", PageSize: 200})
	if e != nil || before.OpeningDebit != "0" {
		t.Fatal(before, e)
	}
	if _, e = f.ledger.Save(ctx, f.cmd("generic-hijack"), ledger.Document{ID: in.ID, Version: 1, Type: "journal", Date: in.Cutoff, Description: "hijack", Lines: inToLines(in.Lines)}); accounting.Code(e) != "SUBLEDGER_REQUIRED" {
		t.Fatal(e)
	}
	if _, e = f.ledger.Post(ctx, f.cmd("generic-post"), ledger.Post{ID: in.ID, Version: 1}); accounting.Code(e) != "SUBLEDGER_REQUIRED" {
		t.Fatal(e)
	}
	post := ledger.Post{ID: in.ID, Version: 1}
	b, e = f.ops.PostOpening(ctx, f.cmd("opening-post"), post)
	state = decodeOpening(t, b, e)
	retry, e := f.ops.PostOpening(ctx, f.cmd("opening-post"), post)
	if e != nil || string(retry) != string(b) {
		t.Fatal(e)
	}
	if state.Document.Status != "posted" || state.Document.Reference != "SDDK-2026-000001" {
		t.Fatal(state.Document)
	}
	report, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "trial", PageSize: 200})
	if e != nil || report.OpeningDebit != "9007199254741093" || report.OpeningCredit != report.OpeningDebit || report.Debit != "0" || report.Credit != "0" {
		t.Fatal(report, e)
	}
	for _, r := range report.Rows {
		if r.Code == "131" && (r.OpeningDebit != "100" || r.OpeningCredit != "40") {
			t.Fatal(r)
		}
	}
	journal, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "journal", PageSize: 200})
	if e != nil || journal.Total != 0 {
		t.Fatal(journal, e)
	}
	source, e := f.ledger.FinancialSource(ctx, "2026-10-01", "2026-10-31")
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range source.Rows {
		if r.Code == "111" && (r.OpeningDebit != "9007199254740993" || r.Debit != "0") {
			t.Fatal(r)
		}
	}
	money, e := f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Account: "111", PageSize: 10})
	if e != nil || money.Money.Opening != "9007199254740993" || money.Money.Receipts != "0" {
		t.Fatal(money, e)
	}
	doc := f.draft("movement")
	doc.Lines = []ledger.Line{{AccountID: "111", Debit: "20", Credit: "0"}, {AccountID: "131", PartnerID: "customer", Debit: "0", Credit: "20"}}
	b, e = f.ledger.Save(ctx, f.cmd("save-movement"), doc)
	d := decodeDocument(t, b, e)
	if _, e = f.ledger.Post(ctx, f.cmd("post-movement"), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
		t.Fatal(e)
	}
	next, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-11", Kind: "trial", PageSize: 200})
	if e != nil || next.OpeningDebit != "9007199254741093" || next.Debit != "0" {
		t.Fatal(next, e)
	}
	for _, r := range next.Rows {
		if r.Code == "131" && (r.OpeningDebit != "80" || r.OpeningCredit != "40") {
			t.Fatal(r)
		}
	}
	old := f.draft("old")
	old.Date = "2026-09-30"
	b, e = f.ledger.Save(ctx, f.cmd("save-old"), old)
	d = decodeDocument(t, b, e)
	if _, e = f.ledger.Post(ctx, f.cmd("post-old"), ledger.Post{ID: d.ID, Version: d.Version}); accounting.Code(e) != "BEFORE_CONVERSION" {
		t.Fatal(e)
	}
	if _, e = f.ops.SaveOpening(ctx, f.cmd("rewrite-opening"), *state.Input); accounting.Code(e) != "OPENING_LOCKED" {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("UPDATE opening_balances SET source='changed'"); e == nil {
		t.Fatal("mutable opening")
	}
	if _, e = f.ledger.Reverse(ctx, f.cmd("reverse-opening"), ledger.Reversal{ID: in.ID, Date: "2026-10-22", Reason: "test"}); accounting.Code(e) != "SUBLEDGER_REVERSAL_REQUIRED" {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	archive, _, e := f.store.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	stage, e := f.store.Stage(ctx, archive)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Activate(ctx, stage.ID); e != nil {
		t.Fatal(e)
	}
	state, e = f.ops.Opening(ctx)
	if e != nil || state.Document.Status != "posted" {
		t.Fatal(state, e)
	}
}
func inToLines(lines []OpeningLine) []ledger.Line {
	out := []ledger.Line{}
	for _, l := range lines {
		out = append(out, l.Line)
	}
	return out
}
func TestOpeningBankStockAssetAndDepreciation(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	f.policy(t, testPolicy())
	if _, e := f.ops.SaveItem(ctx, f.cmd("item"), Item{ID: "goods", Code: "HH", Name: "Hàng giả lập", Unit: "Cái", Tracked: true, Active: true}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ops.SaveBank(ctx, f.cmd("bank"), BankAccount{ID: "bank", Number: "000123", BankName: "Ngân hàng giả lập", AccountID: "112", Active: true}); e != nil {
		t.Fatal(e)
	}
	in := OpeningInput{ID: "opening", Cutoff: "2026-10-01", Source: "Đối chiếu giả lập", Lines: []OpeningLine{{Line: ledger.Line{AccountID: "112", Debit: "500", Credit: "0"}, BankAccountID: "bank"}, openingLine("411", "", "0", "2501")}, Stocks: []OpeningStock{{ItemID: "goods", WarehouseID: "default", Quantity: "2.50", Value: "1001"}}, Assets: []Asset{{ID: "asset", Code: "TS", Name: "Tài sản giả lập", Kind: "fixed", Cost: "1200", Allocated: "300", Residual: "0", Months: 12, StartPeriod: "2026-07"}, {ID: "prepaid", Code: "CP", Name: "Trả trước giả lập", Kind: "prepaid", Cost: "200", Allocated: "100", Residual: "0", Months: 2, StartPeriod: "2026-09"}}}
	in.Lines[1].Credit = "1"
	b, e := f.ops.SaveOpening(ctx, f.cmd("save-unbalanced-opening"), in)
	state := decodeOpening(t, b, e)
	if _, e = f.ops.PostOpening(ctx, f.cmd("unbalanced-post"), ledger.Post{ID: in.ID, Version: state.Document.Version}); accounting.Code(e) != "UNBALANCED_ENTRY" {
		t.Fatal(e)
	}
	var mutations int
	if e = f.store.Read.QueryRow("SELECT (SELECT count(*) FROM bank_movements)+(SELECT count(*) FROM stock_movements)+(SELECT count(*) FROM stock_lots)+(SELECT count(*) FROM assets)+(SELECT count(*) FROM opening_assets)").Scan(&mutations); e != nil || mutations != 0 {
		t.Fatal("partial opening subledgers", mutations, e)
	}
	in.Version = state.Document.Version
	in.Lines[1].Credit = "2501"
	b, e = f.ops.SaveOpening(ctx, f.cmd("save-opening"), in)
	state = decodeOpening(t, b, e)
	if _, e = f.ops.PostOpening(ctx, f.cmd("post-opening"), ledger.Post{ID: in.ID, Version: state.Document.Version}); e != nil {
		t.Fatal(e)
	}
	banks, e := f.ops.Banks(ctx, "2026-10")
	if e != nil || banks.Accounts[0].Opening != "500" || banks.Accounts[0].Receipts != "0" || banks.Accounts[0].Closing != "500" {
		t.Fatal(banks, e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"asset", "prepaid"} {
		b, e = f.ops.Execute(ctx, f.cmd("depreciate-"+id), Input{Kind: "depreciation", SourceID: id + ":2026-10", Date: "2026-10-31", Description: "Phân bổ sau chuyển đổi", AssetID: id})
		if e != nil {
			t.Fatal(string(b), e)
		}
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	// The opening lot is available to the existing FIFO sale path.
	b, e = f.ops.Execute(ctx, f.cmd("sale-opening-stock"), Input{Kind: "sale", SourceID: "sale-initial-stock", Date: "2026-10-31", Description: "Bán hàng tồn chuyển đổi", PartnerID: "customer", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "goods", WarehouseID: "default", Quantity: "1.00", UnitPrice: "1000", VATRate: "0"}}})
	if e != nil {
		t.Fatal(string(b), e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	archive, _, e := f.store.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	stage, e := f.store.Stage(ctx, archive)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Activate(ctx, stage.ID); e != nil {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}
func TestOpeningRollbackVersionAndConcurrency(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	in := openingInput()
	in.Lines[1].Credit = "1"
	b, e := f.ops.SaveOpening(ctx, f.cmd("save-opening"), in)
	state := decodeOpening(t, b, e)
	if _, e = f.ops.PostOpening(ctx, f.cmd("post-bad"), ledger.Post{ID: in.ID, Version: state.Document.Version}); accounting.Code(e) != "UNBALANCED_ENTRY" {
		t.Fatal(e)
	}
	var n int
	f.store.Read.QueryRow("SELECT count(*) FROM counters WHERE scope='opening:2026'").Scan(&n)
	if n != 0 {
		t.Fatal("failed post consumed counter")
	}
	in.Version = state.Document.Version
	in.Lines[1].Credit = in.Lines[0].Debit
	b, e = f.ops.SaveOpening(ctx, f.cmd("fix-opening"), in)
	state = decodeOpening(t, b, e)
	if _, e = f.ops.SaveOpening(ctx, f.cmd("stale-opening"), in); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("CREATE TRIGGER fail_opening_audit BEFORE INSERT ON audit WHEN NEW.action='admin.opening.post' BEGIN SELECT RAISE(ABORT,'test rollback'); END"); e != nil {
		t.Fatal(e)
	}
	post := ledger.Post{ID: in.ID, Version: state.Document.Version}
	if _, e = f.ops.PostOpening(ctx, f.cmd("retry-opening"), post); e == nil {
		t.Fatal("expected rollback")
	}
	got, e := f.ops.Opening(ctx)
	if e != nil || got.Document.Status != "draft" {
		t.Fatal(got, e)
	}
	if _, e = f.store.Write.Exec("DROP TRIGGER fail_opening_audit"); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.ops.PostOpening(ctx, f.cmd("retry-opening"), post); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE status='posted'").Scan(&n)
	if n != 1 {
		t.Fatal(n)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}
func TestOpeningRejectsHistoryAndMissingDetails(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	for _, a := range []string{"156", "211", "511"} {
		in := openingInput()
		in.Lines[0].AccountID = a
		if _, e := f.ops.SaveOpening(ctx, f.cmd("invalid-"+a), in); e == nil {
			t.Fatal(a)
		}
	}
	in := openingInput()
	in.Lines[0].AccountID = "112"
	if _, e := f.ops.SaveOpening(ctx, f.cmd("missing-bank"), in); accounting.Code(e) != "INVALID_BANK" {
		t.Fatal(e)
	}
	in = openingInput()
	in.Lines[0].AccountID = "131"
	if _, e := f.ops.SaveOpening(ctx, f.cmd("missing-partner"), in); accounting.Code(e) != "INVALID_PARTNER" {
		t.Fatal(e)
	}
	d := f.save(t, "history")
	if _, e := f.ledger.Post(ctx, f.cmd("post-history"), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ops.SaveOpening(ctx, f.cmd("overlap"), openingInput()); accounting.Code(e) != "OPENING_HISTORY_EXISTS" {
		t.Fatal(e)
	}
}
