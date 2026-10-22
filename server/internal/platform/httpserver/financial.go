package httpserver

import (
	"bytes"
	"html/template"
	"net/http"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"strconv"
)

func (s *Server) financialTemplate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	s.respond(w, 200, ledger.FinancialForms())
}
func (s *Server) financialList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	out, err := s.Ledger.FinancialReports(r.Context(), r.URL.Query().Get("year"), r.URL.Query().Get("id"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
func (s *Server) financialPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	var input ledger.FinancialInput
	if !s.decode(w, r, &input) {
		return
	}
	out, err := ledger.CalculateFinancial(input)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
func (s *Server) financialSave(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input ledger.FinancialInput
	if !s.decode(w, r, &input) {
		return
	}
	out, err := s.Ledger.SaveFinancial(r.Context(), s.command(r, u), input)
	s.result(w, r, out, err)
}
func (s *Server) financialSource(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	out, err := s.Ledger.FinancialSource(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}

var financialPrintTemplate = template.Must(template.New("financial").Parse(`<!doctype html><html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Bản nháp BCTC {{.Report.Input.From}}</title><script src="/financial-print.js" defer></script><style>body{font:13px Arial,sans-serif;margin:24px;color:#182c23}h1{font-size:20px}h2{text-align:center;font-size:17px}table{border-collapse:collapse;width:100%}th,td{border:1px solid #b6c5bc;padding:7px}td.amount{text-align:right;white-space:nowrap}tr.total{font-weight:bold;background:#f2f7f4}.notes{white-space:pre-wrap;overflow-wrap:anywhere}.sign{display:flex;justify-content:space-between;gap:20px;margin-top:24px}.sheet{break-before:page}.draft{font-weight:bold;color:#715021}@media print{@page{size:A4 landscape;margin:12mm}body{margin:0}thead{display:table-header-group}tr{break-inside:avoid}button{display:none}}</style></head><body><button data-financial-print>In / lưu PDF</button><h1>{{.Report.CompanyName}}</h1><p>{{.Report.Input.Address}}</p><p class="draft">BẢN NHÁP BÁO CÁO TÀI CHÍNH · TT99/2025/TT-BTC · VND</p><p>Kỳ {{.Report.Input.From}} – {{.Report.Input.To}} · So sánh {{.Report.Input.PreviousFrom}} – {{.Report.Input.PreviousTo}}<br>Phiên bản báo cáo {{.Report.Input.Version}} · Phiên bản nguồn tham chiếu {{.Report.Input.SourceRevision}} · Cập nhật {{.Report.UpdatedAt}}</p>{{range .Sheets}}<section class="sheet"><h2>{{.Name}}</h2><p>{{.ID}} · {{if eq .ID "B01-DN"}}Tại ngày {{$.Report.Input.To}}{{else}}Kỳ {{$.Report.Input.From}} – {{$.Report.Input.To}}{{end}}</p><table><thead><tr><th>Chỉ tiêu</th><th>Mã số</th><th>Thuyết minh</th><th>Kỳ này</th><th>{{if eq .ID "B01-DN"}}Đầu kỳ{{else}}Kỳ trước{{end}}</th></tr></thead><tbody>{{range .Rows}}{{$v:=index (index $.Report.Computed .Sheet) .Code}}<tr {{if .Total}}class="total"{{end}}><td>{{.Name}}</td><td>{{.Code}}</td><td>{{$v.Note}}</td><td class="amount">{{if eq $v.Current ""}}Chưa nhập{{else}}{{$v.Current}}{{end}}</td><td class="amount">{{if eq $v.Previous ""}}Chưa nhập{{else}}{{$v.Previous}}{{end}}</td></tr>{{end}}</tbody></table></section>{{end}}<section class="sheet"><h2>Thuyết minh bổ sung và kiểm tra</h2><p class="notes">{{.Report.Input.Notes}}</p>{{if .Report.Issues}}<p>Các mục cần kiểm tra:</p><ul>{{range .Report.Issues}}<li>{{.Sheet}} {{.Code}}: {{.Message}}</li>{{end}}</ul>{{else}}<p>Các phép kiểm tra số học đã khớp; cần kế toán rà soát và phê duyệt.</p>{{end}}<p>Ngày lập: {{.Report.Input.PreparedDate}}</p><div class="sign"><span>Người lập<br>{{.Report.Input.PreparedBy}}</span><span>Kế toán trưởng<br>{{.Report.Input.ChiefAccountant}}</span><span>Người đại diện theo pháp luật<br>{{.Report.Input.Director}}</span></div></section></body></html>`))

func (s *Server) financialPrint(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	version, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil || version < 1 {
		s.failure(w, r, accounting.Fail("VALIDATION", "Phiên bản báo cáo không hợp lệ."))
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
		s.failure(w, r, accounting.Fail("VERSION_CONFLICT", "BCTC đã thay đổi. Mở lại bản đã lưu trước khi in."))
		return
	}
	type row struct {
		Sheet, Code, Name string
		Total             bool
	}
	type sheet struct {
		ID, Name string
		Rows     []row
	}
	sheets := []sheet{}
	for _, id := range report.Input.Sheets {
		for _, t := range ledger.FinancialForms().Sheets {
			if t.ID == id {
				item := sheet{ID: id, Name: t.Name}
				for _, v := range t.Rows {
					item.Rows = append(item.Rows, row{id, v.Code, v.Name, len(v.Formula) > 0})
				}
				sheets = append(sheets, item)
			}
		}
	}
	var body bytes.Buffer
	if err = financialPrintTemplate.Execute(&body, struct {
		Report ledger.FinancialReport
		Sheets []sheet
	}{report, sheets}); err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}
