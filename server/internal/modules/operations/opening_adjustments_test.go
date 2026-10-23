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

func confirmInitial(t *testing.T, f *fixture, in OpeningInput) OpeningState {
	t.Helper()
	b, e := f.ops.SaveOpening(context.Background(), f.cmd("initial-save"), in)
	state := decodeOpening(t, b, e)
	b, e = f.ops.PostOpening(context.Background(), f.cmd("initial-confirm"), ledger.Post{ID: in.ID, Version: state.Document.Version})
	return decodeOpening(t, b, e)
}
func adjustmentInput(state OpeningState, id, amount string) OpeningAdjustmentInput {
	in := *state.Input
	in.ID = id
	in.Version = 0
	in.Lines = append([]OpeningLine(nil), in.Lines...)
	in.Lines[0].Debit = amount
	in.Lines[1].Credit = amount
	return OpeningAdjustmentInput{Input: in, BaseVersion: state.BalanceVersion, Reason: "Đối chiếu lại số dư giả lập"}
}
func saveAdjustment(t *testing.T, f *fixture, key string, in OpeningAdjustmentInput) OpeningState {
	t.Helper()
	b, e := f.ops.SaveOpeningAdjustment(context.Background(), f.cmd(key), in)
	return decodeOpening(t, b, e)
}
func confirmAdjustment(t *testing.T, f *fixture, id string) OpeningState {
	t.Helper()
	p, e := f.ops.OpeningAdjustment(context.Background(), id)
	if e != nil || len(p.Blockers) != 0 {
		t.Fatal(p.Blockers, e)
	}
	b, e := f.ops.ConfirmOpeningAdjustment(context.Background(), f.cmd("confirm-"+id), OpeningAdjustmentPost{ID: id, Version: p.Document.Version, Revision: p.Revision})
	return decodeOpening(t, b, e)
}
func TestOpeningAdjustmentVersionsReportsBanksAndRecovery(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	if _, e := f.ops.SaveBank(ctx, f.cmd("bank"), BankAccount{ID: "bank", Number: "000123", BankName: "Ngân hàng giả lập", AccountID: "112", Active: true}); e != nil {
		t.Fatal(e)
	}
	in := openingInput()
	in.Lines[0].AccountID = "112"
	in.Lines[0].BankAccountID = "bank"
	state := confirmInitial(t, f, in)
	// A real period movement remains separate from corrections.
	d := f.draft("movement")
	d.Lines = []ledger.Line{{AccountID: "111", Debit: "20", Credit: "0"}, {AccountID: "411", Debit: "0", Credit: "20"}}
	b, e := f.ledger.Save(ctx, f.cmd("movement-save"), d)
	doc := decodeDocument(t, b, e)
	if _, e = f.ledger.Post(ctx, f.cmd("movement-post"), ledger.Post{ID: doc.ID, Version: doc.Version}); e != nil {
		t.Fatal(e)
	}
	for i, amount := range []string{"9223372036854775807", "3000000"} {
		id := []string{"adjust-one", "adjust-two"}[i]
		draft := adjustmentInput(state, id, amount)
		saved := saveAdjustment(t, f, "save-"+id, draft)
		if saved.Adjustment == nil || saved.Document.ID != state.Document.ID {
			t.Fatal(saved)
		}
		if _, e = f.ledger.Post(ctx, f.cmd("generic-"+id), ledger.Post{ID: id, Version: 1}); accounting.Code(e) != "SUBLEDGER_REQUIRED" {
			t.Fatal(e)
		}
		if _, e = f.ledger.Save(ctx, f.cmd("hijack-"+id), ledger.Document{ID: id, Version: 1, Type: "journal", Date: in.Cutoff, Description: "hijack", Lines: inToLines(draft.Input.Lines)}); accounting.Code(e) != "SUBLEDGER_REQUIRED" {
			t.Fatal(e)
		}
		preview, e := f.ops.OpeningAdjustment(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		post := OpeningAdjustmentPost{ID: id, Version: preview.Document.Version, Revision: preview.Revision}
		b, e = f.ops.ConfirmOpeningAdjustment(ctx, f.cmd("confirm-"+id), post)
		state = decodeOpening(t, b, e)
		retry, e := f.ops.ConfirmOpeningAdjustment(ctx, f.cmd("confirm-"+id), post)
		if e != nil || string(retry) != string(b) {
			t.Fatal(e)
		}
		if state.BalanceVersion != i+1 || state.Input.Lines[0].Debit != amount || state.Adjustment != nil {
			t.Fatal(state)
		}
		report, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "trial", PageSize: 200})
		if e != nil || report.OpeningDebit != amount || report.Debit != "20" {
			t.Fatal(report, e)
		}
		next, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-11", Kind: "trial", PageSize: 200})
		if e != nil || next.Debit != "0" {
			t.Fatal(next, e)
		}
		bank, e := f.ops.Banks(ctx, "2026-10")
		if e != nil || bank.Accounts[0].Opening != amount || bank.Accounts[0].Receipts != "0" || bank.Accounts[0].Payments != "0" {
			t.Fatal(bank, e)
		}
		financial, e := f.ledger.FinancialSource(ctx, "2026-10-01", "2026-10-31")
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range financial.Rows {
			if r.Code == "112" && (r.OpeningDebit != amount || r.Debit != "0" || r.Credit != "0") {
				t.Fatal(r)
			}
		}
		if e = sqlite.Check(ctx, f.store.Read); e != nil {
			t.Fatal(e)
		}
	}
	original, e := ledger.ReadDocument(ctx, f.store.Read, in.ID)
	if e != nil || original.Lines[0].Debit != "9007199254740993" {
		t.Fatal(original, e)
	}
	if _, e = f.store.Write.Exec("UPDATE opening_adjustments SET reason='changed' WHERE confirmed_version=1"); e == nil {
		t.Fatal("mutable confirmed correction")
	}
	journal, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "journal", PageSize: 200})
	if e != nil || journal.Total != 2 {
		t.Fatal(journal, e)
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
	state, e = f.ops.Opening(ctx)
	if e != nil || state.BalanceVersion != 2 || state.Input.Lines[0].Debit != "3000000" {
		t.Fatal(state, e)
	}
	history, e := f.ops.OpeningHistory(ctx, 1, 20)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = json.Marshal(history); e != nil {
		t.Fatal(e)
	}
}
func TestOpeningAdjustmentConflictsRollbackAndClosedPeriods(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	state := confirmInitial(t, f, openingInput())
	draft := adjustmentInput(state, "adjust", "5000000")
	saved := saveAdjustment(t, f, "save", draft)
	if _, e := f.ops.SaveOpeningAdjustment(ctx, f.cmd("stale"), draft); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
	preview, e := f.ops.OpeningAdjustment(ctx, "adjust")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("CREATE TRIGGER fail_adjustment BEFORE UPDATE OF status ON documents WHEN NEW.id='adjust' AND NEW.status='posted' BEGIN SELECT RAISE(ABORT,'simulated failure');END"); e != nil {
		t.Fatal(e)
	}
	post := OpeningAdjustmentPost{ID: "adjust", Version: preview.Document.Version, Revision: preview.Revision}
	if _, e = f.ops.ConfirmOpeningAdjustment(ctx, f.cmd("fail"), post); e == nil {
		t.Fatal("expected rollback")
	}
	var n int
	if e = f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE source_type='opening_adjustment_reversal'").Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if _, e = f.store.Write.Exec("DROP TRIGGER fail_adjustment"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("INSERT INTO periods VALUES('2026-11',1)"); e != nil {
		t.Fatal(e)
	}
	p, e := f.ops.OpeningAdjustment(ctx, "adjust")
	if e != nil || len(p.Blockers) == 0 || p.Blockers[0].Code != "PERIOD_CLOSED" {
		t.Fatal(p, e)
	}
	draft.Input = saved.Adjustment.Input
	draft.Input.Lines[0].Debit = "6000000"
	draft.Input.Lines[1].Credit = "6000000"
	saved = saveAdjustment(t, f, "save-closed", draft)
	if _, e = f.ops.ConfirmOpeningAdjustment(ctx, f.cmd("stale-preview"), post); accounting.Code(e) != "STALE_PREVIEW" {
		t.Fatal(e)
	}
	p, e = f.ops.OpeningAdjustment(ctx, "adjust")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.ops.ConfirmOpeningAdjustment(ctx, f.cmd("closed"), OpeningAdjustmentPost{ID: "adjust", Version: p.Document.Version, Revision: p.Revision}); accounting.Code(e) != "PERIOD_CLOSED" {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("UPDATE periods SET closed=0 WHERE period='2026-11'"); e != nil {
		t.Fatal(e)
	}
	p, e = f.ops.OpeningAdjustment(ctx, "adjust")
	if e != nil {
		t.Fatal(e)
	}
	post = OpeningAdjustmentPost{ID: "adjust", Version: p.Document.Version, Revision: p.Revision}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, key := range []string{"race-one", "race-two"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, e := f.ops.ConfirmOpeningAdjustment(ctx, f.cmd(k), post)
			errs <- e
		}(key)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if accounting.Code(e) != "STALE_PREVIEW" && accounting.Code(e) != "VERSION_CONFLICT" {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}
func TestOpeningAdjustmentStockAssetsAndDependencyIsolation(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	f.policy(t, testPolicy())
	if _, e := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng giả lập", Unit: "Cái", Tracked: true, Active: true}); e != nil {
		t.Fatal(e)
	}
	in := OpeningInput{ID: "opening", Cutoff: "2026-10-01", Source: "Giả lập", Lines: []OpeningLine{openingLine("111", "", "500", "0"), openingLine("411", "", "0", "2401")}, Stocks: []OpeningStock{{ItemID: "goods", WarehouseID: "default", Quantity: "2.50", Value: "1001"}}, Assets: []Asset{{ID: "asset", Code: "TS", Name: "Tài sản", Kind: "fixed", Cost: "1200", Allocated: "300", Residual: "0", Months: 12, StartPeriod: "2026-07"}}}
	state := confirmInitial(t, f, in)
	draft := adjustmentInput(state, "adjust-subledger", "500")
	draft.Input.Stocks = []OpeningStock{{ItemID: "goods", WarehouseID: "default", Quantity: "3.00", Value: "1200"}}
	draft.Input.Assets = append([]Asset(nil), state.Input.Assets...)
	draft.Input.Assets[0].Cost = "2400"
	draft.Input.Assets[0].Allocated = "600"
	draft.Input.Lines[1].Credit = "3500"
	saveAdjustment(t, f, "subledger-save", draft)
	state = confirmAdjustment(t, f, draft.Input.ID)
	if e := sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ops.Execute(ctx, f.cmd("dep"), Input{Kind: "depreciation", SourceID: "asset:2026-10", Date: "2026-10-31", Description: "Khấu hao", AssetID: "asset"}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ops.Execute(ctx, f.cmd("sale"), Input{Kind: "sale", SourceID: "sale", Date: "2026-10-31", Description: "Bán", PartnerID: "customer", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "goods", WarehouseID: "default", Quantity: "1.00", UnitPrice: "1000", VATRate: "0"}}}); e != nil {
		t.Fatal(e)
	}
	draft = adjustmentInput(state, "money-only", "600")
	draft.Input.Lines[1].Credit = "3600"
	draft.Input.Assets = append([]Asset(nil), state.Input.Assets...)
	draft.Input.Assets[0].Name = "Tên đã sửa"
	saveAdjustment(t, f, "money-save", draft)
	state = confirmAdjustment(t, f, draft.Input.ID)
	if e := sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	draft = adjustmentInput(state, "blocked", "600")
	draft.Input.Lines[1].Credit = "3800"
	draft.Input.Stocks = []OpeningStock{{ItemID: "goods", WarehouseID: "default", Quantity: "4.00", Value: "1300"}}
	draft.Input.Assets = append([]Asset(nil), state.Input.Assets...)
	draft.Input.Assets[0].Cost = "2500"
	saveAdjustment(t, f, "blocked-save", draft)
	p, e := f.ops.OpeningAdjustment(ctx, draft.Input.ID)
	if e != nil {
		t.Fatal(e)
	}
	codes := map[string]bool{}
	for _, b := range p.Blockers {
		codes[b.Code] = true
		if b.DocumentID == "" {
			t.Fatal(b)
		}
	}
	if !codes["OPENING_STOCK_USED"] || !codes["OPENING_ASSET_USED"] {
		t.Fatal(p.Blockers)
	}
	if _, e = f.ops.ConfirmOpeningAdjustment(ctx, f.cmd("blocked-confirm"), OpeningAdjustmentPost{ID: draft.Input.ID, Version: p.Document.Version, Revision: p.Revision}); accounting.Code(e) != "OPENING_STOCK_USED" {
		t.Fatal(e)
	}
	if _, e = f.ops.DiscardOpeningAdjustment(ctx, f.cmd("discard"), ledger.Post{ID: draft.Input.ID, Version: p.Document.Version}); e != nil {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}

func TestOpeningAdjustmentHistoricalInactiveReferencesAndDraftDiscard(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	state := confirmInitial(t, f, openingInput())
	if _, e := f.store.Write.Exec("UPDATE accounts SET active=0 WHERE id='111'"); e != nil {
		t.Fatal(e)
	}
	draft := adjustmentInput(state, "replace-inactive", "5000000")
	draft.Input.Lines[0].AccountID = "113"
	saved := saveAdjustment(t, f, "save-inactive", draft)
	if _, e := f.ops.SaveOpeningAdjustment(ctx, ledger.Command{Actor: f.actor, Epoch: "stale", Key: "stale-epoch"}, draft); accounting.Code(e) != "DATASET_CHANGED" {
		t.Fatal(e)
	}
	if _, e := f.store.Write.Exec("UPDATE users SET role='accountant' WHERE id=?", f.actor); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ops.DiscardOpeningAdjustment(ctx, f.cmd("not-owner"), ledger.Post{ID: draft.Input.ID, Version: saved.Adjustment.Input.Version}); accounting.Code(e) != "FORBIDDEN" {
		t.Fatal(e)
	}
	if _, e := f.store.Write.Exec("UPDATE users SET role='owner' WHERE id=?", f.actor); e != nil {
		t.Fatal(e)
	}
	state = confirmAdjustment(t, f, draft.Input.ID)
	if e := sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	next := adjustmentInput(state, "discard", "6000000")
	saved = saveAdjustment(t, f, "save-discard", next)
	if _, e := f.ops.DiscardOpeningAdjustment(ctx, f.cmd("discard-stale"), ledger.Post{ID: next.Input.ID, Version: 0}); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
	if _, e := f.ops.DiscardOpeningAdjustment(ctx, f.cmd("discard"), ledger.Post{ID: next.Input.ID, Version: saved.Adjustment.Input.Version}); e != nil {
		t.Fatal(e)
	}
	if e := sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}
func TestOpeningAdjustmentAddAndRemoveStockAndAsset(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	f.policy(t, testPolicy())
	if _, e := f.ops.SaveItem(ctx, f.cmd("goods"), Item{ID: "goods", Code: "HH", Name: "Hàng giả lập", Unit: "Cái", Tracked: true, Active: true}); e != nil {
		t.Fatal(e)
	}
	state := confirmInitial(t, f, openingInput())
	draft := adjustmentInput(state, "add-details", "500")
	draft.Input.Stocks = []OpeningStock{{ItemID: "goods", WarehouseID: "default", Quantity: "1.00", Value: "100"}}
	draft.Input.Assets = []Asset{{ID: "asset", Code: "TS", Name: "Tài sản", Kind: "fixed", Cost: "1200", Allocated: "300", Residual: "0", Months: 12, StartPeriod: "2026-07"}}
	draft.Input.Lines[1].Credit = "1500"
	saveAdjustment(t, f, "save-add", draft)
	state = confirmAdjustment(t, f, draft.Input.ID)
	if e := sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	draft = adjustmentInput(state, "remove-details", "1000")
	draft.Input.Stocks = []OpeningStock{}
	draft.Input.Assets = []Asset{}
	saveAdjustment(t, f, "save-remove", draft)
	state = confirmAdjustment(t, f, draft.Input.ID)
	if e := sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	automation, e := f.ops.Data(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(automation.Assets) != 0 {
		t.Fatal(automation.Assets)
	}
	if _, e = f.ops.Execute(ctx, f.cmd("removed-asset"), Input{Kind: "depreciation", SourceID: "asset:2026-10", Date: "2026-10-31", Description: "Khấu hao", AssetID: "asset"}); accounting.Code(e) != "NOT_FOUND" {
		t.Fatal(e)
	}
	archive, _, e := f.store.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Stage(ctx, archive); e != nil {
		t.Fatal(e)
	}
}
