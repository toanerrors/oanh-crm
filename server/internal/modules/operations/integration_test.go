package operations

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/identity"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

type fixture struct {
	store              *sqlite.Store
	ledger             *ledger.Service
	ops                *Service
	actor, epoch, root string
}

func TestBackupRestoreAttachmentsAndEpoch(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	d := f.save(t, "backed-up")
	if _, e := f.ledger.Post(ctx, f.cmd("post-backed"), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
		t.Fatal(e)
	}
	content := []byte("Bằng chứng giả lập")
	hash := sqlite.Hash(content)
	if e := sqlite.WriteFile(filepath.Join(f.store.Dataset, "attachments", hash), content); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.Write.Exec("INSERT INTO attachments VALUES('attachment',?,?,?,?)", d.ID, hash, "test.txt", len(content)); e != nil {
		t.Fatal(e)
	}
	archive, manifest, e := f.store.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if manifest.Posted != 1 || manifest.Debit != "9007199254740993" {
		t.Fatal(manifest)
	}
	stage, e := f.store.Stage(ctx, archive)
	if e != nil {
		t.Fatal(e)
	}
	f.save(t, "after-backup")
	if _, e = f.store.Activate(ctx, stage.ID); e != nil {
		t.Fatal(e)
	}
	metadata, e := ledger.ReadContext(ctx, f.store.Read)
	if e != nil || metadata.Epoch == f.epoch {
		t.Fatal(metadata, e)
	}
	if _, e = ledger.ReadDocument(ctx, f.store.Read, "after-backup"); accounting.Code(e) != "NOT_FOUND" {
		t.Fatal(e)
	}
	if _, e = f.ledger.Save(ctx, f.cmd("old-session"), f.draft("stale")); accounting.Code(e) != "DATASET_CHANGED" {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(f.store.Dataset, "attachments", hash))
	if e != nil || string(got) != string(content) {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(filepath.Join(f.root, "recovery"))
	if e != nil || len(entries) != 1 {
		t.Fatal(entries, e)
	}
	journal, e := os.ReadFile(filepath.Join(f.root, "recovery", entries[0].Name()))
	if e != nil || !strings.Contains(string(journal), `"phase":"activated"`) {
		t.Fatal(string(journal), e)
	}
}

func TestServerReportsPaginationAndWholeFilterTotals(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	for i := range 12 {
		d := f.save(t, fmt.Sprintf("report-%02d", i))
		if _, e := f.ledger.Post(ctx, f.cmd(fmt.Sprintf("post-%d", i)), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
			t.Fatal(e)
		}
	}
	list, e := f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Page: 2, PageSize: 5, Desc: true})
	if e != nil || len(list.Documents) != 5 || list.Total != 12 || list.Amount != "108086391056891916" {
		t.Fatal(list, e)
	}
	report, e := f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "journal", Page: 2, PageSize: 5})
	if e != nil || len(report.Rows) != 5 || report.Total != 24 || report.Debit != list.Amount || report.Credit != list.Amount {
		t.Fatal(report, e)
	}
	report, e = f.ledger.Report(ctx, ledger.Filter{Period: "2026-10", Kind: "trial", Page: 1, PageSize: 100})
	if e != nil || report.ClosingDebit != report.ClosingCredit {
		t.Fatal(report, e)
	}
	stale := list.Revision - 1
	if _, e = f.ledger.Documents(ctx, ledger.Filter{Period: "2026-10", Revision: &stale}); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
}

func TestImportPreviewAtomicityAndRetry(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	csv := "sourceType,sourceId,type,date,description,partnerId,accountId,linePartnerId,lineDescription,debit,credit\nmanual,SRC1,journal,2026-10-21,Nhập CSV,,111,,,100,0\nmanual,SRC1,journal,2026-10-21,Nhập CSV,,411,,,0,100\n"
	preview, e := f.ledger.PreviewImport(ctx, f.cmd("preview"), ledger.ImportInput{CSV: csv})
	if e != nil || len(preview.Documents) != 1 {
		t.Fatal(e)
	}
	var n int
	if e = f.store.Read.QueryRow("SELECT count(*) FROM documents").Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	input := ledger.ImportInput{CSV: csv, ExpectedRevision: &preview.Revision}
	result, e := f.ledger.Import(ctx, f.cmd("import"), input)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.ledger.Import(ctx, f.cmd("import"), input)
	if e != nil || string(again) != string(result) {
		t.Fatal(e)
	}
	bad := strings.ReplaceAll(csv, "SRC1", "SRC2") + "manual,SRC3,journal,2026-10-21,Thiếu tài khoản,,missing,,,100,0\n"
	c, e := ledger.ReadContext(ctx, f.store.Read)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.ledger.Import(ctx, f.cmd("bad-import"), ledger.ImportInput{CSV: bad, ExpectedRevision: &c.Revision}); e == nil {
		t.Fatal("invalid batch accepted")
	}
	if e = f.store.Read.QueryRow("SELECT count(*) FROM documents").Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}

func TestCorporateTaxAndClosing(t *testing.T) {
	f := openFixture(t)
	f.policy(t, testPolicy())
	ctx := context.Background()
	for _, input := range []struct {
		id, account string
		amount      string
		income      bool
	}{{"income", "511", "1000", true}, {"expense", "642", "400", false}} {
		d := f.draft(input.id)
		d.Date = "2026-10-30"
		d.Lines = []ledger.Line{{AccountID: "111", Debit: input.amount, Credit: "0"}, {AccountID: input.account, Debit: "0", Credit: input.amount}}
		if !input.income {
			for i := range d.Lines {
				d.Lines[i].Debit, d.Lines[i].Credit = d.Lines[i].Credit, d.Lines[i].Debit
			}
		}
		b, e := f.ledger.Save(ctx, f.cmd(input.id), d)
		d = decodeDocument(t, b, e)
		if _, e = f.ledger.Post(ctx, f.cmd("post-"+input.id), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
			t.Fatal(e)
		}
	}
	in := Input{Kind: "corporate_tax", SourceID: "TNDN", TaxFrom: "2026-10-01", Date: "2026-10-31", Description: "Tạm tính thuế", NonDeductible: "100", ExemptIncome: "0", CarriedLoss: "0"}
	b, e := f.ops.Execute(ctx, f.cmd("corporate-tax"), in)
	if e != nil {
		t.Fatal(e)
	}
	var result Result
	json.Unmarshal(b, &result)
	if result.Amounts["profitBeforeTax"] != "600" || result.Amounts["taxable"] != "700" || result.Amounts["tax"] != "140" {
		t.Fatal(result.Amounts)
	}
	if _, e = f.ops.Execute(ctx, f.cmd("tax-overlap"), in); accounting.Code(e) != "DUPLICATE_SOURCE" {
		t.Fatal(e)
	}
	b, e = f.ops.Execute(ctx, f.cmd("closing"), Input{Kind: "closing", SourceID: "closing", Date: "2026-10-31", Description: "Kết chuyển"})
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(b, &result)
	if result.Amounts["result"] != "460" {
		t.Fatal(result.Amounts)
	}
	var closed bool
	if e = f.store.Read.QueryRow("SELECT closed FROM periods WHERE period='2026-10'").Scan(&closed); e != nil || !closed {
		t.Fatal(closed, e)
	}
	metrics, e := f.ledger.Metrics(ctx, f.store.Read, "2026-10")
	if e != nil || metrics.Revenue != "1000" || metrics.Expense != "540" {
		t.Fatal(metrics, e)
	}
	if _, e = f.ledger.SavePeriod(ctx, f.cmd("unsafe-reopen"), ledger.Period{Period: "2026-10"}); accounting.Code(e) != "PERIOD_FINALIZED" {
		t.Fatal(e)
	}
	reopen := ledger.Reopen{Period: "2026-10", Reason: "Kiểm tra điều chỉnh có liên kết"}
	first, e := f.ledger.ReopenPeriod(ctx, f.cmd("reopen-closing"), reopen)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.ledger.ReopenPeriod(ctx, f.cmd("reopen-closing"), reopen)
	if e != nil || string(first) != string(again) {
		t.Fatal(string(again), e)
	}
	b, e = f.ops.Execute(ctx, f.cmd("reclosing"), Input{Kind: "closing", SourceID: "closing-again", Date: "2026-10-31", Description: "Kết chuyển lại"})
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(b, &result)
	if result.Amounts["result"] != "460" {
		t.Fatal(result.Amounts)
	}
	metrics, e = f.ledger.Metrics(ctx, f.store.Read, "2026-10")
	if e != nil || metrics.Revenue != "1000" || metrics.Expense != "540" || metrics.Balances["421"] != "-460" || metrics.Balances["511"] != "0" || metrics.Balances["911"] != "0" {
		t.Fatal(metrics, e)
	}
}

func TestPreviewFencingAndMultipleStockLines(t *testing.T) {
	f := openFixture(t)
	p := testPolicy()
	p.InventoryMethod = "moving_average"
	f.policy(t, p)
	ctx := context.Background()
	if _, e := f.ops.SaveItem(ctx, f.cmd("item"), Item{ID: "stock", Code: "ST", Name: "Vật tư", Unit: "cái", Tracked: true, Active: true}); e != nil {
		t.Fatal(e)
	}
	purchase := Input{Kind: "purchase", SourceID: "MH", Date: "2026-10-01", Description: "Mua hai dòng cùng hàng", PartnerID: "supplier", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "stock", Quantity: "10", UnitPrice: "10", VATRate: "0", Deductible: new(true)}, {ItemID: "stock", Quantity: "10", UnitPrice: "20", VATRate: "0", Deductible: new(true)}}}
	if _, e := f.ops.Execute(ctx, f.cmd("purchase"), purchase); e != nil {
		t.Fatal(e)
	}
	sale := Input{Kind: "sale", SourceID: "BH", Date: "2026-10-02", Description: "Bán hai dòng cùng hàng", PartnerID: "customer", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "stock", Quantity: "5", UnitPrice: "30", VATRate: "0"}, {ItemID: "stock", Quantity: "5", UnitPrice: "30", VATRate: "0"}}}
	preview, e := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, sale)
	if e != nil || preview.Amounts["cost"] != "150" {
		t.Fatal(preview, e)
	}
	sale.ExpectedRevision = &preview.Revision
	sale.ExpectedPolicyID = preview.PolicyID
	f.save(t, "another-writer")
	if _, e = f.ops.Execute(ctx, f.cmd("stale-preview"), sale); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
	sale.ExpectedRevision = nil
	sale.ExpectedPolicyID = ""
	if _, e = f.ops.Execute(ctx, f.cmd("sale"), sale); e != nil {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}

func TestWorkInProgressAndReservedSources(t *testing.T) {
	f := openFixture(t)
	f.policy(t, testPolicy())
	ctx := context.Background()
	d := f.draft("work-in-progress")
	d.Lines = []ledger.Line{{AccountID: "154", Debit: "1000", Credit: "0"}, {AccountID: "111", Debit: "0", Credit: "1000"}}
	b, e := f.ledger.Save(ctx, f.cmd("wip"), d)
	d = decodeDocument(t, b, e)
	if _, e = f.ledger.Post(ctx, f.cmd("post-wip"), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
		t.Fatal(e)
	}
	metrics, e := f.ledger.Metrics(ctx, f.store.Read, "2026-10")
	if e != nil || metrics.Expense != "0" {
		t.Fatal(metrics, e)
	}
	b, e = f.ops.Execute(ctx, f.cmd("service-cost"), Input{Kind: "costing", SourceID: "service-cost", Date: "2026-10-30", Description: "Dịch vụ hoàn thành", Amount: "600"})
	if e != nil {
		t.Fatal(e)
	}
	var r Result
	json.Unmarshal(b, &r)
	if _, e = f.ops.Execute(ctx, f.cmd("over-cost"), Input{Kind: "costing", SourceID: "over-cost", Date: "2026-10-30", Description: "Vượt chi phí", Amount: "401"}); accounting.Code(e) != "INSUFFICIENT_COST" {
		t.Fatal(e)
	}
	if _, e = f.ops.Execute(ctx, f.cmd("close-wip"), Input{Kind: "closing", SourceID: "close", Date: "2026-10-31", Description: "Kết chuyển"}); e != nil {
		t.Fatal(e)
	}
	metrics, e = f.ledger.Metrics(ctx, f.store.Read, "2026-10")
	if e != nil || metrics.Balances["154"] != "400" || metrics.Expense != "600" || metrics.Balances["421"] != "600" {
		t.Fatal(metrics, e)
	}
	trace, e := f.ops.Trace(ctx, r.Document.ID)
	if e != nil || trace.Input.Amount != "600" || trace.Policy.Version != 1 || trace.Calculation.Amounts["amount"] != "600" {
		t.Fatal(trace, e)
	}
	for _, source := range []string{"closing", "closing_reversal", "reversal", "payroll"} {
		d = f.draft("fake-" + source)
		d.SourceType = source
		if _, e = f.ledger.Save(ctx, f.cmd("fake-"+source), d); accounting.Code(e) != "DERIVED_FIELD" {
			t.Fatal(e)
		}
	}
}

func TestScheduledInventoryPolicyCannotRevalueExistingStock(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	p := testPolicy()
	f.policy(t, p)
	p.EffectiveFrom = "2026-11-01"
	p.InventoryMethod = "moving_average"
	if _, err := f.ops.SavePolicy(ctx, f.cmd("future-policy"), p); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ops.SaveItem(ctx, f.cmd("stock-item"), Item{ID: "item", Code: "HH1", Name: "Hàng thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	_, err := f.ops.Execute(ctx, f.cmd("fifo-stock"), Input{Kind: "purchase", SourceID: "MH1", Date: "2026-10-15", Description: "Tồn theo FIFO", PartnerID: "supplier", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "item", Quantity: "2", UnitPrice: "100", VATRate: "0", Deductible: new(true)}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.ledger.SavePeriod(ctx, f.cmd("nov"), ledger.Period{Period: "2026-11"}); err != nil {
		t.Fatal(err)
	}
	_, err = f.ops.Execute(ctx, f.cmd("mixed-policy"), Input{Kind: "sale", SourceID: "BH1", Date: "2026-11-01", Description: "Không trộn giá", PartnerID: "customer", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "item", Quantity: "1", UnitPrice: "200", VATRate: "0"}}})
	if accounting.Code(err) != "STOCK_POLICY_LOCKED" {
		t.Fatal(err)
	}
	p.EffectiveFrom = "2026-10-01"
	if _, err = f.ops.SavePolicy(ctx, f.cmd("out-of-order-policy"), p); accounting.Code(err) != "HISTORY_LOCKED" {
		t.Fatal(err)
	}
}

func TestRestoreRejectsTraversalAndTampering(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "invalid.zip")
	file, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	archive := zip.NewWriter(file)
	entry, e := archive.Create("../escaped")
	if e != nil {
		t.Fatal(e)
	}
	entry.Write([]byte("invalid"))
	archive.Close()
	file.Close()
	if _, e = f.store.Stage(ctx, path); e == nil {
		t.Fatal("path traversal accepted")
	}
	d := f.save(t, "original")
	if _, e = f.ledger.Post(ctx, f.cmd("post"), ledger.Post{ID: d.ID, Version: d.Version}); e != nil {
		t.Fatal(e)
	}
	path, _, e = f.store.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	stage, e := f.store.Stage(ctx, path)
	if e != nil {
		t.Fatal(e)
	}
	stageDB := filepath.Join(f.store.Root, "staging", stage.ID, "accounting.sqlite")
	if e = os.WriteFile(stageDB, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Activate(ctx, stage.ID); e == nil {
		t.Fatal("corrupt staging activated")
	}
	metadata, e := ledger.ReadContext(ctx, f.store.Read)
	if e != nil || metadata.Epoch != f.epoch {
		t.Fatal(metadata, e)
	}
}

func openFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	db, e := sqlite.Open(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	auth := identity.Service{Store: db}
	if e = auth.Setup(ctx, identity.Setup{Username: "owner", Password: "test-password-only-2026", Name: "Chủ sở hữu", CompanyName: "Công ty kiểm thử", Regime: "Chính sách kiểm thử", ConfirmAccounts: true}); e != nil {
		t.Fatal(e)
	}
	f := &fixture{store: db, root: root}
	f.ledger = &ledger.Service{Store: db}
	f.ops = &Service{Ledger: f.ledger}
	if e = db.Read.QueryRow("SELECT id FROM users").Scan(&f.actor); e != nil {
		t.Fatal(e)
	}
	c, e := ledger.ReadContext(ctx, db.Read)
	if e != nil {
		t.Fatal(e)
	}
	f.epoch = c.Epoch
	if _, e = f.ledger.SavePeriod(ctx, f.cmd("period"), ledger.Period{Period: "2026-10"}); e != nil {
		t.Fatal(e)
	}
	_, e = f.ledger.SavePartners(ctx, f.cmd("partners"), []ledger.Partner{{ID: "customer", Code: "KH1", Name: "Khách mẫu", Role: "customer", Active: true}, {ID: "supplier", Code: "NCC1", Name: "Nhà cung cấp mẫu", Role: "supplier", Active: true}})
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func (f *fixture) cmd(key string) ledger.Command {
	return ledger.Command{Actor: f.actor, Epoch: f.epoch, Key: key}
}
func (f *fixture) draft(id string) ledger.Document {
	return ledger.Document{ID: id, Type: "journal", Date: "2026-10-21", Description: "Chứng từ kiểm thử", Lines: []ledger.Line{{AccountID: "111", Debit: "9007199254740993", Credit: "0"}, {AccountID: "411", Debit: "0", Credit: "9007199254740993"}}}
}
func decodeDocument(t *testing.T, b []byte, e error) ledger.Document {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	var d ledger.Document
	if e = json.Unmarshal(b, &d); e != nil {
		t.Fatal(e)
	}
	return d
}
func (f *fixture) save(t *testing.T, id string) ledger.Document {
	t.Helper()
	b, e := f.ledger.Save(context.Background(), f.cmd("save-"+id), f.draft(id))
	return decodeDocument(t, b, e)
}
func testPolicy() Policy {
	return Policy{Regime: "Chế độ kiểm thử", CorporateTaxRate: "20", EffectiveFrom: "2026-01-01", Confirmed: true, Rounding: "half_up", VATMethod: "deduction", VATRates: []string{"0", "10"}, InventoryMethod: "fifo", QuantityScale: 2, StandardDays: "26", HoursPerDay: "8", PersonalDeduction: "100", DependentDeduction: "50", Insurance: []Insurance{{Name: "Thử nghiệm", EmployeeRate: "10", EmployerRate: "20"}}, PITBrackets: []Bracket{{UpTo: "1000", Rate: "5"}, {UpTo: "", Rate: "10"}}, DepreciationMethod: "straight_line", ResidualMethod: "last_period"}
}
func (f *fixture) policy(t *testing.T, p Policy) {
	t.Helper()
	if _, e := f.ops.SavePolicy(context.Background(), f.cmd("policy"), p); e != nil {
		t.Fatal(e)
	}
}
func TestPostingRetryReopenAndImmutable(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	d := f.save(t, "doc")
	payload := ledger.Post{ID: d.ID, Version: d.Version}
	b, e := f.ledger.Post(ctx, f.cmd("post"), payload)
	posted := decodeDocument(t, b, e)
	if posted.Reference != "CT-2026-000001" {
		t.Fatal(posted)
	}
	retry, e := f.ledger.Post(ctx, f.cmd("post"), payload)
	if e != nil || string(retry) != string(b) {
		t.Fatal(e)
	}
	if _, e = f.ledger.Post(ctx, f.cmd("post"), ledger.Post{ID: d.ID, Version: 99}); accounting.Code(e) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("UPDATE document_lines SET debit=1 WHERE document_id=?", d.ID); e == nil {
		t.Fatal("posted lines changed")
	}
	if e = f.store.Close(); e != nil {
		t.Fatal(e)
	}
	db, e := sqlite.Open(ctx, f.root)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	f.ledger.Store = db
	retry, e = f.ledger.Post(ctx, f.cmd("post"), payload)
	if e != nil || string(retry) != string(b) {
		t.Fatal(e)
	}
	if e = sqlite.Check(ctx, db.Read); e != nil {
		t.Fatal(e)
	}
}
func TestRollbackIncludesNumbersAuditAndCommands(t *testing.T) {
	f := openFixture(t)
	d := f.save(t, "rollback")
	if _, e := f.store.Write.Exec("CREATE TRIGGER reject_post_audit BEFORE INSERT ON audit WHEN NEW.action='document.post' BEGIN SELECT RAISE(ABORT,'injected audit failure'); END"); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ledger.Post(context.Background(), f.cmd("post"), ledger.Post{ID: d.ID, Version: d.Version}); e == nil {
		t.Fatal("accepted failed audit")
	}
	got, e := ledger.ReadDocument(context.Background(), f.store.Read, d.ID)
	if e != nil || got.Status != "draft" || got.Version != d.Version {
		t.Fatal(got, e)
	}
	for _, query := range []string{"SELECT count(*) FROM counters", "SELECT count(*) FROM commands WHERE operation='document.post'"} {
		var n int
		if e = f.store.Read.QueryRow(query).Scan(&n); e != nil || n != 0 {
			t.Fatal(query, n, e)
		}
	}
}
func TestConcurrentPostingAndPeriodLock(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	d := f.save(t, "concurrent")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, e := f.ledger.Post(ctx, f.cmd("same-intent"), ledger.Post{ID: d.ID, Version: d.Version})
			errs <- e
		})
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if e := f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE status='posted'").Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	next := f.save(t, "lock-race")
	errs = make(chan error, 2)
	wg.Go(func() {
		_, e := f.ledger.Post(ctx, f.cmd("race-post"), ledger.Post{ID: next.ID, Version: next.Version})
		errs <- e
	})
	wg.Go(func() {
		_, e := f.ledger.SavePeriod(ctx, f.cmd("race-lock"), ledger.Period{Period: "2026-10", Closed: true})
		errs <- e
	})
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil && accounting.Code(e) != "PERIOD_CLOSED" {
			t.Fatal(e)
		}
	}
	if _, e := f.ledger.Post(ctx, f.cmd("closed-post"), ledger.Post{ID: next.ID, Version: next.Version}); e == nil {
		t.Fatal("posted into closed period")
	}
}
func TestEpochVersionSourceAndSQLiteConfiguration(t *testing.T) {
	f := openFixture(t)
	d := f.save(t, "version")
	changed := f.draft("version")
	changed.Version = 0
	if _, e := f.ledger.Save(context.Background(), f.cmd("stale"), changed); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
	cmd := f.cmd("epoch")
	cmd.Epoch = "old"
	if _, e := f.ledger.Post(context.Background(), cmd, ledger.Post{ID: d.ID, Version: d.Version}); accounting.Code(e) != "DATASET_CHANGED" {
		t.Fatal(e)
	}
	changed = f.draft("source-copy")
	changed.SourceType = "journal"
	changed.SourceID = "version"
	if _, e := f.ledger.Save(context.Background(), f.cmd("copy"), changed); accounting.Code(e) != "DUPLICATE_SOURCE" {
		t.Fatal(e)
	}
	csv := "sourceType,sourceId,type,date,description,partnerId,accountId,linePartnerId,lineDescription,debit,credit\nmanual,version,journal,2026-10-21,Nguồn đã nhập tay,,111,,,1,0\nmanual,version,journal,2026-10-21,Nguồn đã nhập tay,,411,,,0,1\n"
	if _, e := f.ledger.PreviewImport(context.Background(), f.cmd("duplicate-import"), ledger.ImportInput{CSV: csv}); accounting.Code(e) != "DUPLICATE_SOURCE" {
		t.Fatal(e)
	}
	connections := []*sql.Conn{}
	for range 3 {
		conn, e := f.store.Read.Conn(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		connections = append(connections, conn)
	}
	for _, conn := range connections {
		var fk, timeout, sync int
		if e := conn.QueryRowContext(context.Background(), "SELECT * FROM pragma_foreign_keys").Scan(&fk); e != nil {
			t.Fatal(e)
		}
		conn.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&timeout)
		conn.QueryRowContext(context.Background(), "PRAGMA synchronous").Scan(&sync)
		if fk != 1 || timeout != 5000 || sync != 2 {
			t.Fatal(fk, timeout, sync)
		}
		conn.Close()
	}
	if _, e := sqlite.Open(context.Background(), f.root); e == nil {
		t.Fatal("two application owners")
	}
}
func TestAutomaticTradeAndStock(t *testing.T) {
	for _, method := range []string{"fifo", "moving_average"} {
		t.Run(method, func(t *testing.T) {
			f := openFixture(t)
			p := testPolicy()
			p.InventoryMethod = method
			f.policy(t, p)
			ctx := context.Background()
			_, e := f.ops.SaveItem(ctx, f.cmd("item"), Item{ID: "item", Code: "H1", Name: "Hàng thử", Unit: "cái", Tracked: true, Active: true})
			if e != nil {
				t.Fatal(e)
			}
			for index, price := range []string{"10", "20"} {
				input := Input{Kind: "purchase", SourceID: fmt.Sprintf("invoice-%d", index), Date: "2026-10-01", Description: "Mua hàng", PartnerID: "supplier", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "item", Quantity: "10", UnitPrice: price, VATRate: "10", Deductible: new(true)}}}
				if _, e = f.ops.Execute(ctx, f.cmd(fmt.Sprintf("purchase-%d", index)), input); e != nil {
					t.Fatal(e)
				}
			}
			input := Input{Kind: "sale", SourceID: "sale", Date: "2026-10-02", Description: "Bán hàng", PartnerID: "customer", PriceMode: "exclusive", Lines: []TradeLine{{ItemID: "item", Quantity: "5", UnitPrice: "30", VATRate: "10"}}}
			preview, e := f.ops.Preview(ctx, identity.User{ID: f.actor, Role: "owner"}, f.epoch, input)
			if e != nil {
				t.Fatal(e)
			}
			want := "50"
			if method == "moving_average" {
				want = "75"
			}
			if preview.Amounts["cost"] != want || preview.Amounts["total"] != "165" {
				t.Fatal(preview.Amounts)
			}
			b, e := f.ops.Execute(ctx, f.cmd("sale"), input)
			if e != nil {
				t.Fatal(e)
			}
			again, e := f.ops.Execute(ctx, f.cmd("sale"), input)
			if e != nil || string(again) != string(b) {
				t.Fatal(e)
			}
			data, e := f.ops.Data(ctx)
			if e != nil || data.Items[0].Quantity != "1500" {
				t.Fatal(data, e)
			}
			expected := "250"
			if method == "moving_average" {
				expected = "225"
			}
			if data.Items[0].Value != expected {
				t.Fatal(data.Items)
			}
			input.SourceID = "oversell"
			input.Lines[0].Quantity = "100"
			if _, e = f.ops.Execute(ctx, f.cmd("oversell"), input); accounting.Code(e) != "INSUFFICIENT_STOCK" {
				t.Fatal(e)
			}
			var n int
			if e = f.store.Read.QueryRow("SELECT count(*) FROM documents WHERE status='posted'").Scan(&n); e != nil || n != 3 {
				t.Fatal(n, e)
			}
		})
	}
}
func TestPayrollAndDepreciationRemainder(t *testing.T) {
	f := openFixture(t)
	f.policy(t, testPolicy())
	ctx := context.Background()
	_, e := f.ops.SaveEmployee(ctx, f.cmd("employee"), Employee{ID: "employee", Code: "NV1", Name: "Nhân viên mẫu", BaseSalary: "2600", InsuranceBase: "1000", Dependents: 1, Active: true})
	if e != nil {
		t.Fatal(e)
	}
	input := Input{Kind: "payroll", SourceID: "salary", Date: "2026-10-31", Description: "Tính lương", EmployeeID: "employee", WorkedDays: "26", Allowance: "0", NonTaxableAllowance: "0", OvertimeHours: "0", OvertimeMultiplier: "1"}
	b, e := f.ops.Execute(ctx, f.cmd("payroll"), input)
	if e != nil {
		t.Fatal(e)
	}
	var result Result
	json.Unmarshal(b, &result)
	if result.Amounts["gross"] != "2600" || result.Amounts["tax"] != "185" || result.Amounts["net"] != "2315" {
		t.Fatal(result.Amounts)
	}
	if _, e = f.ops.SaveEmployee(ctx, f.cmd("salary-change"), Employee{ID: "employee", Code: "NV1", Name: "Tên mới", BaseSalary: "5000", InsuranceBase: "2000", Dependents: 0, Active: true}); e != nil {
		t.Fatal(e)
	}
	trace, e := f.ops.Trace(ctx, result.Document.ID)
	if e != nil || trace.Calculation.Amounts["baseSalary"] != "2600" || trace.Calculation.Amounts["insuranceBase"] != "1000" || trace.Calculation.Amounts["dependentCount"] != "1" {
		t.Fatal(trace, e)
	}
	input = Input{Kind: "asset", SourceID: "asset-source", Date: "2026-10-01", Description: "Ghi tăng", PartnerID: "supplier", Asset: Asset{ID: "asset", Code: "TS1", Name: "Tài sản thử", Kind: "fixed", Cost: "100", Residual: "0", Months: 3, StartPeriod: "2026-10"}}
	if _, e = f.ops.Execute(ctx, f.cmd("asset"), input); e != nil {
		t.Fatal(e)
	}
	for index, month := range []string{"2026-10", "2026-11", "2026-12"} {
		if index > 0 {
			if _, e = f.ledger.SavePeriod(ctx, f.cmd(month), ledger.Period{Period: month}); e != nil {
				t.Fatal(e)
			}
		}
		in := Input{Kind: "depreciation", SourceID: month, Date: month + "-28", Description: "Khấu hao", AssetID: "asset"}
		b, e = f.ops.Execute(ctx, f.cmd("dep-"+month), in)
		if e != nil {
			t.Fatal(e)
		}
		json.Unmarshal(b, &result)
		want := "33"
		if index == 2 {
			want = "34"
		}
		if result.Amounts["amount"] != want {
			t.Fatal(result.Amounts)
		}
	}
	var allocated int64
	if e = f.store.Read.QueryRow("SELECT allocated FROM assets WHERE id='asset'").Scan(&allocated); e != nil || allocated != 100 {
		t.Fatal(allocated, e)
	}
	if filepath.Base(f.store.Dataset) == "" {
		t.Fatal("missing dataset")
	}
}
