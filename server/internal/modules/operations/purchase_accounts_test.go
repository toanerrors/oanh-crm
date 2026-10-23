package operations

import (
	"context"
	"encoding/json"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestPurchaseSelectedAccountDraftPostingRetryAndReopen(t *testing.T) {
	for _, tc := range []struct {
		name, mode, account, expected string
		stock                         int
	}{
		{"goods-default", "goods", "", "156", 1},
		{"goods-stock", "goods", "156", "156", 1},
		{"goods-expense", "goods", "6427", "6427", 0},
		{"goods-production", "goods", "621", "621", 0},
		{"goods-wip", "goods", "154", "154", 0},
		{"service-expense", "service", "6427", "6427", 0},
		{"service-wip", "service", "154", "154", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := treasuryFixture(t)
			ctx := context.Background()
			in := tradeInput(tc.name, "purchase", "900719925474099.3")
			in.TradeMode = tc.mode
			in.Lines[0].Quantity = "10"
			in.Lines[0].AccountID = tc.account
			in.Lines[0].VATRate = "0"
			if tc.mode == "goods" {
				if _, err := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
					t.Fatal(err)
				}
				in.Lines[0].ItemID = "goods"
			}
			b, err := f.ops.SaveVoucher(ctx, f.cmd("save"), in)
			if err != nil {
				t.Fatal(err)
			}
			var saved Voucher
			if err = json.Unmarshal(b, &saved); err != nil {
				t.Fatal(err)
			}
			var count int
			if err = f.store.Read.QueryRow("SELECT count(*) FROM stock_movements").Scan(&count); err != nil || count != 0 {
				t.Fatal("draft moved stock", count, err)
			}
			loaded, err := f.ops.Voucher(ctx, saved.Document.ID)
			if err != nil || loaded.Input.Lines[0].AccountID != tc.account {
				t.Fatal(loaded, err)
			}
			preview, err := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, loaded.Input)
			if err != nil || preview.Document.Lines[0].AccountID != tc.expected || preview.Document.Lines[0].Debit != "9007199254740993" {
				t.Fatal(preview, err)
			}
			first, err := f.ops.Execute(ctx, f.cmd("post"), loaded.Input)
			if err != nil {
				t.Fatal(err)
			}
			again, err := f.ops.Execute(ctx, f.cmd("post"), loaded.Input)
			if err != nil || string(first) != string(again) {
				t.Fatal("retry differs", err)
			}
			loaded.Input.Lines[0].AccountID = "6428"
			if _, err = f.ops.Execute(ctx, f.cmd("post"), loaded.Input); accounting.Code(err) != "IDEMPOTENCY_CONFLICT" {
				t.Fatal(err)
			}
			if err = f.store.Read.QueryRow("SELECT count(*) FROM stock_movements").Scan(&count); err != nil || count != tc.stock {
				t.Fatal("unexpected stock movement", count, err)
			}
			if err = sqlite.Check(ctx, f.store.Read); err != nil {
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
			loaded, err = f.ops.Voucher(ctx, saved.Document.ID)
			if err != nil || loaded.Document.Status != "posted" || loaded.Document.Lines[0].AccountID != tc.expected || loaded.Document.Lines[0].Debit != "9007199254740993" {
				t.Fatal(loaded, err)
			}
		})
	}
}

func TestPurchaseInvalidAccountRollsBackStock(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	if _, err := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	in := tradeInput("invalid-account", "purchase", "100")
	in.TradeMode = "goods"
	in.Lines[0].ItemID = "goods"
	in.Lines = append(in.Lines, in.Lines[0])
	if _, err := f.ledger.SaveAccount(ctx, f.cmd("disable-expense"), ledger.Account{ID: "6428", Code: "6428", Name: "Chi phí bằng tiền khác", Group: "expense", Active: false}); err != nil {
		t.Fatal(err)
	}
	for _, account := range []string{"242", "111", "511", "missing", "6428"} {
		in.Lines[1].AccountID = account
		if _, err := f.ops.Execute(ctx, f.cmd("invalid-"+account), in); accounting.Code(err) != "INVALID_ACCOUNT" {
			t.Fatal(account, err)
		}
		for _, table := range []string{"documents", "stock_lots", "stock_movements", "operations", "counters"} {
			var count int
			if err := f.store.Read.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
				t.Fatal("rollback", table, count, err)
			}
		}
	}
}

func TestPurchaseMixedInventoryAndExpense(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	if _, err := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	in := tradeInput("mixed-accounts", "purchase", "1000")
	in.TradeMode = "goods"
	in.Lines[0].ItemID, in.Lines[0].AccountID = "goods", "156"
	in.Lines = append(in.Lines, in.Lines[0])
	in.Lines[1].AccountID = "6427"
	r := executeTreasury(t, f, "mixed", in)
	if r.Document.Lines[0].AccountID != "156" || r.Document.Lines[3].AccountID != "6427" || r.Amounts["total"] != "2200" {
		t.Fatal(r)
	}
	var value int64
	if err := f.store.Read.QueryRow("SELECT SUM(value) FROM stock_lots").Scan(&value); err != nil || value != 1000 {
		t.Fatal("expense entered stock", value, err)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestPurchaseExpenseRejectsStockCostAllocation(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	source := tradeInput("freight", "purchase", "101")
	source.TradeMode, source.IsPurchaseCost = "service", true
	cost := executeTreasury(t, f, "freight", source)
	if _, err := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	in := tradeInput("expense-with-stock-cost", "purchase", "1000")
	in.TradeMode = "goods"
	in.Lines[0].ItemID, in.Lines[0].AccountID, in.Lines[0].PurchaseCost = "goods", "6427", "101"
	in.PurchaseCosts = []PurchaseCost{{DocumentID: cost.Document.ID, Line: 0, Amount: "101"}}
	if _, err := f.ops.Execute(ctx, f.cmd("invalid-cost"), in); accounting.Code(err) != "INVALID_COST_ALLOCATION" {
		t.Fatal(err)
	}
	sources, err := f.ops.PurchaseCostSources(ctx, in.Date)
	if err != nil || len(sources) != 1 || sources[0].Remaining != "101" {
		t.Fatal(sources, err)
	}
	if err = sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
