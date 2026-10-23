package operations

import (
	"context"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/platform/sqlite"
)

func TestItemClassificationCorrectionReplayAuditAndReopen(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	item := Item{ID: "rent", Code: "PDV", Name: "THUÊ NHÀ", Unit: "THÁNG", Tracked: true, Active: true}
	if _, err := f.ops.SaveItem(ctx, f.cmd("create-rent"), item); err != nil {
		t.Fatal(err)
	}
	item.Tracked = false
	first, err := f.ops.SaveItem(ctx, f.cmd("correct-rent"), item)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.ops.SaveItem(ctx, f.cmd("correct-rent"), item)
	if err != nil || string(first) != string(again) {
		t.Fatal("retry", err)
	}
	event := lastAudit(t, f, "item.save")
	undo, err := auditRestore(t, f, event, "undo-classification")
	if err != nil {
		t.Fatal(err)
	}
	var tracked bool
	if err = f.store.Read.QueryRow("SELECT tracked FROM items WHERE id='rent'").Scan(&tracked); err != nil || !tracked {
		t.Fatal("undo", tracked, err)
	}
	if _, err = auditRestore(t, f, undo, "redo-classification"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.store.Read.QueryRow("SELECT count(*) FROM items WHERE code='PDV'").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate", count, err)
	}
	archive, _, err := f.store.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Stage(ctx, archive); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Open(ctx, f.root)
	if err != nil {
		t.Fatal(err)
	}
	f.store = store
	f.ledger.Store = store
	t.Cleanup(func() { store.Close() })
	if err = store.Read.QueryRow("SELECT tracked FROM items WHERE id='rent'").Scan(&tracked); err != nil || tracked {
		t.Fatal("reopen", tracked, err)
	}
	if err = sqlite.Check(ctx, store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestItemClassificationLocksReferencedCatalog(t *testing.T) {
	for _, source := range []string{"voucher-draft", "posted-service", "stock-history", "opening-draft", "opening-adjustment-draft"} {
		t.Run(source, func(t *testing.T) {
			f := treasuryFixture(t)
			ctx := context.Background()
			item := Item{ID: "service", Code: "DV", Name: "Dịch vụ", Unit: "lần", Active: true}
			if source == "stock-history" || source == "opening-draft" || source == "opening-adjustment-draft" {
				item.Tracked = true
				if _, err := f.ops.SaveItem(ctx, f.cmd("classify-goods"), item); err != nil {
					t.Fatal(err)
				}
			}
			switch source {
			case "voucher-draft":
				in := tradeInput("draft-service", "purchase", "100")
				in.TradeMode = "service"
				if _, err := f.ops.SaveVoucher(ctx, f.cmd("save-service"), in); err != nil {
					t.Fatal(err)
				}
			case "posted-service":
				in := tradeInput("posted-service", "sale", "100")
				in.TradeMode = "service"
				executeTreasury(t, f, "post-service", in)
			case "stock-history":
				in := tradeInput("posted-goods", "purchase", "100")
				in.TradeMode = "goods"
				in.Lines[0].WarehouseID = "default"
				executeTreasury(t, f, "post-goods", in)
			case "opening-draft":
				in := openingInput()
				in.Stocks = []OpeningStock{{ItemID: item.ID, WarehouseID: "default", Quantity: "1", Value: "100"}}
				in.Lines = []OpeningLine{openingLine("411", "", "0", "100")}
				if _, err := f.ops.SaveOpening(ctx, f.cmd("opening-stock"), in); err != nil {
					t.Fatal(err)
				}
			case "opening-adjustment-draft":
				state := confirmInitial(t, f, openingInput())
				in := adjustmentInput(state, "catalog-opening-change", "1000")
				in.Input.Stocks = []OpeningStock{{ItemID: item.ID, WarehouseID: "default", Quantity: "1", Value: "100"}}
				in.Input.Lines = append(in.Input.Lines, openingLine("411", "", "0", "100"))
				saveAdjustment(t, f, "save-stock-adjustment", in)
			}
			original := item.Tracked
			item.Tracked = !item.Tracked
			if _, err := f.ops.SaveItem(ctx, f.cmd("reclassify-used"), item); accounting.Code(err) != "HISTORY_LOCKED" {
				t.Fatal("changed used item", err)
			}
			var actual bool
			if err := f.store.Read.QueryRow("SELECT tracked FROM items WHERE id=?", item.ID).Scan(&actual); err != nil || actual != original {
				t.Fatal(actual, err)
			}
			if err := sqlite.Check(ctx, f.store.Read); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSharedItemsPurchaseSaleAndServiceWithoutWarehouse(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	for _, kind := range []string{"purchase", "sale"} {
		in := tradeInput("shared-service-"+kind, kind, "100")
		in.TradeMode = "service"
		executeTreasury(t, f, "service-"+kind, in)
	}
	var count int
	if err := f.store.Read.QueryRow("SELECT count(*) FROM stock_movements").Scan(&count); err != nil || count != 0 {
		t.Fatal("service changed stock", count, err)
	}
	if _, err := f.ops.SaveItem(ctx, f.cmd("new-goods"), Item{ID: "goods", Code: "HH", Name: "Hàng dùng chung", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"purchase", "sale"} {
		in := tradeInput("shared-goods-"+kind, kind, "100")
		in.TradeMode = "goods"
		in.Lines[0].ItemID = "goods"
		in.Lines[0].WarehouseID = "default"
		executeTreasury(t, f, "goods-"+kind, in)
	}
	if err := f.store.Read.QueryRow("SELECT count(*) FROM items").Scan(&count); err != nil || count != 2 {
		t.Fatal("catalog copied", count, err)
	}
	if err := f.store.Read.QueryRow("SELECT count(*) FROM stock_movements WHERE item_id='goods'").Scan(&count); err != nil || count != 2 {
		t.Fatal("goods stock", count, err)
	}
	bad := tradeInput("wrong-type", "purchase", "100")
	bad.TradeMode = "service"
	bad.Lines[0].ItemID = "goods"
	if _, err := f.ops.Execute(ctx, f.cmd("wrong-type"), bad); accounting.Code(err) != "INVALID_ITEM" {
		t.Fatal(err)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}

func TestItemClassificationConcurrentPosting(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	in := tradeInput("concurrent-service", "sale", "100")
	in.TradeMode = "service"
	item := Item{ID: "service", Code: "DV", Name: "Dịch vụ", Unit: "lần", Tracked: true, Active: true}
	var wg sync.WaitGroup
	var classifyErr, postErr error
	wg.Add(2)
	go func() { defer wg.Done(); _, classifyErr = f.ops.SaveItem(ctx, f.cmd("concurrent-classify"), item) }()
	go func() { defer wg.Done(); _, postErr = f.ops.Execute(ctx, f.cmd("concurrent-post"), in) }()
	wg.Wait()
	if classifyErr == nil && postErr == nil {
		t.Fatal("both classification and posting succeeded")
	}
	if classifyErr != nil && accounting.Code(classifyErr) != "HISTORY_LOCKED" {
		t.Fatal(classifyErr)
	}
	if postErr != nil && accounting.Code(postErr) != "INVALID_ITEM" {
		t.Fatal(postErr)
	}
	var tracked bool
	if err := f.store.Read.QueryRow("SELECT tracked FROM items WHERE id='service'").Scan(&tracked); err != nil {
		t.Fatal(err)
	}
	var posted int
	if err := f.store.Read.QueryRow("SELECT count(*) FROM operations WHERE kind='sale'").Scan(&posted); err != nil {
		t.Fatal(err)
	}
	if tracked && posted != 0 || !tracked && posted != 1 {
		t.Fatal(tracked, posted)
	}
	if err := sqlite.Check(ctx, f.store.Read); err != nil {
		t.Fatal(err)
	}
}
