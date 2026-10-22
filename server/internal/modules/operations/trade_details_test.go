package operations

import (
	"context"
	"encoding/json"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/platform/sqlite"
	"testing"
)

func TestDiscountCalculationAndAllocation(t *testing.T) {
	f := treasuryFixture(t)
	cases := []struct {
		name, mode, price, base, tax, total, discount string
		d                                             Discount
	}{
		{"exclusive", "exclusive", "1001", "901", "90", "991", "100", Discount{Mode: "percent", Rate: "10"}},
		{"inclusive", "inclusive", "1100", "909", "91", "1000", "100", Discount{Mode: "line"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tradeInput(tc.name, "sale", tc.price)
			in.PriceMode = tc.mode
			in.Discount = tc.d
			if tc.d.Mode == "line" {
				in.Lines[0].DiscountAmount = "100"
			}
			r := executeTreasury(t, f, tc.name, in)
			if r.Amounts["base"] != tc.base || r.Amounts["tax"] != tc.tax || r.Amounts["total"] != tc.total || r.Amounts["discount"] != tc.discount {
				t.Fatal(r.Amounts)
			}
		})
	}
	in := tradeInput("remainder", "sale", "100")
	in.Lines = append(in.Lines, in.Lines[0], in.Lines[0])
	in.Discount = Discount{Mode: "total", Amount: "2", Allocation: "value"}
	r := executeTreasury(t, f, "remainder", in)
	if r.LineAmounts[0].Discount != "1" || r.LineAmounts[1].Discount != "1" || r.LineAmounts[2].Discount != "0" || r.Amounts["discount"] != "2" {
		t.Fatal(r)
	}
	in.SourceID = "manual-mismatch"
	in.Discount.Allocation = "manual"
	in.Lines[0].DiscountAmount = "1"
	if _, e := f.ops.Execute(context.Background(), f.cmd("mismatch"), in); accounting.Code(e) != "ALLOCATION_MISMATCH" {
		t.Fatal(e)
	}
	in.SourceID = "too-much"
	in.Discount = Discount{Mode: "percent", Rate: "101"}
	in.Lines[0].DiscountAmount = ""
	if _, e := f.ops.Execute(context.Background(), f.cmd("too-much"), in); accounting.Code(e) != "INVALID_RATE" {
		t.Fatal(e)
	}
	weights := []TradeLine{{Quantity: "100", UnitPrice: "1"}, {Quantity: "1", UnitPrice: "100"}}
	in.Lines = weights
	in.Discount = Discount{Mode: "total", Amount: "150", Allocation: "quantity"}
	if _, e := tradeDiscounts(in, testPolicy()); accounting.Code(e) != "DISCOUNT_EXCEEDS_VALUE" {
		t.Fatal(e)
	}
}
func TestPurchaseCostsDraftBoundsReclassificationAndRecovery(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	source := tradeInput("freight", "purchase", "101")
	source.TradeMode = "service"
	source.IsPurchaseCost = true
	source.Lines[0].AccountID = "6427"
	cost := executeTreasury(t, f, "freight", source)
	if _, e := f.ops.Execute(ctx, f.cmd("close-pending"), Input{Kind: "closing", Date: "2026-10-31", SourceID: "close-pending", Description: "Kết chuyển"}); accounting.Code(e) != "UNALLOCATED_PURCHASE_COST" {
		t.Fatal(e)
	}
	if _, e := f.ops.SaveItem(ctx, f.cmd("goods-cost"), Item{ID: "goods", Code: "HH", Name: "Hàng", Unit: "cái", Tracked: true, Active: true}); e != nil {
		t.Fatal(e)
	}
	in := tradeInput("with-cost", "purchase", "1000")
	in.TradeMode = "goods"
	in.Lines[0].ItemID = "goods"
	in.Lines[0].PurchaseCost = "101"
	in.PurchaseCosts = []PurchaseCost{{DocumentID: cost.Document.ID, Line: 0, Amount: "101"}}
	b, e := f.ops.SaveVoucher(ctx, f.cmd("draft-cost"), in)
	if e != nil {
		t.Fatal(e)
	}
	var v Voucher
	json.Unmarshal(b, &v)
	list, e := f.ops.PurchaseCostSources(ctx, in.Date)
	if e != nil || list[0].Remaining != "101" {
		t.Fatal(list, e)
	}
	in = v.Input
	r := executeTreasury(t, f, "post-cost", in)
	if r.Amounts["total"] != "1100" || r.Amounts["purchaseCost"] != "101" {
		t.Fatal(r)
	}
	var value int64
	if e = f.store.Read.QueryRow("SELECT value FROM stock_lots WHERE document_id=?", r.Document.ID).Scan(&value); e != nil || value != 1101 {
		t.Fatal(value, e)
	}
	in.DraftID = ""
	in.DraftVersion = 0
	in.SourceID = "again"
	if _, e = f.ops.Execute(ctx, f.cmd("again"), in); accounting.Code(e) != "COST_OVERALLOCATED" {
		t.Fatal(e)
	}
	list, e = f.ops.PurchaseCostSources(ctx, in.Date)
	if e != nil || list[0].Remaining != "0" {
		t.Fatal(list, e)
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
}
func TestWarehouseCostIsolationAndBackup(t *testing.T) {
	for _, method := range []string{"fifo", "moving_average"} {
		t.Run(method, func(t *testing.T) {
			f := openFixture(t)
			ctx := context.Background()
			p := testPolicy()
			p.InventoryMethod = method
			f.policy(t, p)
			if _, e := f.ops.SaveWarehouse(ctx, f.cmd("warehouse"), Warehouse{ID: "second", Code: "KHO2", Name: "Kho thứ hai", Active: true}); e != nil {
				t.Fatal(e)
			}
			if _, e := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng", Unit: "cái", Tracked: true, Active: true}); e != nil {
				t.Fatal(e)
			}
			buy := tradeInput("buy-default", "purchase", "100")
			buy.TradeMode = "goods"
			buy.Lines[0].ItemID = "goods"
			buy.Lines[0].Quantity = "2"
			executeTreasury(t, f, "buy-default", buy)
			buy.SourceID = "buy-second"
			buy.Lines[0].WarehouseID = "second"
			buy.Lines[0].UnitPrice = "1000"
			executeTreasury(t, f, "buy-second", buy)
			sale := tradeInput("sell-second", "sale", "2000")
			sale.TradeMode = "goods"
			sale.Lines[0].ItemID = "goods"
			sale.Lines[0].WarehouseID = "second"
			r := executeTreasury(t, f, "sell-second", sale)
			if r.Amounts["cost"] != "1000" {
				t.Fatal(r.Amounts)
			}
			sale.SourceID = "sell-default"
			sale.Lines[0].WarehouseID = "default"
			sale.Lines[0].Quantity = "2"
			r = executeTreasury(t, f, "sell-default", sale)
			if r.Amounts["cost"] != "200" {
				t.Fatal(r.Amounts)
			}
			sale.SourceID = "no-stock"
			sale.Lines[0].Quantity = "1"
			if _, e := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, sale); accounting.Code(e) != "INSUFFICIENT_STOCK" {
				t.Fatal(e)
			}
			if e := sqlite.Check(ctx, f.store.Read); e != nil {
				t.Fatal(e)
			}
			b, _, e := f.store.Backup(ctx)
			if e != nil {
				t.Fatal(e)
			}
			stage, e := f.store.Stage(ctx, b)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.store.Activate(ctx, stage.ID); e != nil {
				t.Fatal(e)
			}
			stocks, e := f.ops.Warehouses(ctx)
			if e != nil {
				t.Fatal(e)
			}
			for _, stock := range stocks.Stock {
				if stock.WarehouseID == "second" && stock.Value != "1000" {
					t.Fatal(stock)
				}
			}
			if _, e = f.store.Write.Exec("UPDATE stock_movement_warehouses SET warehouse_id='second' WHERE warehouse_id='default'"); e == nil {
				t.Fatal("posted warehouse was changed")
			}
		})
	}
}

func TestAutomaticPurchaseCostAllocation(t *testing.T) {
	for _, method := range []string{"quantity", "value", "percent"} {
		t.Run(method, func(t *testing.T) {
			f := treasuryFixture(t)
			ctx := context.Background()
			source := tradeInput("cost-auto", "purchase", "101")
			source.TradeMode = "service"
			source.IsPurchaseCost = true
			cost := executeTreasury(t, f, "cost-auto", source)
			if _, e := f.ops.SaveItem(ctx, f.cmd("goods-auto"), Item{ID: "goods", Code: "HH", Name: "Hàng", Unit: "cái", Tracked: true, Active: true}); e != nil {
				t.Fatal(e)
			}
			in := tradeInput("goods-auto", "purchase", "1000")
			in.TradeMode = "goods"
			in.CostAllocation = method
			in.Lines[0].ItemID = "goods"
			in.Lines = append(in.Lines, in.Lines[0])
			in.PurchaseCosts = []PurchaseCost{{DocumentID: cost.Document.ID, Line: 0, Amount: "101"}}
			switch method {
			case "quantity":
				in.Lines[1].Quantity = "2"
			case "value":
				in.Lines[1].UnitPrice = "2000"
			case "percent":
				in.Lines[0].CostPercent = "33.333333"
				in.Lines[1].CostPercent = "66.666667"
			}
			r := executeTreasury(t, f, "goods-auto", in)
			if r.LineAmounts[0].PurchaseCost != "34" || r.LineAmounts[1].PurchaseCost != "67" {
				t.Fatal(r.LineAmounts)
			}
			if e := sqlite.Check(ctx, f.store.Read); e != nil {
				t.Fatal(e)
			}
			if _, _, e := f.store.Backup(ctx); e != nil {
				t.Fatal(e)
			}
		})
	}
	in := tradeInput("large-discount", "sale", "900719925474099.3")
	in.Lines[0].Quantity = "10"
	in.Discount = Discount{Mode: "percent", Rate: "10"}
	v, e := tradeDiscounts(in, testPolicy())
	if e != nil || v[0] != 900719925474099 {
		t.Fatal(v, e)
	}
}
