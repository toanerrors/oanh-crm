package operations

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"github.com/xuri/excelize/v2"
	"sync"
	"testing"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/sqlite"
)

func stageExcelTest(t *testing.T, f *fixture, key, kind string, headers []string, rows [][]string) (ExcelBatch, ExcelConfiguration) {
	t.Helper()
	buffer := bytes.Buffer{}
	writer := csv.NewWriter(&buffer)
	writer.Write(headers)
	writer.WriteAll(rows)
	writer.Flush()
	raw, e := f.ops.StageExcel(context.Background(), f.cmd("stage-"+key), key+".csv", buffer.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	var batch ExcelBatch
	if e = json.Unmarshal(raw, &batch); e != nil {
		t.Fatal(e)
	}
	config := ExcelConfiguration{SchemaVersion: 1, Kind: kind, Sheet: "CSV", HeaderRow: 1, Mapping: map[string]int{}, DateFormat: "dmy", NumberFormat: "plain", Mode: "add"}
	for i, h := range headers {
		config.Mapping[h] = i
	}
	return batch, config
}
func excelPreviewTest(t *testing.T, f *fixture, b ExcelBatch, c ExcelConfiguration) ExcelPreview {
	t.Helper()
	p, e := f.ops.PreviewExcel(context.Background(), f.cmd("preview-"+b.ID), b.ID, c)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func excelCommitTest(t *testing.T, f *fixture, b ExcelBatch, c ExcelConfiguration, p ExcelPreview, key string) ExcelResult {
	t.Helper()
	raw, e := f.ops.CommitExcel(context.Background(), f.cmd(key), b.ID, ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint})
	if e != nil {
		t.Fatal(e)
	}
	var result ExcelResult
	if e = json.Unmarshal(raw, &result); e != nil {
		t.Fatal(e)
	}
	return result
}
func excelCount(t *testing.T, f *fixture, query string) int {
	t.Helper()
	var n int
	if e := f.store.Read.QueryRow(query).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestExcelPreviewCommitAtomicExactAndRetry(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	headers := []string{"sourceId", "date", "description", "debitAccount", "creditAccount", "amount"}
	b, c := stageExcelTest(t, f, "bad", "journal-pairs", headers, [][]string{{"SRC1", "22/10/2026", "Thử tiền lớn", "111", "411", "9007199254740993"}, {"SRC2", "22/10/2026", "Sai tài khoản", "missing", "411", "100"}})
	p := excelPreviewTest(t, f, b, c)
	if p.Valid != 1 || p.Invalid != 1 || p.ErrorCount != 1 || p.Debit != "9007199254740993" {
		t.Fatalf("%+v", p)
	}
	if excelCount(t, f, "SELECT count(*) FROM documents") != 0 || excelCount(t, f, "SELECT count(*) FROM counters") != 0 {
		t.Fatal("preview persisted data")
	}
	if _, e := f.ops.CommitExcel(ctx, f.cmd("bad-commit"), b.ID, ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint}); accounting.Code(e) != "INVALID_IMPORT" {
		t.Fatal(e)
	}
	if excelCount(t, f, "SELECT count(*) FROM documents") != 0 {
		t.Fatal("partial import")
	}
	b, c = stageExcelTest(t, f, "valid", "journal-pairs", headers, [][]string{{"SRC1", "22/10/2026", "Thử tiền lớn", "111", "411", "9007199254740993"}})
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 || p.Debit != "9007199254740993" || p.Credit != p.Debit {
		t.Fatal(p)
	}
	input := ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint}
	raw, e := f.ops.CommitExcel(ctx, f.cmd("commit"), b.ID, input)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.ops.CommitExcel(ctx, f.cmd("commit"), b.ID, input)
	if e != nil || string(again) != string(raw) {
		t.Fatal(e)
	}
	if excelCount(t, f, "SELECT count(*) FROM documents") != 1 || excelCount(t, f, "SELECT count(*) FROM documents WHERE status='posted'") != 0 || excelCount(t, f, "SELECT count(*) FROM counters") != 0 {
		t.Fatal("unexpected posting or duplicate")
	}
	var result ExcelResult
	json.Unmarshal(raw, &result)
	doc, e := ledger.ReadDocument(ctx, f.store.Read, result.DocumentIDs[0])
	if e != nil || doc.Lines[0].Debit != "9007199254740993" {
		t.Fatal(doc, e)
	}
	changed := input
	changed.ExpectedRevision++
	if _, e = f.ops.CommitExcel(ctx, f.cmd("commit"), b.ID, changed); accounting.Code(e) != "IDEMPOTENCY_CONFLICT" {
		t.Fatal(e)
	}
	b2, c2 := stageExcelTest(t, f, "same-data", "journal-pairs", headers, [][]string{{"SRC1", "22/10/2026", "Thử tiền lớn", "111", "411", "9007199254740993"}})
	p2 := excelPreviewTest(t, f, b2, c2)
	if p2.ErrorCount == 0 {
		t.Fatal("duplicate batch accepted")
	}
	if e = sqlite.Check(ctx, f.store.Read); e != nil {
		t.Fatal(e)
	}
}
func TestExcelCatalogRollbackStaleRevisionAndProtectedHistory(t *testing.T) {
	f := openFixture(t)
	headers := []string{"code", "name", "role"}
	b, c := stageExcelTest(t, f, "partners", "partners", headers, [][]string{{"NEW", "Đối tượng mới", "customer"}, {"KH1", "Trùng đối tượng", "customer"}})
	p := excelPreviewTest(t, f, b, c)
	if p.Valid != 1 || p.Invalid != 1 {
		t.Fatal(p)
	}
	if excelCount(t, f, "SELECT count(*) FROM partners WHERE code='NEW'") != 0 {
		t.Fatal("preview leaked record")
	}
	if _, e := f.ops.CommitExcel(context.Background(), f.cmd("fail-partners"), b.ID, ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint}); accounting.Code(e) != "INVALID_IMPORT" {
		t.Fatal(e)
	}
	if excelCount(t, f, "SELECT count(*) FROM partners WHERE code='NEW'") != 0 {
		t.Fatal("partial catalog")
	}
	b, c = stageExcelTest(t, f, "partner-valid", "partners", headers, [][]string{{"NEW", "Đối tượng mới", "customer"}})
	p = excelPreviewTest(t, f, b, c)
	f.save(t, "revision-change")
	if _, e := f.ops.CommitExcel(context.Background(), f.cmd("stale-partners"), b.ID, ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint}); accounting.Code(e) != "VERSION_CONFLICT" {
		t.Fatal(e)
	}
	p = excelPreviewTest(t, f, b, c)
	excelCommitTest(t, f, b, c, p, "valid-partner-commit")
	if excelCount(t, f, "SELECT count(*) FROM partners WHERE code='NEW'") != 1 {
		t.Fatal("missing partner")
	}
}
func TestExcelFailedFinalizationRollsBackEntireBatch(t *testing.T) {
	f := openFixture(t)
	b, c := stageExcelTest(t, f, "rollback", "partners", []string{"code", "name", "role"}, [][]string{{"NEW1", "Đối tượng một", "customer"}, {"NEW2", "Đối tượng hai", "supplier"}})
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 {
		t.Fatal(p)
	}
	if _, e := f.store.Write.Exec("CREATE TRIGGER fail_excel BEFORE UPDATE OF state ON excel_imports BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); e != nil {
		t.Fatal(e)
	}
	defer f.store.Write.Exec("DROP TRIGGER fail_excel")
	if _, e := f.ops.CommitExcel(context.Background(), f.cmd("rollback-commit"), b.ID, ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint}); e == nil {
		t.Fatal("expected injected failure")
	}
	if excelCount(t, f, "SELECT count(*) FROM partners WHERE code IN ('NEW1','NEW2')") != 0 || excelCount(t, f, "SELECT count(*) FROM commands WHERE operation='excel.import'") != 0 {
		t.Fatal("partial commit")
	}
}
func TestExcelVoucherDraftReusesBusinessRulesWithoutPosting(t *testing.T) {
	f := openFixture(t)
	f.policy(t, testPolicy())
	if _, e := f.ops.SaveItem(context.Background(), f.cmd("item"), Item{ID: "service", Code: "DV1", Name: "Dịch vụ", Unit: "lần", Active: true}); e != nil {
		t.Fatal(e)
	}
	b, c := stageExcelTest(t, f, "purchase", "purchase", []string{"sourceId", "date", "description", "partnerCode", "tradeMode", "priceMode", "itemCode", "quantity", "unitPrice", "vatRate", "deductible"}, [][]string{{"PU-001", "22/10/2026", "Mua dịch vụ", "NCC1", "service", "exclusive", "DV1", "2", "500", "10", "true"}})
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 || p.Debit != "1100" || p.Credit != "1100" {
		t.Fatal(p)
	}
	result := excelCommitTest(t, f, b, c, p, "purchase-commit")
	voucher, e := f.ops.Voucher(context.Background(), result.DocumentIDs[0])
	if e != nil || !voucher.Document.Voucher || voucher.Document.Status != "draft" || voucher.Input.Lines[0].UnitPrice != "500" {
		t.Fatal(voucher, e)
	}
	if excelCount(t, f, "SELECT count(*) FROM operations") != 0 || excelCount(t, f, "SELECT count(*) FROM stock_movements") != 0 || excelCount(t, f, "SELECT count(*) FROM bank_movements") != 0 || excelCount(t, f, "SELECT count(*) FROM counters") != 0 {
		t.Fatal("draft affected subledgers")
	}
	b, c = stageExcelTest(t, f, "receipt", "receipt", []string{"sourceId", "date", "description", "moneyAccount", "counterAccount", "amount"}, [][]string{{"PT-001", "22/10/2026", "Thu vốn", "111", "411", "100"}})
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 || p.Debit != "100" {
		t.Fatal(p)
	}
	excelCommitTest(t, f, b, c, p, "receipt-commit")
}
func TestExcelOpeningMergesOnlyNewDetailsAndNeverConfirms(t *testing.T) {
	f := openFixture(t)
	b, c := stageExcelTest(t, f, "opening", "opening", []string{"rowKind", "accountCode", "debit", "credit"}, [][]string{{"account", "111", "9007199254740993", "0"}, {"account", "411", "0", "9007199254740993"}})
	c.Cutoff = "2026-10-01"
	c.Source = "Sổ giả lập đã đối chiếu"
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 || p.Debit != "9007199254740993" {
		t.Fatal(p)
	}
	excelCommitTest(t, f, b, c, p, "opening-commit")
	state, e := f.ops.Opening(context.Background())
	if e != nil || state.Document.Status != "draft" || state.PostedCount != 0 || len(state.Input.Lines) != 2 {
		t.Fatal(state, e)
	}
	b, c = stageExcelTest(t, f, "opening-duplicate", "opening", []string{"rowKind", "accountCode", "debit"}, [][]string{{"account", "111", "1"}})
	c.Cutoff = "2026-10-01"
	c.Source = "Sổ giả lập"
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 1 || p.Issues[0].Code != "DUPLICATE_CODE" {
		t.Fatal(p)
	}
}
func TestExcelMappingBlankAndNumericIdentifierErrors(t *testing.T) {
	f := openFixture(t)
	b, c := stageExcelTest(t, f, "mapping", "partners", []string{"code", "name", "role", "unknown"}, [][]string{{"M1", "Đối tượng", "customer", "Dữ liệu chưa ghép"}})
	delete(c.Mapping, "unknown")
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 1 || p.Issues[0].Code != "UNMAPPED_COLUMNS" {
		t.Fatal(p)
	}
	c.IgnoreUnmapped = true
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 {
		t.Fatal(p)
	}
	c.Mapping["role"] = c.Mapping["name"]
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount == 0 {
		t.Fatal("duplicate mapping accepted")
	}
}
func TestExcelBatchEpochOwnershipRoleAndBackupRestore(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	b, c := stageExcelTest(t, f, "restore", "journal-pairs", []string{"sourceId", "date", "description", "debitAccount", "creditAccount", "amount"}, [][]string{{"RESTORE-1", "22/10/2026", "Nháp từ Excel", "111", "411", "9007199254740993"}})
	p := excelPreviewTest(t, f, b, c)
	excelCommitTest(t, f, b, c, p, "restore-commit")
	if _, e := f.store.Write.Exec("UPDATE users SET role='accountant' WHERE id=?", f.actor); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ops.ExcelSource(ctx, f.cmd("source"), b.ID, "", 1); accounting.Code(e) != "FORBIDDEN" {
		t.Fatal(e)
	}
	f.store.Write.Exec("UPDATE users SET role='owner' WHERE id=?", f.actor)
	source, e := f.ops.ExcelSource(ctx, f.cmd("source"), b.ID, "", 1)
	if e != nil || source.Batch.Result.Count != 1 {
		t.Fatal(source, e)
	}
	other := f.cmd("other")
	other.Actor = "not-an-actor"
	if _, e = f.ops.ExcelSource(ctx, other, b.ID, "", 1); accounting.Code(e) != "FORBIDDEN" {
		t.Fatal(e)
	}
	if _, e = f.store.Write.Exec("INSERT INTO users SELECT 'other-owner','other-owner','Kiểm thử khác','owner',password_hash,1 FROM users WHERE id=?", f.actor); e != nil {
		t.Fatal(e)
	}
	other.Actor = "other-owner"
	if _, e = f.ops.ExcelSource(ctx, other, b.ID, "", 1); accounting.Code(e) != "NOT_FOUND" {
		t.Fatal(e)
	}
	archive, _, e := f.store.Backup(ctx)
	if e != nil {
		t.Fatal(e)
	}
	staged, e := f.store.Stage(ctx, archive)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.Activate(ctx, staged.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.ops.ExcelSource(ctx, f.cmd("old-epoch"), b.ID, "", 1); accounting.Code(e) != "DATASET_CHANGED" {
		t.Fatal(e)
	}
	metadata, e := ledger.ReadContext(ctx, f.store.Read)
	if e != nil {
		t.Fatal(e)
	}
	f.epoch = metadata.Epoch
	source, e = f.ops.ExcelSource(ctx, f.cmd("restored-source"), b.ID, "", 1)
	if e != nil || source.Batch.Result.Count != 1 || source.Sample[0][5].Value != "9007199254740993" {
		t.Fatal(source, e)
	}
	root := f.root
	f.store.Close()
	reopened, e := sqlite.Open(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	f.store = reopened
	f.ledger.Store = reopened
	if e = sqlite.Check(ctx, reopened.Read); e != nil {
		t.Fatal(e)
	}
	source, e = f.ops.ExcelSource(ctx, f.cmd("reopened-source"), b.ID, "", 1)
	if e != nil || source.Batch.Result.Count != 1 {
		t.Fatal(source, e)
	}
}
func TestExcelSchemasAndTemplatesCoverSupportedKinds(t *testing.T) {
	if len(ExcelSchemas("viewer")) != 0 {
		t.Fatal("viewer schemas")
	}
	for _, s := range ExcelSchemas("accountant") {
		if s.Owner {
			t.Fatal(s)
		}
	}
	for _, s := range ExcelSchemas("owner") {
		for _, advanced := range []bool{false, true} {
			raw, e := (&Service{}).ExcelTemplate(s.Key, advanced)
			if e != nil || !bytes.HasPrefix(raw, []byte("PK")) {
				t.Fatal(s.Key, e)
			}
		}
	}
	if excelHeader("Số Chứng Từ!") != "sochungtu" {
		t.Fatal("header normalization")
	}
}

func TestExcelConfigurationResumeAndConcurrentCommit(t *testing.T) {
	f := openFixture(t)
	b, c := stageExcelTest(t, f, "resume", "journal-pairs", []string{"sourceId", "date", "description", "debitAccount", "creditAccount", "amount"}, [][]string{{"resume-src", "22/10/2026", "Đồng thời", "111", "411", "123"}})
	ctx := context.Background()
	if _, err := f.ops.SaveExcelConfiguration(ctx, f.cmd("configure"), b.ID, c); err != nil {
		t.Fatal(err)
	}
	source, err := f.ops.ExcelSource(ctx, f.cmd("resume"), b.ID, "", 1)
	if err != nil || source.Batch.Configuration == nil || source.Batch.Configuration.Mapping["amount"] != 5 {
		t.Fatal(source, err)
	}
	p := excelPreviewTest(t, f, b, c)
	input := ExcelCommit{Configuration: c, ExpectedRevision: p.Revision, Fingerprint: p.Fingerprint}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, e := f.ops.CommitExcel(ctx, f.cmd(key), b.ID, input)
			results <- e
		}(key)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if accounting.Code(err) != "VERSION_CONFLICT" {
			t.Fatal(err)
		}
	}
	if success != 1 || excelCount(t, f, "SELECT count(*) FROM documents") != 1 {
		t.Fatal("duplicate concurrent commit")
	}
}

func TestExcelInvoiceAllocationAndPurchaseCostBindings(t *testing.T) {
	f := treasuryFixture(t)
	ctx := context.Background()
	sale := executeTreasury(t, f, "excel-invoice", tradeInput("excel-invoice", "sale", "1000"))
	b, c := stageExcelTest(t, f, "invoice-allocation", "receipt", []string{"sourceId", "date", "description", "partnerCode", "moneyAccount", "amount", "invoiceSourceId", "allocationAmount"}, [][]string{{"excel-collection", "10/10/2026", "Thu một phần", "KH1", "111", "300", "excel-invoice", "300"}})
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 || p.Debit != "300" {
		t.Fatal(p)
	}
	result := excelCommitTest(t, f, b, c, p, "allocation-commit")
	voucher, err := f.ops.Voucher(ctx, result.DocumentIDs[0])
	if err != nil || len(voucher.Input.Allocations) != 1 || voucher.Input.Allocations[0].DocumentID != sale.Document.ID || voucher.Document.Status != "draft" {
		t.Fatal(voucher, err)
	}
	invoices, err := f.ops.Invoices(ctx, "2026-10-10", "sale", "customer", 1, 10)
	if err != nil || invoices.Paid != "0" {
		t.Fatal(invoices, err)
	}
	source := tradeInput("excel-freight", "purchase", "101")
	source.TradeMode = "service"
	source.IsPurchaseCost = true
	source.Lines[0].AccountID = "6427"
	cost := executeTreasury(t, f, "excel-freight", source)
	if _, err = f.ops.SaveItem(ctx, f.cmd("excel-goods"), Item{ID: "excel-goods", Code: "EXHH", Name: "Hàng kiểm thử", Unit: "cái", Tracked: true, Active: true}); err != nil {
		t.Fatal(err)
	}
	b, c = stageExcelTest(t, f, "cost-binding", "purchase", []string{"sourceId", "date", "description", "partnerCode", "tradeMode", "itemCode", "quantity", "unitPrice", "vatRate", "deductible", "priceMode", "costAllocation", "costSourceId", "costSourceLine", "costAmount", "purchaseCostAmount"}, [][]string{{"excel-cost-goods", "01/10/2026", "Hàng kèm phí", "NCC1", "goods", "EXHH", "1", "1000", "10", "true", "exclusive", "manual", "excel-freight", "1", "101", "101"}})
	p = excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 {
		t.Fatal(p)
	}
	result = excelCommitTest(t, f, b, c, p, "cost-commit")
	voucher, err = f.ops.Voucher(ctx, result.DocumentIDs[0])
	if err != nil || len(voucher.Input.PurchaseCosts) != 1 || voucher.Input.PurchaseCosts[0].DocumentID != cost.Document.ID || voucher.Input.Lines[0].PurchaseCost != "101" {
		t.Fatal(voucher, err)
	}
}

func TestExcelCatalogUpdatePreservesUnmappedOptionalValues(t *testing.T) {
	f := openFixture(t)
	ctx := context.Background()
	if _, err := f.ledger.SavePartners(ctx, f.cmd("contact"), []ledger.Partner{{ID: "contact", Code: "CONTACT", Name: "Liên hệ", Role: "customer", Email: "synthetic@example.invalid", Phone: "0000123", Active: false}}); err != nil {
		t.Fatal(err)
	}
	b, c := stageExcelTest(t, f, "update-contact", "partners", []string{"code", "name", "role"}, [][]string{{"CONTACT", "Liên hệ", "customer"}})
	c.Mode = "update"
	p := excelPreviewTest(t, f, b, c)
	if p.ErrorCount != 0 {
		t.Fatal(p)
	}
	excelCommitTest(t, f, b, c, p, "update-contact")
	var email, phone string
	var active bool
	if err := f.store.Read.QueryRow("SELECT email,phone,active FROM partners WHERE id='contact'").Scan(&email, &phone, &active); err != nil || active || phone != "0000123" || email != "synthetic@example.invalid" {
		t.Fatal(email, phone, active, err)
	}
}

func TestExcelNumericIdentifiersAndFormulasAreRowErrors(t *testing.T) {
	for _, kind := range []string{"numeric", "formula"} {
		t.Run(kind, func(t *testing.T) {
			f := openFixture(t)
			book := excelize.NewFile()
			defer book.Close()
			for i, value := range []string{"code", "name", "role", "phone"} {
				axis, _ := excelize.CoordinatesToCellName(i+1, 1)
				book.SetCellStr("Sheet1", axis, value)
			}
			for i, value := range []string{"0001", "Kiểm thử", "customer"} {
				axis, _ := excelize.CoordinatesToCellName(i+1, 2)
				book.SetCellStr("Sheet1", axis, value)
			}
			want := "TEXT_REQUIRED"
			if kind == "numeric" {
				book.SetCellInt("Sheet1", "D2", 123)
			} else {
				book.SetCellFormula("Sheet1", "D2", "1+2")
				want = "FORMULA_CELL"
			}
			raw, err := book.WriteToBuffer()
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.ops.StageExcel(context.Background(), f.cmd("stage-typed"), "typed.xlsx", raw.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			var batch ExcelBatch
			json.Unmarshal(result, &batch)
			config := ExcelConfiguration{SchemaVersion: 1, Kind: "partners", Sheet: "Sheet1", HeaderRow: 1, Mapping: map[string]int{"code": 0, "name": 1, "role": 2, "phone": 3}, DateFormat: "dmy", NumberFormat: "plain", Mode: "add"}
			preview := excelPreviewTest(t, f, batch, config)
			if preview.ErrorCount != 1 || preview.Invalid != 1 || preview.Issues[0].Code != want {
				t.Fatal(preview)
			}
		})
	}
}
