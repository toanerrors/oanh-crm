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

func TestTrialBalanceHidesSixZeroColumnsAndKeepsActiveAccounts(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	empty, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "trial", PageSize: 200})
	if err != nil || empty.Total != 0 || len(empty.Rows) != 0 || empty.OpeningDebit != "0" || empty.ClosingCredit != "0" {
		t.Fatal(empty, err)
	}
	zeroLedger, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "ledger", Account: "112", PageSize: 20})
	if err != nil || zeroLedger.AccountBalance == nil || zeroLedger.AccountBalance.Code != "112" || zeroLedger.AccountBalance.ClosingDebit != "0" || zeroLedger.AccountBalance.ClosingCredit != "0" {
		t.Fatal(zeroLedger, err)
	}
	in := openingInput()
	b, err := f.ops.SaveOpening(ctx, f.cmd("trial-opening-save"), in)
	state := decodeOpening(t, b, err)
	if _, err = f.ops.PostOpening(ctx, f.cmd("trial-opening-post"), ledger.Post{ID: state.Document.ID, Version: state.Document.Version}); err != nil {
		t.Fatal(err)
	}
	for index, lines := range [][]ledger.Line{
		{{AccountID: "131", PartnerID: "customer", Debit: "500", Credit: "0"}, {AccountID: "511", Debit: "0", Credit: "500"}},
		{{AccountID: "511", Debit: "500", Credit: "0"}, {AccountID: "131", PartnerID: "customer", Debit: "0", Credit: "500"}},
	} {
		doc := f.draft("settled-in-period-" + string(rune('a'+index)))
		doc.Lines = lines
		b, err = f.ledger.Save(ctx, f.cmd("save-settled-"+string(rune('a'+index))), doc)
		saved := decodeDocument(t, b, err)
		if _, err = f.ledger.Post(ctx, f.cmd("post-settled-"+string(rune('a'+index))), ledger.Post{ID: saved.ID, Version: saved.Version}); err != nil {
			t.Fatal(err)
		}
	}
	october, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "trial", PageSize: 200})
	if err != nil || october.Total != 4 || len(october.Rows) != 4 || october.OpeningDebit != "9007199254740993" || october.Debit != "1000" || october.Credit != "1000" || october.ClosingDebit != "9007199254740993" {
		t.Fatal(october, err)
	}
	codes := []string{october.Rows[0].Code, october.Rows[1].Code, october.Rows[2].Code, october.Rows[3].Code}
	if codes[0] != "111" || codes[1] != "131" || codes[2] != "411" || codes[3] != "511" {
		t.Fatal(codes)
	}
	if october.Rows[1].OpeningDebit != "0" || october.Rows[1].Debit != "500" || october.Rows[1].Credit != "500" || october.Rows[1].ClosingDebit != "0" || october.Rows[1].ClosingCredit != "0" {
		t.Fatal(october.Rows[1])
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.store = reopened
	f.ledger.Store = reopened
	t.Cleanup(func() { reopened.Close() })
	november, err := f.ledger.Report(ctx, ledger.Filter{Period: "2026-11", Kind: "trial", PageSize: 200})
	if err != nil || november.Total != 2 || len(november.Rows) != 2 || november.Rows[0].Code != "111" || november.Rows[1].Code != "411" || november.Debit != "0" || november.ClosingDebit != "9007199254740993" {
		t.Fatal(november, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestCustomDateRangeReportsDocumentsAndMoneyCutoff(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	in := OpeningInput{
		ID:     "range-opening",
		Cutoff: "2026-02-01",
		Source: "Sổ chuyển đổi 01/02/2026",
		Lines: []OpeningLine{
			openingLine("111", "", "9007199254740993", "0"),
			openingLine("131", "customer", "1000", "0"),
			openingLine("131", "supplier", "0", "400"),
			openingLine("411", "", "0", "9007199254741593"),
		},
	}
	b, err := f.ops.SaveOpening(ctx, f.cmd("range-opening-save"), in)
	state := decodeOpening(t, b, err)
	if _, err = f.ops.PostOpening(ctx, f.cmd("range-opening-post"), ledger.Post{ID: state.Document.ID, Version: state.Document.Version}); err != nil {
		t.Fatal(err)
	}
	vouchers := []struct {
		id    string
		date  string
		lines []ledger.Line
	}{
		{"doc-feb", "2026-02-15", []ledger.Line{{AccountID: "111", Debit: "200", Credit: "0"}, {AccountID: "131", PartnerID: "customer", Debit: "0", Credit: "200"}}},
		{"doc-oct-before", "2026-10-09", []ledger.Line{{AccountID: "642", Debit: "300", Credit: "0"}, {AccountID: "111", Debit: "0", Credit: "300"}}},
		{"doc-oct-start", "2026-10-10", []ledger.Line{{AccountID: "131", PartnerID: "customer", Debit: "500", Credit: "0"}, {AccountID: "511", Debit: "0", Credit: "500"}}},
		{"doc-nov-end", "2026-11-15", []ledger.Line{{AccountID: "111", Debit: "150", Credit: "0"}, {AccountID: "131", PartnerID: "supplier", Debit: "0", Credit: "150"}}},
		{"doc-nov-after", "2026-11-16", []ledger.Line{{AccountID: "642", Debit: "700", Credit: "0"}, {AccountID: "111", Debit: "0", Credit: "700"}}},
	}
	for _, v := range vouchers {
		doc := f.draft(v.id)
		doc.Date = v.date
		doc.Lines = v.lines
		b, err = f.ledger.Save(ctx, f.cmd("save-"+v.id), doc)
		saved := decodeDocument(t, b, err)
		if _, err = f.ledger.Post(ctx, f.cmd("post-"+v.id), ledger.Post{ID: saved.ID, Version: saved.Version}); err != nil {
			t.Fatal(err)
		}
	}
	draft := f.draft("doc-nov-draft")
	draft.Date = "2026-11-10"
	draft.Lines = []ledger.Line{{AccountID: "642", Debit: "999", Credit: "0"}, {AccountID: "111", Debit: "0", Credit: "999"}}
	if _, err = f.ledger.Save(ctx, f.cmd("save-draft"), draft); err != nil {
		t.Fatal(err)
	}

	if _, err = f.ledger.Report(ctx, ledger.Filter{From: "2026-01-01", To: "2026-11-18", Kind: "trial"}); accounting.Code(err) != "BEFORE_CONVERSION" {
		t.Fatalf("expected BEFORE_CONVERSION, got %v", err)
	}
	for _, bad := range []ledger.Filter{{From: "2026-11-15", To: "2026-10-10", Kind: "trial"}, {From: "2026-10-10", Kind: "trial"}, {From: "2026-02-29", To: "2026-03-01", Kind: "trial"}} {
		if _, err = f.ledger.Report(ctx, bad); accounting.Code(err) != "BAD_REQUEST" {
			t.Fatalf("expected BAD_REQUEST for %+v, got %v", bad, err)
		}
	}

	sliceFilter := ledger.Filter{Period: "2026-11", From: "2026-10-10", To: "2026-11-15", Kind: "trial", PageSize: 200}
	sliceTrial, err := f.ledger.Report(ctx, sliceFilter)
	if err != nil {
		t.Fatal(err)
	}
	if sliceTrial.Period != "2026-10-10 – 2026-11-15" || sliceTrial.OpeningDebit != "9007199254741993" || sliceTrial.OpeningCredit != "9007199254741993" || sliceTrial.Debit != "650" || sliceTrial.Credit != "650" || sliceTrial.ClosingDebit != "9007199254742643" || sliceTrial.ClosingCredit != "9007199254742643" {
		t.Fatalf("unexpected slice trial summary: %+v", sliceTrial)
	}
	byCode := map[string]ledger.Row{}
	for _, r := range sliceTrial.Rows {
		byCode[r.Code] = r
	}
	if r := byCode["131"]; r.OpeningDebit != "800" || r.OpeningCredit != "400" || r.Debit != "500" || r.Credit != "150" || r.ClosingDebit != "1300" || r.ClosingCredit != "550" {
		t.Fatalf("unexpected two-sided 131 row: %+v", r)
	}

	debt, err := f.ledger.Report(ctx, ledger.Filter{From: "2026-10-10", To: "2026-11-15", Kind: "debt", Account: "receivable", PageSize: 50})
	if err != nil || debt.Total != 2 || debt.Debit != "1300" || debt.Credit != "550" {
		t.Fatalf("unexpected debt report: %+v %v", debt, err)
	}
	journal, err := f.ledger.Report(ctx, ledger.Filter{From: "2026-10-10", To: "2026-11-15", Kind: "journal", PageSize: 50})
	if err != nil || journal.Total != 4 || journal.Debit != "650" || journal.Credit != "650" {
		t.Fatalf("unexpected journal report: %+v %v", journal, err)
	}
	cashDocs, err := f.ledger.Documents(ctx, ledger.Filter{From: "2026-10-10", To: "2026-11-15", Account: "111", Status: "all", PageSize: 50})
	if err != nil || cashDocs.Counts["all"] != 2 || cashDocs.Counts["posted"] != 1 || cashDocs.Counts["draft"] != 1 || cashDocs.Money == nil || cashDocs.Money.Opening != "9007199254740893" || cashDocs.Money.Receipts != "150" || cashDocs.Money.Payments != "0" || cashDocs.Money.Closing != "9007199254741043" {
		t.Fatalf("unexpected cash documents/money: %+v %+v %v", cashDocs, cashDocs.Money, err)
	}

	ytdDocs, err := f.ledger.Documents(ctx, ledger.Filter{From: "2026-02-01", To: "2026-11-18", Account: "all", Status: "all", PageSize: 50})
	if err != nil || ytdDocs.Counts["all"] != 6 || ytdDocs.Counts["posted"] != 5 || ytdDocs.Counts["draft"] != 1 {
		t.Fatalf("unexpected YTD documents: %+v %v", ytdDocs, err)
	}
	ytdTrial, err := f.ledger.Report(ctx, ledger.Filter{From: "2026-02-01", To: "2026-11-18", Kind: "trial", PageSize: 200})
	if err != nil || ytdTrial.OpeningDebit != "9007199254741993" || ytdTrial.Debit != "1850" || ytdTrial.Credit != "1850" {
		t.Fatalf("unexpected YTD trial: %+v %v", ytdTrial, err)
	}

	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.store = reopened
	f.ledger.Store = reopened
	t.Cleanup(func() { reopened.Close() })
	afterReopen, err := f.ledger.Report(ctx, sliceFilter)
	if err != nil || afterReopen.ClosingDebit != sliceTrial.ClosingDebit || afterReopen.Debit != "650" {
		t.Fatalf("unexpected report after reopen: %+v %v", afterReopen, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
