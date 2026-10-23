package httpserver

import (
	"bytes"
	"html/template"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/platform/spreadsheet"
)

func (s *Server) exportFormat(w http.ResponseWriter, r *http.Request) (string, bool) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "xlsx" && format != "print" {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Chọn định dạng Excel, CSV hoặc bản in."))
		return "", false
	}
	return format, true
}
func exportLimit(format string) int {
	if format == "csv" {
		return 100000
	}
	return spreadsheet.MaxExportRows
}
func filterMetadata(f ledger.Filter) [][]string {
	rows := [][]string{{"Phạm vi", "Toàn bộ tập lọc"}}
	for _, row := range [][]string{{"Tìm kiếm", f.Query}, {"Tài khoản", f.Account}, {"Tài khoản ngân hàng", f.BankAccount}, {"Loại chứng từ", f.Type}, {"Trạng thái", f.Status}, {"Nguồn", f.Source}} {
		if row[1] != "" && row[1] != "all" {
			rows = append(rows, row)
		}
	}
	sort := "Ngày"
	if f.Sort == "amount" {
		sort = "Số tiền"
	}
	direction := "Tăng dần"
	if f.Desc {
		direction = "Giảm dần"
	}
	return append(rows, []string{"Sắp xếp", sort + " · " + direction})
}
func (s *Server) writeExport(w http.ResponseWriter, r *http.Request, format, name string, scope ledger.Context, book spreadsheet.ExportBook) {
	if scope.Epoch != "" {
		book.Metadata = append(book.Metadata, []string{"Phiên dữ liệu", scope.Epoch}, []string{"Mã doanh nghiệp", scope.CompanyID}, []string{"Chế độ kế toán", scope.Regime})
	}
	var body bytes.Buffer
	var err error
	contentType := "text/csv; charset=utf-8"
	extension := "csv"
	switch format {
	case "xlsx":
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		extension = "xlsx"
		var output []byte
		output, err = spreadsheet.ExportXLSX(r.Context(), book)
		if err == nil {
			_, err = body.Write(output)
		}
	case "print":
		contentType = "text/html; charset=utf-8"
		extension = "html"
		err = exportPrintTemplate.Execute(&body, book)
	default:
		rows := [][]string{}
		for i, table := range book.Tables {
			if i > 0 {
				rows = append(rows, []string{table.Name}, table.Headers)
			}
			for _, row := range table.Rows {
				values := append([]string(nil), row...)
				for col := range table.MoneyColumns {
					if col < len(values) {
						values[col] += " VND"
					}
				}
				rows = append(rows, values)
			}
		}
		err = ledger.ExportRows(&body, scope, book.Tables[0].Headers, rows)
	}
	if err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	disposition := "attachment"
	if format == "print" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name + "." + extension}))
	_, _ = w.Write(body.Bytes())
}

func displayExport(table spreadsheet.ExportTable, column int, value string) string {
	if !table.MoneyColumns[column] || value == "" {
		return value
	}
	sign := ""
	if strings.HasPrefix(value, "-") {
		sign = "-"
		value = value[1:]
	}
	parts := strings.SplitN(value, ".", 2)
	digits := parts[0]
	for _, c := range digits {
		if c < '0' || c > '9' {
			return sign + value
		}
	}
	var out strings.Builder
	out.WriteString(sign)
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(c)
	}
	if len(parts) > 1 {
		out.WriteByte(',')
		out.WriteString(parts[1])
	}
	return out.String()
}

var exportPrintTemplate = template.Must(template.New("export").Funcs(template.FuncMap{"display": displayExport, "money": func(t spreadsheet.ExportTable, c int) bool { return t.MoneyColumns[c] }}).Parse(`<!doctype html><html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Title}}</title><script src="/financial-print.js" defer></script><style>body{font:13px Arial,sans-serif;color:#17372c;margin:24px}h1{font-size:22px}h2{font-size:17px}table{width:100%;border-collapse:collapse;margin:16px 0}th,td{padding:7px;border:1px solid #bdcec5;text-align:left;white-space:pre-wrap;overflow-wrap:anywhere}th{background:#e7f2ed}td.amount{text-align:right;white-space:nowrap;font-variant-numeric:tabular-nums}tfoot{font-weight:bold}.meta{font-size:12px;color:#53695f}.sheet+.sheet{break-before:page}button{padding:8px 14px;background:#176b57;color:white;border:0;border-radius:5px;cursor:pointer}details{margin:10px 0}details p{display:inline-block;margin:4px 12px 0 0;font-size:12px;color:#53695f}@media print{@page{size:A4 landscape;margin:12mm}body{margin:0}button,summary{display:none}details{display:block}details p{display:inline;margin-right:12px}thead{display:table-header-group}tr{break-inside:avoid}}</style></head><body><button data-financial-print>In / Lưu PDF</button><h1>{{.Company}}</h1><h2>{{.Title}}</h2><p class="meta">{{.Period}} · VND · Phiên bản dữ liệu {{.Revision}} · Toàn bộ tập lọc</p><details open><summary>Bộ lọc và thông tin nguồn</summary>{{range .Metadata}}<p>{{range .}}{{.}} · {{end}}</p>{{end}}</details>{{range .Tables}}{{$table:=.}}<section class="sheet"><h2>{{.Name}}</h2><p class="meta">{{len .Rows}} dòng</p><table><thead><tr>{{range .Headers}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Rows}}<tr>{{range $col,$value:=.}}<td {{if money $table $col}}class="amount"{{end}}>{{display $table $col $value}}</td>{{end}}</tr>{{end}}</tbody>{{if .Total}}<tfoot><tr>{{range $col,$value:=.Total}}<td {{if money $table $col}}class="amount"{{end}}>{{display $table $col $value}}</td>{{end}}</tr></tfoot>{{end}}</table></section>{{end}}</body></html>`))

func (s *Server) exportCatalog(w http.ResponseWriter, r *http.Request) {
	user, ok := s.user(w, r, false)
	if !ok {
		return
	}
	format, ok := s.exportFormat(w, r)
	if !ok {
		return
	}
	f, ok := s.filter(w, r)
	if !ok {
		return
	}
	kind, role := r.URL.Query().Get("catalog"), r.URL.Query().Get("role")
	if kind != "accounts" && kind != "partners" || role != "" && role != "all" && role != "customer" && role != "supplier" {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Danh mục hoặc vai trò không hợp lệ."))
		return
	}
	workspace, err := s.Ledger.Snapshot(r.Context(), user.ID, user.Role, f.Period)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if f.Revision != nil && *f.Revision != workspace.Revision {
		s.failure(w, r, accounting.Fail("VERSION_CONFLICT", "Danh mục đã thay đổi. Tải lại dữ liệu trước khi xuất."))
		return
	}
	table := spreadsheet.ExportTable{Name: "Đối tượng", Headers: []string{"Mã đối tượng", "Tên đối tượng", "Vai trò", "Email", "Điện thoại", "Trạng thái"}}
	title := "Danh mục khách hàng và nhà cung cấp"
	active := func(value bool) string {
		if value {
			return "Đang sử dụng"
		}
		return "Ngừng sử dụng"
	}
	if kind == "accounts" {
		title = "Hệ thống tài khoản"
		table.Name = "Tài khoản"
		table.Headers = []string{"Mã tài khoản", "Tên tài khoản", "Tài khoản cha", "Yêu cầu đối tượng", "Trạng thái"}
		codes := map[string]string{}
		for _, account := range workspace.Accounts {
			codes[account.ID] = account.Code
		}
		for _, account := range ledger.CatalogAccounts(workspace.Accounts, f.Query) {
			parent := ""
			if account.ParentID != nil {
				parent = codes[*account.ParentID]
			}
			requires := "Không"
			if account.RequiresPartner {
				requires = "Có"
			}
			table.Rows = append(table.Rows, []string{account.Code, account.Name, parent, requires, active(account.Active)})
		}
	} else {
		roles := map[string]string{"customer": "Khách hàng", "supplier": "Nhà cung cấp", "both": "Khách hàng và nhà cung cấp"}
		for _, partner := range ledger.CatalogPartners(workspace.Partners, f.Query, role) {
			table.Rows = append(table.Rows, []string{partner.Code, partner.Name, roles[partner.Role], partner.Email, partner.Phone, active(partner.Active)})
		}
	}
	if len(table.Rows) > exportLimit(format) {
		s.failure(w, r, accounting.Fail("EXPORT_LIMIT", "Tập xuất quá lớn. Thu hẹp bộ lọc hoặc chọn CSV."))
		return
	}
	book := spreadsheet.ExportBook{Title: title, Company: workspace.CompanyName, Period: f.Period, Revision: workspace.Revision, Metadata: append(filterMetadata(f), []string{"Vai trò đối tượng", role}), Tables: []spreadsheet.ExportTable{table}}
	s.writeExport(w, r, format, "danh-muc-"+kind, workspace.Context, book)
}

func (s *Server) exportFinancial(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	format, ok := s.exportFormat(w, r)
	if !ok {
		return
	}
	version, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil || version < 1 {
		s.failure(w, r, accounting.Fail("VALIDATION", "Phiên bản BCTC không hợp lệ."))
		return
	}
	reports, err := s.Ledger.FinancialReports(r.Context(), "", r.URL.Query().Get("id"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if len(reports) != 1 {
		s.failure(w, r, accounting.Fail("NOT_FOUND", "BCTC không tồn tại."))
		return
	}
	report := reports[0]
	if report.Input.Version != version {
		s.failure(w, r, accounting.Fail("VERSION_CONFLICT", "BCTC đã thay đổi. Mở lại bản đã lưu trước khi xuất."))
		return
	}
	book := spreadsheet.ExportBook{Title: "BẢN NHÁP BÁO CÁO TÀI CHÍNH", Company: report.CompanyName, Period: report.Input.From + " – " + report.Input.To, Revision: report.Input.SourceRevision, Metadata: [][]string{{"Phiên bản báo cáo", strconv.Itoa(version)}, {"Kỳ so sánh", report.Input.PreviousFrom + " – " + report.Input.PreviousTo}, {"Mẫu báo cáo", report.Input.Template}}}
	forms := ledger.FinancialForms()
	for _, id := range report.Input.Sheets {
		for _, sheet := range forms.Sheets {
			if sheet.ID != id {
				continue
			}
			previous := "Kỳ trước (VND)"
			if id == "B01-DN" {
				previous = "Đầu kỳ (VND)"
			}
			table := spreadsheet.ExportTable{Name: id, Headers: []string{"Chỉ tiêu", "Mã số", "Thuyết minh", "Kỳ này (VND)", previous}, MoneyColumns: map[int]bool{3: true, 4: true}}
			for _, row := range sheet.Rows {
				v := report.Computed[id][row.Code]
				table.Rows = append(table.Rows, []string{row.Name, row.Code, v.Note, v.Current, v.Previous})
			}
			book.Tables = append(book.Tables, table)
		}
	}
	notes := spreadsheet.ExportTable{Name: "Thuyết minh", Headers: []string{"Mục", "Nội dung"}, Rows: [][]string{{"Thuyết minh bổ sung", report.Input.Notes}, {"Ngày lập", report.Input.PreparedDate}, {"Người lập", report.Input.PreparedBy}, {"Kế toán trưởng", report.Input.ChiefAccountant}, {"Đại diện pháp luật", report.Input.Director}}}
	for _, issue := range report.Issues {
		notes.Rows = append(notes.Rows, []string{"Cần kiểm tra · " + issue.Sheet + " " + issue.Code, issue.Message})
	}
	book.Tables = append(book.Tables, notes)
	scope := ledger.Context{CompanyName: report.CompanyName, Period: book.Period, Revision: report.Input.SourceRevision, Currency: "VND", Regime: forms.Regime}
	if format == "csv" {
		scope.CompanyName = book.Title + " · " + report.CompanyName + " · bản v" + strconv.Itoa(version)
	}
	s.writeExport(w, r, format, "bctc-nhap-"+report.Input.From[:4]+"-v"+strconv.Itoa(version), scope, book)
}
