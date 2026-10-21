package operations

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func treasuryFixture(t *testing.T) *fixture {
	t.Helper()
	f := openFixture(t)
	f.policy(t, testPolicy())
	if _, err := f.ops.SaveItem(context.Background(), f.cmd("service"), Item{ID: "service", Code: "DV", Name: "Dịch vụ", Unit: "lần", Active: true}); err != nil {
		t.Fatal(err)
	}
	return f
}

func executeTreasury(t *testing.T, f *fixture, key string, input Input) Result {
	t.Helper()
	b, err := f.ops.Execute(context.Background(), f.cmd(key), input)
	if err != nil {
		t.Fatal(err)
	}
	var result Result
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func tradeInput(source, kind, price string) Input {
	partner := "customer"
	if kind == "purchase" {
		partner = "supplier"
	}
	deductible := true
	return Input{Kind: kind, SourceID: source, Date: "2026-10-01", Description: "Dịch vụ kiểm thử", PartnerID: partner, PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "service", Quantity: "1", UnitPrice: price, VATRate: "10", Deductible: &deductible}}}
}

func TestTreasuryPartialAllocationRetryReversalAndReopen(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	a := executeTreasury(t, f, "invoice-a", tradeInput("sale-a", "sale", "1000"))
	b := executeTreasury(t, f, "invoice-b", tradeInput("sale-b", "sale", "2000"))
	input := Input{Kind: "receipt", SourceID: "paid-a-b", Date: "2026-10-10", Description: "Thu một phần hai chứng từ", PartnerID: "customer", MoneyAccount: "1121", Amount: "600", Allocations: []Allocation{{DocumentID: a.Document.ID, Amount: "300"}, {DocumentID: b.Document.ID, Amount: "300"}}}
	preview, err := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, input)
	if err != nil || preview.Amounts["total"] != "600" {
		t.Fatal(preview, err)
	}
	before, err := f.ops.Invoices(ctx, "2026-10-10", "sale", "customer", 1, 1)
	if err != nil || before.Paid != "0" {
		t.Fatal(before, err)
	}
	first, err := f.ops.Execute(ctx, f.cmd("collect"), input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.ops.Execute(ctx, f.cmd("collect"), input)
	if err != nil || string(first) != string(again) {
		t.Fatal(string(again), err)
	}
	input.Amount = "601"
	if _, err = f.ops.Execute(ctx, f.cmd("collect"), input); accounting.Code(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal(err)
	}
	var payment Result
	if err = json.Unmarshal(first, &payment); err != nil {
		t.Fatal(err)
	}
	list, err := f.ops.Invoices(ctx, "2026-10-10", "sale", "customer", 1, 1)
	if err != nil || list.Total != 2 || len(list.Invoices) != 1 || list.Amount != "3300" || list.Paid != "600" || list.Remaining != "2700" {
		t.Fatal(list, err)
	}
	reversal := executeTreasury(t, f, "reverse", Input{Kind: "treasury_reversal", SourceID: "reverse-collect", OriginalDocumentID: payment.Document.ID, Date: "2026-10-15", Description: "Đảo phiếu thu lập sai"})
	if reversal.Document.ReversalOf != payment.Document.ID {
		t.Fatal(reversal)
	}
	for _, test := range []struct{ date, paid, remaining string }{{"2026-10-09", "0", "3300"}, {"2026-10-12", "600", "2700"}, {"2026-10-15", "0", "3300"}} {
		list, err = f.ops.Invoices(ctx, test.date, "sale", "customer", 1, 10)
		if err != nil || list.Paid != test.paid || list.Remaining != test.remaining {
			t.Fatal(test, list, err)
		}
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	f.store, f.ledger.Store = reopened, reopened
	list, err = f.ops.Invoices(ctx, "2026-10-31", "sale", "customer", 1, 10)
	if err != nil || list.Paid != "0" || list.Remaining != "3300" {
		t.Fatal(list, err)
	}
	input.Amount = "600"
	again, err = f.ops.Execute(ctx, f.cmd("collect"), input)
	if err != nil || string(first) != string(again) {
		t.Fatal(err)
	}
}

func TestTreasuryRejectsInvalidAllocationsAndRollsBack(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	sale := executeTreasury(t, f, "sale", tradeInput("sale", "sale", "1000"))
	purchase := executeTreasury(t, f, "purchase", tradeInput("purchase", "purchase", "1000"))
	valid := Input{Kind: "receipt", SourceID: "collect", Date: "2026-10-10", Description: "Thu tiền", PartnerID: "customer", MoneyAccount: "111", Amount: "500", Allocations: []Allocation{{DocumentID: sale.Document.ID, Amount: "500"}}}
	for _, test := range []struct {
		name, code string
		edit       func(*Input)
	}{
		{"overpay", "OVERPAYMENT", func(in *Input) { in.Amount = "1101"; in.Allocations[0].Amount = "1101" }},
		{"mismatch", "ALLOCATION_MISMATCH", func(in *Input) { in.Amount = "501" }},
		{"zero", "OVERPAYMENT", func(in *Input) { in.Allocations[0].Amount = "0" }},
		{"duplicate", "VALIDATION", func(in *Input) { in.Allocations = append(in.Allocations, in.Allocations[0]) }},
		{"wrong-partner", "INVALID_INVOICE", func(in *Input) { in.PartnerID = "supplier" }},
		{"wrong-kind", "INVALID_INVOICE", func(in *Input) { in.Allocations[0].DocumentID = purchase.Document.ID }},
		{"missing-invoice", "INVALID_INVOICE", func(in *Input) {
			in.Allocations = append(in.Allocations, Allocation{DocumentID: "missing", Amount: "1"})
		}},
		{"before-invoice", "INVALID_INVOICE", func(in *Input) { in.Date = "2026-09-30" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			input.Allocations = append([]Allocation{}, valid.Allocations...)
			test.edit(&input)
			if _, err := f.ops.Execute(ctx, f.cmd(test.name), input); accounting.Code(err) != test.code {
				t.Fatal(err)
			}
		})
	}
	var documents, operations, audits int
	if err := f.store.Read.QueryRow("SELECT count(*) FROM documents").Scan(&documents); err != nil || documents != 2 {
		t.Fatal(documents, err)
	}
	if err := f.store.Read.QueryRow("SELECT count(*) FROM operations").Scan(&operations); err != nil || operations != 2 {
		t.Fatal(operations, err)
	}
	if err := f.store.Read.QueryRow("SELECT count(*) FROM audit WHERE action='operation.receipt'").Scan(&audits); err != nil || audits != 0 {
		t.Fatal(audits, err)
	}
	paid := executeTreasury(t, f, "pay-supplier", Input{Kind: "payment", SourceID: "pay-supplier", Date: "2026-10-10", Description: "Trả nhà cung cấp", PartnerID: "supplier", MoneyAccount: "1121", Amount: "1100", Allocations: []Allocation{{DocumentID: purchase.Document.ID, Amount: "1100"}}})
	if paid.Document.Type != "payment" || paid.Document.Lines[0].AccountID != "331" || paid.Document.Lines[0].Debit != "1100" {
		t.Fatal(paid)
	}
	if _, err := f.ledger.SavePeriod(ctx, f.cmd("close"), ledger.Period{Period: "2026-10", Closed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ops.Execute(ctx, f.cmd("closed"), valid); accounting.Code(err) != "PERIOD_CLOSED" {
		t.Fatal(err)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestTreasuryConcurrentPaymentsCannotOverpay(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	sale := executeTreasury(t, f, "sale", tradeInput("sale", "sale", "1000"))
	input := Input{Kind: "receipt", Date: "2026-10-10", Description: "Thu đồng thời", PartnerID: "customer", MoneyAccount: "111", Amount: "700", Allocations: []Allocation{{DocumentID: sale.Document.ID, Amount: "700"}}}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, key := range []string{"first", "second"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			in := input
			in.SourceID = key
			_, err := f.ops.Execute(ctx, f.cmd(key), in)
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	var successes, failures int
	for err := range errors {
		if err == nil {
			successes++
		} else if accounting.Code(err) == "OVERPAYMENT" {
			failures++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatal(successes, failures)
	}
	list, err := f.ops.Invoices(ctx, "2026-10-10", "sale", "customer", 1, 10)
	if err != nil || list.Paid != "700" || list.Remaining != "400" {
		t.Fatal(list, err)
	}
}

func TestTreasuryMoneySummaryAndUnallocatedCredits(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	executeTreasury(t, f, "capital", Input{Kind: "receipt", SourceID: "capital", Date: "2026-10-01", Description: "Thu vốn", MoneyAccount: "111", CounterAccount: "411", Amount: "9007199254740993"})
	executeTreasury(t, f, "transfer", Input{Kind: "transfer", SourceID: "transfer", Date: "2026-10-02", Description: "Nộp tiền vào ngân hàng", MoneyAccount: "111", DestinationAccount: "1121", Amount: "500"})
	executeTreasury(t, f, "expense", Input{Kind: "payment", SourceID: "expense", Date: "2026-10-03", Description: "Chi phí", MoneyAccount: "1121", CounterAccount: "642", Amount: "100"})
	for _, test := range []struct{ account, receipt, payment, closing string }{{"111", "9007199254740993", "500", "9007199254740493"}, {"1121", "500", "100", "400"}} {
		list, err := f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Account: test.account, Status: "draft", Page: 1, PageSize: 1})
		if err != nil || list.Money == nil || list.Money.Opening != "0" || list.Money.Receipts != test.receipt || list.Money.Payments != test.payment || list.Money.Closing != test.closing {
			t.Fatal(test, list, err)
		}
	}
	sale := executeTreasury(t, f, "sale", tradeInput("sale", "sale", "1000"))
	d := f.draft("old-manual-payment")
	d.Lines = []ledger.Line{{AccountID: "111", Debit: "100", Credit: "0"}, {AccountID: "131", PartnerID: "customer", Debit: "0", Credit: "100"}}
	raw, err := f.ledger.Save(ctx, f.cmd("manual"), d)
	d = decodeDocument(t, raw, err)
	if _, err = f.ledger.Post(ctx, f.cmd("post-manual"), ledger.Post{ID: d.ID, Version: d.Version}); err != nil {
		t.Fatal(err)
	}
	list, err := f.ops.Invoices(ctx, "2026-10-31", "sale", "customer", 1, 10)
	if err != nil || !list.Invoices[0].Unallocated {
		t.Fatal(list, err)
	}
	input := Input{Kind: "receipt", SourceID: "new-payment", Date: "2026-10-31", Description: "Thu chưa đối soát", PartnerID: "customer", MoneyAccount: "111", Amount: "100", Allocations: []Allocation{{DocumentID: sale.Document.ID, Amount: "100"}}}
	if _, err = f.ops.Execute(ctx, f.cmd("unallocated"), input); accounting.Code(err) != "UNALLOCATED_SETTLEMENT" {
		t.Fatal(err)
	}
}

func TestTradeInvoiceMetadataAndIssuerIdentity(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	in := tradeInput("invoice", "sale", "1000")
	in.InvoiceNumber, in.InvoiceSeries, in.InvoiceDate, in.DueDate = "000001", "SERIES", "2026-10-01", "2026-10-15"
	executeTreasury(t, f, "invoice", in)
	in.SourceID = "duplicate"
	if _, err := f.ops.Execute(ctx, f.cmd("duplicate"), in); accounting.Code(err) != "DUPLICATE_INVOICE" {
		t.Fatal(err)
	}
	purchase := tradeInput("purchase", "purchase", "1000")
	purchase.InvoiceNumber, purchase.InvoiceSeries, purchase.InvoiceDate = "000001", "SERIES", "2026-10-01"
	executeTreasury(t, f, "other-issuer", purchase)
	list, err := f.ops.Invoices(ctx, "2026-10-31", "sale", "", 1, 10)
	if err != nil || list.Invoices[0].InvoiceNumber != "000001" || list.Invoices[0].DueDate != "2026-10-15" {
		t.Fatal(list, err)
	}
	in.InvoiceDate = ""
	if _, err = f.ops.Execute(ctx, f.cmd("missing-date"), in); accounting.Code(err) != "VALIDATION" {
		t.Fatal(err)
	}
}

func TestTreasuryPaysPrepaidPurchaseWithoutDuplicatingAsset(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	asset := executeTreasury(t, f, "rent", Input{Kind: "asset", SourceID: "rent", Date: "2026-10-01", Description: "Chi phí thuê trả trước", PartnerID: "supplier", Asset: Asset{ID: "rent", Code: "RENT", Name: "Thuê văn phòng", Kind: "prepaid", Cost: "240000000", Residual: "0", Months: 12, StartPeriod: "2026-10"}})
	list, err := f.ops.Invoices(ctx, "2026-10-31", "purchase", "supplier", 1, 10)
	if err != nil || list.Total != 1 || list.Remaining != "240000000" {
		t.Fatal(list, err)
	}
	executeTreasury(t, f, "pay-rent", Input{Kind: "payment", SourceID: "pay-rent", Date: "2026-10-21", Description: "Thanh toán tiền thuê", PartnerID: "supplier", MoneyAccount: "1121", Amount: "240000000", Allocations: []Allocation{{DocumentID: asset.Document.ID, Amount: "240000000"}}})
	list, err = f.ops.Invoices(ctx, "2026-10-31", "purchase", "supplier", 1, 10)
	if err != nil || list.Remaining != "0" {
		t.Fatal(list, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestTreasuryMoneySummaryUsesCalendarBoundaries(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	for _, period := range []string{"2026-09", "2026-11"} {
		if _, err := f.ledger.SavePeriod(ctx, f.cmd(period), ledger.Period{Period: period}); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []struct{ source, date, kind, amount string }{{"opening", "2026-09-30", "receipt", "100"}, {"start", "2026-10-01", "receipt", "200"}, {"end", "2026-10-31", "payment", "20"}, {"next", "2026-11-01", "receipt", "400"}} {
		executeTreasury(t, f, entry.source, Input{Kind: entry.kind, SourceID: entry.source, Date: entry.date, Description: "Biên kỳ", MoneyAccount: "111", CounterAccount: "411", Amount: entry.amount})
	}
	list, err := f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Account: "111", Page: 1, PageSize: 1})
	if err != nil || list.Money == nil || *list.Money != (ledger.MoneySummary{Opening: "100", Receipts: "200", Payments: "20", Closing: "280"}) {
		t.Fatal(list, err)
	}
}

func TestTreasuryRequiresManagedInvoiceSource(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	d := f.draft("manual-sale")
	d.Type, d.PartnerID = "sale", "customer"
	d.Lines = []ledger.Line{{AccountID: "131", PartnerID: "customer", Debit: "1000", Credit: "0"}, {AccountID: "511", Debit: "0", Credit: "1000"}}
	raw, err := f.ledger.Save(ctx, f.cmd("manual-sale"), d)
	d = decodeDocument(t, raw, err)
	if _, err = f.ledger.Post(ctx, f.cmd("post-manual-sale"), ledger.Post{ID: d.ID, Version: d.Version}); err != nil {
		t.Fatal(err)
	}
	in := Input{Kind: "receipt", SourceID: "manual-collection", Date: d.Date, Description: "Thu tiền", PartnerID: "customer", MoneyAccount: "111", Amount: "100", Allocations: []Allocation{{DocumentID: d.ID, Amount: "100"}}}
	if _, err = f.ops.Execute(ctx, f.cmd("manual-collection"), in); accounting.Code(err) != "UNALLOCATED_SETTLEMENT" {
		t.Fatal(err)
	}
}

func TestTreasuryBackupRejectsInvalidAllocationPayload(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	sale := executeTreasury(t, f, "sale", tradeInput("sale", "sale", "1000"))
	payment := executeTreasury(t, f, "payment", Input{Kind: "receipt", SourceID: "payment", Date: "2026-10-21", Description: "Thu công nợ", PartnerID: "customer", MoneyAccount: "111", Amount: "100", Allocations: []Allocation{{DocumentID: sale.Document.ID, Amount: "100"}}})
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
	for _, amount := range []string{"100", "101"} {
		if _, err := f.store.Write.Exec("UPDATE operations SET payload=json_set(payload,'$.allocations[0].amount','101','$.amount',?) WHERE document_id=?", amount, payment.Document.ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := f.store.Backup(ctx); err == nil {
			t.Fatal("backup accepted invalid settlement allocations or ledger mismatch")
		}
	}
}
