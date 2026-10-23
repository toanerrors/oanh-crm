package operations

import (
	"context"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestTradeAmountsMatchPostingWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name, price, rate, mode, rounding, vatMethod, base, tax, total, discount string
		d                                                                        Discount
	}{
		{name: "purchase-vat8", price: "3000000", rate: "8", mode: "exclusive", base: "3000000", tax: "240000", total: "3240000", discount: "0"},
		{name: "inclusive-vat8", price: "3240000", rate: "8", mode: "inclusive", base: "3000000", tax: "240000", total: "3240000", discount: "0"},
		{name: "percent-discount", price: "1001", rate: "10", mode: "exclusive", base: "901", tax: "90", total: "991", discount: "100", d: Discount{Mode: "percent", Rate: "10"}},
		{name: "inclusive-discount", price: "1100", rate: "10", mode: "inclusive", base: "909", tax: "91", total: "1000", discount: "100", d: Discount{Mode: "line"}},
		{name: "large-money", price: "900719925474099.3", rate: "0", mode: "exclusive", base: "9007199254740993", tax: "0", total: "9007199254740993", discount: "0"},
		{name: "half-even", price: "1000.5", rate: "0", mode: "exclusive", rounding: "half_even", base: "1000", tax: "0", total: "1000", discount: "0"},
		{name: "down", price: "1000.9", rate: "0", mode: "exclusive", rounding: "down", base: "1000", tax: "0", total: "1000", discount: "0"},
		{name: "direct-vat", price: "1000", rate: "8", mode: "inclusive", vatMethod: "direct", base: "920", tax: "80", total: "1000", discount: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := openFixture(t)
			ctx := context.Background()
			p := testPolicy()
			p.VATRates = []string{"0", "8", "10"}
			if tc.rounding != "" {
				p.Rounding = tc.rounding
			}
			if tc.vatMethod != "" {
				p.VATMethod = tc.vatMethod
			}
			f.policy(t, p)
			if _, err := f.ops.SaveItem(ctx, f.cmd("service"), Item{ID: "service", Code: "DV", Name: "Dịch vụ", Unit: "lần", Active: true}); err != nil {
				t.Fatal(err)
			}
			in := tradeInput(tc.name, "purchase", tc.price)
			in.PriceMode, in.Discount, in.Lines[0].VATRate = tc.mode, tc.d, tc.rate
			if tc.name == "large-money" {
				in.Lines[0].Quantity = "10"
			}
			if tc.d.Mode == "line" {
				in.Lines[0].DiscountAmount = "100"
			}
			incomplete := in
			incomplete.SourceID, incomplete.PartnerID, incomplete.Description = "", "", ""
			incomplete.InvoiceNumber, incomplete.PaymentAccount = "15", "1121"
			before, err := ledger.ReadContext(ctx, f.store.Read)
			if err != nil {
				t.Fatal(err)
			}
			quote, err := f.ops.TradeAmounts(ctx, identity.User{ID: f.actor, Role: "accountant"}, f.epoch, incomplete)
			if err != nil {
				t.Fatal(err)
			}
			for key, expected := range map[string]string{"base": tc.base, "tax": tc.tax, "total": tc.total, "discount": tc.discount} {
				if quote.Amounts[key] != expected {
					t.Fatalf("%s: got %s, want %s", key, quote.Amounts[key], expected)
				}
			}
			if quote.LineAmounts[0].Tax != tc.tax || quote.PolicyID == "" || quote.PolicyVersion != 1 || quote.Revision != before.Revision {
				t.Fatal(quote)
			}
			after, err := ledger.ReadContext(ctx, f.store.Read)
			if err != nil || after.Revision != before.Revision {
				t.Fatal(after, err)
			}
			for _, table := range []string{"documents", "operations", "counters", "stock_movements"} {
				var count int
				if err = f.store.Read.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
					t.Fatal(table, count, err)
				}
			}
			posted := executeTreasury(t, f, "post", in)
			for key, expected := range quote.Amounts {
				if posted.Amounts[key] != expected {
					t.Fatalf("posting %s: got %s, quote %s", key, posted.Amounts[key], expected)
				}
			}
			if tc.tax != "0" && p.VATMethod == "deduction" {
				found := false
				for _, line := range posted.Document.Lines {
					if line.AccountID == "1331" && line.Debit == tc.tax {
						found = true
					}
				}
				if !found {
					t.Fatal("missing deductible VAT on 1331", posted.Document.Lines)
				}
			}
			if err = sqlite.Check(ctx, f.store.Read); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTradeAmountsAllocationValidationAndReopen(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	user := identity.User{ID: f.actor, Role: "owner"}
	in := tradeInput("quote", "purchase", "100")
	in.Lines = append(in.Lines, in.Lines[0], in.Lines[0])
	in.Discount = Discount{Mode: "total", Amount: "2", Allocation: "value"}
	quote, err := f.ops.TradeAmounts(ctx, user, f.epoch, in)
	if err != nil || quote.LineAmounts[0].Discount != "1" || quote.LineAmounts[1].Discount != "1" || quote.LineAmounts[2].Discount != "0" || quote.Amounts["total"] != "328" {
		t.Fatal(quote, err)
	}
	if _, err = f.ops.TradeAmounts(ctx, identity.User{Role: "viewer"}, f.epoch, in); accounting.Code(err) != "FORBIDDEN" {
		t.Fatal(err)
	}
	if _, err = f.ops.TradeAmounts(ctx, user, "old-dataset", in); accounting.Code(err) != "DATASET_CHANGED" {
		t.Fatal(err)
	}
	in = tradeInput("invalid", "purchase", "922337203685477.5")
	in.Lines[0].Quantity = "10000"
	if _, err = f.ops.TradeAmounts(ctx, user, f.epoch, in); accounting.Code(err) != "AMOUNT_OVERFLOW" {
		t.Fatal(err)
	}
	in.Lines[0].VATRate = "0"
	in.Lines = append(in.Lines, in.Lines[0])
	if _, err = f.ops.TradeAmounts(ctx, user, f.epoch, in); accounting.Code(err) != "AMOUNT_OVERFLOW" {
		t.Fatal(err)
	}
	in = tradeInput("invalid", "purchase", "100")
	in.Lines[0].VATRate = "7"
	if _, err = f.ops.TradeAmounts(ctx, user, f.epoch, in); accounting.Code(err) != "INVALID_RATE" {
		t.Fatal(err)
	}
	in.Lines[0].VATRate, in.Lines[0].UnitPrice = "10", ""
	if _, err = f.ops.TradeAmounts(ctx, user, f.epoch, in); accounting.Code(err) != "INVALID_DECIMAL" {
		t.Fatal(err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f.store, f.ledger.Store = db, db
	in = tradeInput("reopened", "purchase", "100")
	quote, err = f.ops.TradeAmounts(ctx, user, f.epoch, in)
	if err != nil || quote.Amounts["total"] != "110" {
		t.Fatal(quote, err)
	}
}
