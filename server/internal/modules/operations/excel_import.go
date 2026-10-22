package operations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/spreadsheet"
	"oanh.local/accounting/internal/platform/sqlite"
)

type ExcelBatch struct {
	ID            string              `json:"id"`
	Filename      string              `json:"filename"`
	Hash          string              `json:"hash"`
	State         string              `json:"state"`
	Kind          string              `json:"kind,omitempty"`
	CreatedAt     string              `json:"createdAt"`
	ImportedAt    string              `json:"importedAt,omitempty"`
	Sheets        []ExcelSheetInfo    `json:"sheets"`
	Configuration *ExcelConfiguration `json:"configuration,omitempty"`
	Result        *ExcelResult        `json:"result,omitempty"`
}
type ExcelSheetInfo struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}
type ExcelSource struct {
	Batch            ExcelBatch           `json:"batch"`
	Sheet            string               `json:"sheet"`
	HeaderRow        int                  `json:"headerRow"`
	Headers          []string             `json:"headers"`
	Sample           [][]spreadsheet.Cell `json:"sample"`
	SampleRows       []int                `json:"sampleRows"`
	PopulatedColumns []int                `json:"populatedColumns"`
}
type ExcelConfiguration struct {
	SchemaVersion  int            `json:"schemaVersion"`
	Kind           string         `json:"kind"`
	Sheet          string         `json:"sheet"`
	HeaderRow      int            `json:"headerRow"`
	Mapping        map[string]int `json:"mapping"`
	DateFormat     string         `json:"dateFormat"`
	NumberFormat   string         `json:"numberFormat"`
	Mode           string         `json:"mode"`
	Cutoff         string         `json:"cutoff,omitempty"`
	Source         string         `json:"source,omitempty"`
	IgnoreUnmapped bool           `json:"ignoreUnmapped"`
}
type ExcelIssue struct {
	Row     int    `json:"row"`
	Column  string `json:"column,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ExcelPreviewRow struct {
	Row         int    `json:"row"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Debit       string `json:"debit"`
	Credit      string `json:"credit"`
}
type ExcelPreview struct {
	BatchID     string            `json:"batchId"`
	Revision    int64             `json:"revision"`
	Rows        []ExcelPreviewRow `json:"rows"`
	Issues      []ExcelIssue      `json:"issues"`
	ErrorCount  int               `json:"errorCount"`
	Valid       int               `json:"valid"`
	Invalid     int               `json:"invalid"`
	Records     int               `json:"records"`
	Debit       string            `json:"debit"`
	Credit      string            `json:"credit"`
	Fingerprint string            `json:"fingerprint"`
}
type ExcelCommit struct {
	Configuration    ExcelConfiguration `json:"configuration"`
	ExpectedRevision int64              `json:"expectedRevision"`
	Fingerprint      string             `json:"fingerprint"`
}
type ExcelResult struct {
	BatchID     string   `json:"batchId"`
	Kind        string   `json:"kind"`
	Count       int      `json:"count"`
	Rows        int      `json:"rows"`
	DocumentIDs []string `json:"documentIds"`
	Revision    int64    `json:"revision"`
}
type excelRecord struct {
	Rows   []int
	Values []map[string]string
}

func excelOperation(config ExcelConfiguration) string {
	schema, _ := excelSchema(config.Kind)
	if schema.Owner {
		return "admin.excel.import"
	}
	return "excel.import"
}
func (s *Service) StageExcel(ctx context.Context, cmd ledger.Command, name string, data []byte) (json.RawMessage, error) {
	name = filepath.Base(name)
	if name == "." || name == "" || len(name) > 180 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return nil, accounting.Fail("VALIDATION", "Tên tệp không hợp lệ (tối đa 180 byte).")
	}
	book, err := spreadsheet.Read(name, data)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(book)
	if err != nil {
		return nil, err
	}
	hash := sqlite.Hash(data)
	payload := struct{ Name, Hash string }{name, hash}
	return s.Ledger.Write(ctx, cmd, "excel.stage", payload, func(tx *sql.Tx) (any, error) {
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM excel_imports WHERE actor_id=? AND state='staged'", cmd.Actor).Scan(&n); err != nil {
			return nil, err
		}
		if n >= 10 {
			return nil, accounting.Fail("IMPORT_LIMIT", "Bạn đang có 10 tệp chờ nhập. Bỏ tệp không dùng trong lịch sử nhập trước khi tải thêm.")
		}
		batch := ExcelBatch{ID: sqlite.ID(), Filename: name, Hash: hash, State: "staged", CreatedAt: sqlite.Now(), Sheets: []ExcelSheetInfo{}}
		for _, sheet := range book.Sheets {
			batch.Sheets = append(batch.Sheets, ExcelSheetInfo{Name: sheet.Name, Rows: len(sheet.Rows)})
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO excel_imports(id,actor_id,epoch,owner_only,filename,file_hash,workbook,state,created_at) VALUES(?,?,?,(SELECT role='owner' FROM users WHERE id=?),?,?,?,'staged',?)", batch.ID, cmd.Actor, cmd.Epoch, cmd.Actor, name, hash, string(raw), batch.CreatedAt)
		return batch, err
	})
}
func (s *Service) excelBatchTx(ctx context.Context, tx *sql.Tx, cmd ledger.Command, id string) (ExcelBatch, spreadsheet.Workbook, error) {
	var batch ExcelBatch
	var book spreadsheet.Workbook
	var epoch, raw, result, configuration string
	var imported sql.NullString
	var ownerOnly bool
	err := tx.QueryRowContext(ctx, "SELECT id,epoch,filename,file_hash,workbook,state,created_at,imported_at,result,owner_only,configuration FROM excel_imports WHERE id=? AND actor_id=?", id, cmd.Actor).Scan(&batch.ID, &epoch, &batch.Filename, &batch.Hash, &raw, &batch.State, &batch.CreatedAt, &imported, &result, &ownerOnly, &configuration)
	if err == sql.ErrNoRows {
		return batch, book, accounting.Fail("NOT_FOUND", "Không tìm thấy tệp nhập của bạn.")
	}
	if err != nil {
		return batch, book, err
	}
	if ownerOnly {
		if err = s.Ledger.CheckWriteScopeTx(ctx, tx, cmd, "admin.excel.read"); err != nil {
			return batch, book, err
		}
	}
	if epoch != cmd.Epoch && batch.State != "imported" {
		return batch, book, accounting.Fail("DATASET_CHANGED", "Tệp chờ thuộc phiên dữ liệu cũ. Hãy tải lại tệp và kiểm tra.")
	}
	if err = json.Unmarshal([]byte(raw), &book); err != nil {
		return batch, book, err
	}
	if configuration != "{}" {
		batch.Configuration = &ExcelConfiguration{}
		if err = json.Unmarshal([]byte(configuration), batch.Configuration); err != nil {
			return batch, book, err
		}
	}
	batch.ImportedAt = imported.String
	batch.Sheets = []ExcelSheetInfo{}
	for _, sheet := range book.Sheets {
		batch.Sheets = append(batch.Sheets, ExcelSheetInfo{Name: sheet.Name, Rows: len(sheet.Rows)})
	}
	if batch.State == "imported" {
		batch.Result = &ExcelResult{}
		if err = json.Unmarshal([]byte(result), batch.Result); err != nil {
			return batch, book, err
		}
		batch.Kind = batch.Result.Kind
	}
	return batch, book, nil
}
func (s *Service) ExcelSource(ctx context.Context, cmd ledger.Command, id, sheetName string, header int) (ExcelSource, error) {
	var result ExcelSource
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = s.Ledger.CheckWriteScopeTx(ctx, tx, cmd, "excel.read"); err != nil {
		return result, err
	}
	batch, book, err := s.excelBatchTx(ctx, tx, cmd, id)
	if err != nil {
		return result, err
	}
	result.Batch = batch
	if sheetName == "" && len(book.Sheets) > 0 {
		sheetName = book.Sheets[0].Name
	}
	if header == 0 {
		header = 1
	}
	sheet, err := excelSheet(book, sheetName, header)
	if err != nil {
		return result, err
	}
	result.Sheet = sheetName
	result.HeaderRow = header
	result.Headers = []string{}
	result.Sample = [][]spreadsheet.Cell{}
	result.SampleRows = []int{}
	if header > len(sheet.Rows) {
		return result, tx.Commit()
	}
	populated := map[int]bool{}
	width := len(sheet.Rows[header-1])
	for _, row := range sheet.Rows[header:] {
		for col, cell := range row {
			if cell.Value != "" || cell.Formula {
				populated[col] = true
			}
		}
		if len(row) > width {
			width = len(row)
		}
	}
	result.PopulatedColumns = []int{}
	for col := range populated {
		result.PopulatedColumns = append(result.PopulatedColumns, col)
	}
	sort.Ints(result.PopulatedColumns)
	for col := 0; col < width; col++ {
		label := ""
		if col < len(sheet.Rows[header-1]) {
			label = sheet.Rows[header-1][col].Value
		}
		if label == "" {
			label = fmt.Sprintf("Cột %d (trống tiêu đề)", col+1)
		}
		result.Headers = append(result.Headers, label)
	}
	for i := header; i < len(sheet.Rows) && len(result.Sample) < 10; i++ {
		if excelNonempty(sheet.Rows[i]) {
			result.Sample = append(result.Sample, sheet.Rows[i])
			result.SampleRows = append(result.SampleRows, i+1)
		}
	}
	return result, tx.Commit()
}
func (s *Service) ExcelHistory(ctx context.Context, cmd ledger.Command) ([]ExcelBatch, error) {
	result := []ExcelBatch{}
	tx, err := s.Ledger.Store.Read.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.Ledger.CheckWriteScopeTx(ctx, tx, cmd, "excel.read"); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,filename,file_hash,state,created_at,COALESCE(imported_at,''),result FROM excel_imports WHERE actor_id=? AND (owner_only=0 OR (SELECT role FROM users WHERE id=?)='owner') ORDER BY created_at DESC LIMIT 50", cmd.Actor, cmd.Actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b ExcelBatch
		var raw string
		if err = rows.Scan(&b.ID, &b.Filename, &b.Hash, &b.State, &b.CreatedAt, &b.ImportedAt, &raw); err != nil {
			return nil, err
		}
		b.Sheets = []ExcelSheetInfo{}
		if b.State == "imported" {
			b.Result = &ExcelResult{}
			if err = json.Unmarshal([]byte(raw), b.Result); err != nil {
				return nil, err
			}
			b.Kind = b.Result.Kind
		}
		result = append(result, b)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return result, tx.Commit()
}
func (s *Service) DiscardExcel(ctx context.Context, cmd ledger.Command, id string) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, "excel.discard", id, func(tx *sql.Tx) (any, error) {
		result, err := tx.ExecContext(ctx, "DELETE FROM excel_imports WHERE id=? AND actor_id=? AND state='staged'", id, cmd.Actor)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, accounting.Fail("NOT_FOUND", "Chỉ bỏ được tệp chờ do bạn tải lên.")
		}
		return map[string]bool{"discarded": true}, nil
	})
}
func excelSheet(book spreadsheet.Workbook, name string, header int) (spreadsheet.Sheet, error) {
	for _, sheet := range book.Sheets {
		if sheet.Name == name {
			if header < 1 || header > 50 {
				return sheet, accounting.Fail("VALIDATION", "Dòng tiêu đề phải từ 1–50 và nằm trong sheet.")
			}
			return sheet, nil
		}
	}
	return spreadsheet.Sheet{}, accounting.Fail("VALIDATION", "Chọn sheet tồn tại trong tệp.")
}
func excelNonempty(row []spreadsheet.Cell) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell.Value) != "" || cell.Formula {
			return true
		}
	}
	return false
}
func excelFingerprint(batch ExcelBatch, config ExcelConfiguration) string {
	raw, _ := json.Marshal(config)
	return sqlite.Hash([]byte(batch.Hash + ":" + string(raw)))
}
func (p *ExcelPreview) issue(row int, column string, err error) {
	p.ErrorCount++
	if len(p.Issues) < 1000 {
		message := err.Error()
		code := accounting.Code(err)
		if code == "INTERNAL_ERROR" {
			message = "Không thể kiểm tra dữ liệu. Thử lại hoặc liên hệ quản trị."
		}
		p.Issues = append(p.Issues, ExcelIssue{Row: row, Column: column, Code: code, Message: message})
	}
}
func (s *Service) PreviewExcel(ctx context.Context, cmd ledger.Command, id string, config ExcelConfiguration) (ExcelPreview, error) {
	tx, err := s.Ledger.Store.Write.BeginTx(ctx, nil)
	if err != nil {
		return ExcelPreview{}, err
	}
	defer tx.Rollback()
	if err = s.Ledger.CheckWriteScopeTx(ctx, tx, cmd, excelOperation(config)); err != nil {
		return ExcelPreview{}, err
	}
	batch, book, err := s.excelBatchTx(ctx, tx, cmd, id)
	if err != nil {
		return ExcelPreview{}, err
	}
	preview, _, err := s.checkExcelTx(ctx, tx, batch, book, config)
	return preview, err
}
func (s *Service) CommitExcel(ctx context.Context, cmd ledger.Command, id string, input ExcelCommit) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, excelOperation(input.Configuration), struct {
		ID    string
		Input ExcelCommit
	}{id, input}, func(tx *sql.Tx) (any, error) {
		metadata, err := ledger.ReadContext(ctx, tx)
		if err != nil {
			return nil, err
		}
		if metadata.Revision != input.ExpectedRevision {
			return nil, accounting.Fail("VERSION_CONFLICT", "Dữ liệu đã thay đổi. Kiểm tra lại tệp trước khi nhập.")
		}
		batch, book, err := s.excelBatchTx(ctx, tx, cmd, id)
		if err != nil {
			return nil, err
		}
		if batch.State != "staged" {
			return nil, accounting.Fail("IMPORT_ALREADY_DONE", "Tệp này đã được nhập. Xem kết quả trong lịch sử nhập.")
		}
		if excelFingerprint(batch, input.Configuration) != input.Fingerprint {
			return nil, accounting.Fail("VERSION_CONFLICT", "Thiết lập nhập đã thay đổi. Hãy kiểm tra lại dữ liệu.")
		}
		preview, ids, err := s.checkExcelTx(ctx, tx, batch, book, input.Configuration)
		if err != nil {
			return nil, err
		}
		if preview.ErrorCount > 0 {
			return nil, accounting.Fail("INVALID_IMPORT", "Gói nhập có lỗi. Sửa toàn bộ lỗi và kiểm tra lại; chưa ghi nhận dòng nào.")
		}
		result := ExcelResult{BatchID: id, Kind: input.Configuration.Kind, Count: preview.Records, Rows: len(preview.Rows), DocumentIDs: ids, Revision: metadata.Revision + 1}
		raw, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		configuration, err := json.Marshal(input.Configuration)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "UPDATE excel_imports SET state='imported',configuration=?,fingerprint=?,result=?,imported_at=? WHERE id=?", string(configuration), preview.Fingerprint, string(raw), sqlite.Now(), id)
		return result, err
	})
}
func (s *Service) checkExcelTx(ctx context.Context, tx *sql.Tx, batch ExcelBatch, book spreadsheet.Workbook, config ExcelConfiguration) (ExcelPreview, []string, error) {
	p := ExcelPreview{BatchID: batch.ID, Rows: []ExcelPreviewRow{}, Issues: []ExcelIssue{}, Debit: "0", Credit: "0", Fingerprint: excelFingerprint(batch, config)}
	ids := []string{}
	metadata, err := ledger.ReadContext(ctx, tx)
	if err != nil {
		return p, ids, err
	}
	p.Revision = metadata.Revision
	schema, ok := excelSchema(config.Kind)
	if !ok || config.SchemaVersion != schema.Version {
		return p, ids, accounting.Fail("VALIDATION", "Mẫu nhập không tồn tại hoặc phiên bản không phù hợp.")
	}
	if batch.State != "staged" {
		return p, ids, accounting.Fail("IMPORT_ALREADY_DONE", "Tệp đã được nhập.")
	}
	if config.DateFormat != "dmy" && config.DateFormat != "iso" || config.NumberFormat != "plain" && config.NumberFormat != "vi" && config.NumberFormat != "en" || config.Mode != "add" && config.Mode != "update" {
		return p, ids, accounting.Fail("VALIDATION", "Chọn cách xử lý danh mục, định dạng ngày và số hợp lệ.")
	}
	sheet, err := excelSheet(book, config.Sheet, config.HeaderRow)
	if err != nil {
		return p, ids, err
	}
	if config.HeaderRow > len(sheet.Rows) {
		return p, ids, accounting.Fail("VALIDATION", "Dòng tiêu đề nằm ngoài dữ liệu sheet.")
	}
	allowed := map[string]ExcelField{}
	mapped := map[int]bool{}
	for _, f := range schema.Fields {
		allowed[f.Key] = f
		if _, ok := config.Mapping[f.Key]; f.Required && !ok {
			p.issue(config.HeaderRow, f.Label, accounting.Fail("MAPPING_REQUIRED", "Chưa ghép cột bắt buộc."))
		}
	}
	for key, col := range config.Mapping {
		if _, ok := allowed[key]; !ok || col < 0 || col >= spreadsheet.MaxColumns {
			return p, ids, accounting.Fail("VALIDATION", "Có trường hoặc vị trí cột không được phép.")
		}
		if mapped[col] {
			p.issue(config.HeaderRow, allowed[key].Label, accounting.Fail("DUPLICATE_MAPPING", "Một cột nguồn chỉ ghép với một trường dữ liệu."))
		}
		mapped[col] = true
	}
	var duplicate string
	err = tx.QueryRowContext(ctx, "SELECT id FROM excel_imports WHERE fingerprint=? AND state='imported'", p.Fingerprint).Scan(&duplicate)
	if err == nil {
		p.issue(0, "", accounting.Fail("DUPLICATE_SOURCE", "Phần dữ liệu này của tệp đã được nhập trước đó."))
	} else if err != sql.ErrNoRows {
		return p, ids, err
	}
	globalErrors := p.ErrorCount > 0
	normalized := []map[string]string{}
	badRows := map[int]bool{}
	unmapped := map[int]bool{}
	for i := config.HeaderRow; i < len(sheet.Rows); i++ {
		row := sheet.Rows[i]
		if !excelNonempty(row) {
			continue
		}
		if len(p.Rows) >= 5000 {
			return p, ids, accounting.Fail("IMPORT_LIMIT", "Mỗi lần nhập tối đa 5.000 dòng dữ liệu.")
		}
		values := map[string]string{}
		for col, cell := range row {
			if !mapped[col] && (cell.Value != "" || cell.Formula) {
				unmapped[col] = true
			}
		}
		for _, f := range schema.Fields {
			cell := spreadsheet.Cell{Kind: "text"}
			if col, exists := config.Mapping[f.Key]; exists && col < len(row) {
				cell = row[col]
			}
			value, e := excelValue(f, cell, config, book.Date1904)
			if e != nil {
				p.issue(i+1, f.Label, e)
				badRows[len(p.Rows)] = true
			}
			values[f.Key] = value
		}

		if config.Kind == "receipt" || config.Kind == "payment" {
			if values["counterAccount"] == "" && values["invoiceSourceId"] == "" {
				p.issue(i+1, "Tài khoản đối ứng", accounting.Fail("REQUIRED", "Nhập tài khoản đối ứng hoặc nguồn công nợ cần phân bổ."))
				badRows[len(p.Rows)] = true
			}
			if values["invoiceSourceId"] != "" && (values["allocationAmount"] == "" || values["partnerCode"] == "") {
				p.issue(i+1, "Phân bổ công nợ", accounting.Fail("REQUIRED", "Nhập mã đối tượng và số tiền phân bổ công nợ."))
				badRows[len(p.Rows)] = true
			}
		}
		if config.Kind == "opening" {
			required := map[string][]string{"account": {"accountCode"}, "stock": {"itemCode", "warehouseCode", "quantity", "value"}, "fixed": {"code", "name", "cost", "allocated", "residual", "months", "startPeriod"}, "prepaid": {"code", "name", "cost", "allocated", "residual", "months", "startPeriod"}}[values["rowKind"]]
			for _, key := range required {
				if values[key] == "" {
					p.issue(i+1, allowed[key].Label, accounting.Fail("REQUIRED", "Thiếu thông tin cho loại dòng số dư đã chọn."))
					badRows[len(p.Rows)] = true
				}
			}
		}
		source := values["sourceId"]
		if source == "" {
			source = values["code"]
		}
		if source == "" {
			source = values["number"]
		}
		if source == "" {
			source = values["rowKind"]
		}
		description := values["description"]
		if description == "" {
			description = values["name"]
		}
		p.Rows = append(p.Rows, ExcelPreviewRow{Row: i + 1, Source: source, Description: description, Status: "valid", Debit: "0", Credit: "0"})
		normalized = append(normalized, values)
	}
	if len(p.Rows) == 0 {
		p.issue(0, "", accounting.Fail("IMPORT_EMPTY", "Không có dòng dữ liệu sau dòng tiêu đề."))
	}
	if !config.IgnoreUnmapped && len(unmapped) > 0 {
		columns := []int{}
		for col := range unmapped {
			columns = append(columns, col+1)
		}
		sort.Ints(columns)
		p.issue(config.HeaderRow, "", accounting.Fail("UNMAPPED_COLUMNS", fmt.Sprintf("Có dữ liệu ở cột chưa ghép %v. Ghép bổ sung hoặc xác nhận bỏ qua các cột này.", columns)))
	}
	if p.ErrorCount > 0 && (globalErrors || !config.IgnoreUnmapped && len(unmapped) > 0 || len(p.Rows) == 0) {
		for i := range p.Rows {
			p.Rows[i].Status = "invalid"
		}
		p.Invalid = len(p.Rows)
		return p, ids, nil
	}

	records := []excelRecord{}
	grouped := map[string]int{}
	isDocument := schema.Group == "Chứng từ"
	for i, values := range normalized {
		key := fmt.Sprintf("row:%d", i)
		if isDocument {
			key = values["sourceId"]
		}
		if config.Kind == "opening" {
			key = "opening"
		}
		if index, ok := grouped[key]; ok {
			records[index].Rows = append(records[index].Rows, i)
			records[index].Values = append(records[index].Values, values)
		} else {
			grouped[key] = len(records)
			records = append(records, excelRecord{Rows: []int{i}, Values: []map[string]string{values}})
		}
	}
	if len(records) > 500 {
		return p, ids, accounting.Fail("IMPORT_LIMIT", "Mỗi lần nhập tối đa 500 chứng từ hoặc bản ghi danh mục.")
	}
	p.Records = len(records)
	seen := map[string]bool{}
	debit, credit := new(big.Int), new(big.Int)
	for _, record := range records {
		invalid := false
		for _, i := range record.Rows {
			if badRows[i] {
				invalid = true
			}
		}
		if invalid {
			p.Invalid += len(record.Rows)
			for _, i := range record.Rows {
				p.Rows[i].Status = "invalid"
			}
			continue
		}
		if err = ctx.Err(); err != nil {
			return p, ids, err
		}
		if _, err = tx.ExecContext(ctx, "SAVEPOINT excel_record"); err != nil {
			return p, ids, err
		}
		doc, e := s.applyExcelRecord(ctx, tx, batch, config, record, seen)
		if e != nil {
			if _, err = tx.ExecContext(ctx, "ROLLBACK TO excel_record"); err != nil {
				return p, ids, err
			}
			if accounting.Code(e) == "INTERNAL_ERROR" {
				return p, ids, e
			}
			p.issue(p.Rows[record.Rows[0]].Row, "", e)
			p.Invalid += len(record.Rows)
			for _, i := range record.Rows {
				p.Rows[i].Status = "invalid"
			}
		} else {
			p.Valid += len(record.Rows)
			if doc != nil {
				ids = append(ids, doc.ID)
				d, c := new(big.Int), new(big.Int)
				for _, line := range doc.Lines {
					n, _ := new(big.Int).SetString(line.Debit, 10)
					m, _ := new(big.Int).SetString(line.Credit, 10)
					d.Add(d, n)
					c.Add(c, m)
				}
				debit.Add(debit, d)
				credit.Add(credit, c)
				p.Rows[record.Rows[0]].Debit = d.String()
				p.Rows[record.Rows[0]].Credit = c.String()
			}
		}
		if _, err = tx.ExecContext(ctx, "RELEASE excel_record"); err != nil {
			return p, ids, err
		}
	}
	p.Debit, p.Credit = debit.String(), credit.String()
	return p, ids, nil
}

func excelValue(field ExcelField, cell spreadsheet.Cell, config ExcelConfiguration, date1904 bool) (string, error) {
	value := strings.TrimSpace(cell.Value)
	if cell.Formula {
		return value, accounting.Fail("FORMULA_CELL", "Ô đang chứa công thức. Dán giá trị đã đối chiếu vào tệp trước khi nhập.")
	}
	if cell.Kind == "error" {
		return value, accounting.Fail("CELL_ERROR", "Ô Excel đang có giá trị lỗi.")
	}
	if value == "" {
		if field.Required {
			return "", accounting.Fail("REQUIRED", "Thông tin bắt buộc chưa có dữ liệu.")
		}
		return field.Default, nil
	}
	switch field.Type {
	case "date":
		return spreadsheet.Date(cell, config.DateFormat, date1904)
	case "money", "decimal", "integer":
		n, e := spreadsheet.Number(value, config.NumberFormat, cell.Kind == "number")
		if e != nil {
			return value, e
		}
		if field.Type == "money" || field.Type == "integer" {
			_, e = accounting.Money(n)
		}
		if field.Type == "decimal" {
			_, e = accounting.Decimal(n)
		}
		return n, e
	case "identifier":
		if cell.Kind == "number" {
			return value, accounting.Fail("TEXT_REQUIRED", "Mã số, số tài khoản, số hóa đơn và điện thoại phải lưu dạng Text để giữ số 0 đầu.")
		}
	case "bool":
		switch excelHeader(value) {
		case "true", "1", "co", "dangsu dung", "dangsudung":
			return "true", nil
		case "false", "0", "khong", "ngungsudung":
			return "false", nil
		default:
			return value, accounting.Fail("VALIDATION", "Chỉ nhận Có/Không hoặc true/false.")
		}
	case "choice":
		for _, choice := range field.Choices {
			if strings.EqualFold(value, choice) {
				return choice, nil
			}
		}
		return value, accounting.Fail("VALIDATION", "Giá trị phải thuộc danh sách: "+strings.Join(field.Choices, ", "))
	case "period":
		if !accounting.Period(value) {
			return value, accounting.Fail("VALIDATION", "Kỳ phải có định dạng YYYY-MM.")
		}
	}
	if len(value) > 1000 {
		return value, accounting.Fail("VALIDATION", "Nội dung tối đa 1.000 byte.")
	}
	return value, nil
}

func (s *Service) ExcelTemplate(kind string, advanced bool) ([]byte, error) {
	schema, ok := excelSchema(kind)
	if !ok {
		return nil, accounting.Fail("VALIDATION", "Không có mẫu nhập này.")
	}
	headers := []string{}
	notes := []string{"Chỉ nhập ở sheet DuLieu; chọn đúng sheet và dòng tiêu đề trong trình nhập.", "Giữ dạng Text để không mất số 0 đầu hoặc độ chính xác tiền. Không nhập công thức, macro hay liên kết ngoài.", "Dùng mã danh mục đã khai báo; không dùng ID nội bộ. Không lấy số dòng làm mã chứng từ nguồn.", "Các dòng cùng mã chứng từ nguồn là một chứng từ; lặp đầy đủ thông tin chung trên các dòng.", "Chứng từ và số dư chỉ nhập thành bản nháp, chưa ghi sổ hoặc xác nhận số dư.", "Ngày dạng DD/MM/YYYY; số theo định dạng bạn chọn tại bước ghép dữ liệu.", "Số dư đầu kỳ: loại dòng account, stock, fixed hoặc prepaid; nhập đúng các cột chi tiết của loại đã chọn."}
	for _, field := range schema.Fields {
		if advanced || field.Basic || field.Required || kind == "opening" || kind == "journal-lines" {
			headers = append(headers, field.Label)
		}
		info := field.Label + " (" + field.Key + ")"
		if field.Required {
			info += " · Bắt buộc"
		}
		if len(field.Choices) > 0 {
			info += " · " + strings.Join(field.Choices, " / ")
		}
		if field.Default != "" {
			info += " · Để trống: " + field.Default
		}
		notes = append(notes, info)
	}
	return spreadsheet.Template(schema.Label, headers, notes)
}

func (s *Service) SaveExcelConfiguration(ctx context.Context, cmd ledger.Command, id string, config ExcelConfiguration) (json.RawMessage, error) {
	return s.Ledger.Write(ctx, cmd, excelOperation(config)+".configure", struct {
		ID            string
		Configuration ExcelConfiguration
	}{id, config}, func(tx *sql.Tx) (any, error) {
		schema, ok := excelSchema(config.Kind)
		if !ok || schema.Version != config.SchemaVersion {
			return nil, accounting.Fail("VALIDATION", "Mẫu nhập không hợp lệ.")
		}
		batch, book, err := s.excelBatchTx(ctx, tx, cmd, id)
		if err != nil {
			return nil, err
		}
		if batch.State != "staged" {
			return nil, accounting.Fail("IMPORT_ALREADY_DONE", "Tệp đã nhập, không đổi thiết lập.")
		}
		if _, err = excelSheet(book, config.Sheet, config.HeaderRow); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, "UPDATE excel_imports SET configuration=? WHERE id=?", string(raw), id)
		return map[string]bool{"saved": true}, err
	})
}
