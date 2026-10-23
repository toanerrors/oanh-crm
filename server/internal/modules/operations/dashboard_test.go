package operations

import (
	"context"
	"testing"
)

func TestDashboardInventoryAndSupplierBalanceMatchPostedPurchase(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	if _, err := f.ops.SaveItem(ctx, f.cmd("dashboard-stock-item"), Item{ID: "dashboard-item", Code: "DASH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	_, err := f.ops.Execute(ctx, f.cmd("dashboard-purchase"), Input{Kind: "purchase", SourceID: "dashboard-purchase", Date: "2026-10-15", Description: "Hàng nhập kiểm thử", PartnerID: "supplier", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "dashboard-item", Quantity: "2", UnitPrice: "100", VATRate: "0", Deductible: new(true)}}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.ledger.Snapshot(ctx, "owner", "owner", "2026-10")
	if err != nil || snapshot.Server.InventoryBalance != "200" || snapshot.Server.Payable != "200" || snapshot.Server.DraftCount != 0 {
		t.Fatal(snapshot.Server, err)
	}
}

func TestDashboardOpeningAdjustmentsStayOutsideBusinessActivity(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	state := confirmInitial(t, f, openingInput())
	input := adjustmentInput(state, "dashboard-opening-change", "2000")
	saveAdjustment(t, f, "dashboard-opening-draft", input)
	snapshot, err := f.ledger.Snapshot(ctx, "owner", "owner", "2026-10")
	if err != nil || snapshot.Server.DocumentCount != 0 || len(snapshot.Documents) != 0 {
		t.Fatal(snapshot.Server, len(snapshot.Documents), err)
	}
	confirmAdjustment(t, f, input.Input.ID)
	snapshot, err = f.ledger.Snapshot(ctx, "owner", "owner", "2026-10")
	if err != nil || snapshot.Server.DocumentCount != 0 || len(snapshot.Documents) != 0 || snapshot.Server.MoneyBalance != "2000" {
		t.Fatal(snapshot.Server, len(snapshot.Documents), err)
	}
}
