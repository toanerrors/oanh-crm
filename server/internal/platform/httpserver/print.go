package httpserver

import (
	"bytes"
	"html/template"
	"net/http"

	"oanh.local/accounting/internal/accounting"
)

var reportTemplate = template.Must(template.New("report").Parse(`<!doctype html><html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Sổ kế toán · {{.CompanyName}} · {{.Period}}</title><style>body{font:13px system-ui,sans-serif;color:#172b23;margin:24px}table{border-collapse:collapse;width:100%}td,th{border:1px solid #bdc9c1;padding:7px;text-align:left}td.amount{text-align:right;white-space:nowrap;font-variant-numeric:tabular-nums}h1{font-size:22px}p{line-height:1.6}@media print{@page{size:A4 landscape;margin:12mm}body{margin:0}thead{display:table-header-group}tr{break-inside:avoid}}</style></head><body><h1>{{.CompanyName}}</h1><p>Sổ kế toán · Kỳ {{.Period}} · VND · Phiên bản {{.Revision}}<br>Phiên dữ liệu: {{.Epoch}} · {{.Total}} dòng trong toàn bộ tập lọc</p><table><thead><tr><th>Mã / số chứng từ</th><th>Ngày</th><th>Tên / diễn giải</th><th>TK</th><th>Dư đầu Nợ</th><th>Dư đầu Có</th><th>Nợ</th><th>Có</th><th>Dư cuối Nợ</th><th>Dư cuối Có</th></tr></thead><tbody>{{range .Rows}}<tr><td>{{.Code}}</td><td>{{.Date}}</td><td>{{.Name}}<br>{{.Description}}</td><td>{{.Account}}</td><td class="amount">{{.OpeningDebit}}</td><td class="amount">{{.OpeningCredit}}</td><td class="amount">{{.Debit}}</td><td class="amount">{{.Credit}}</td><td class="amount">{{.ClosingDebit}}</td><td class="amount">{{.ClosingCredit}}</td></tr>{{end}}</tbody><tfoot><tr><th colspan="4">Tổng toàn bộ tập lọc</th><td class="amount">{{.OpeningDebit}}</td><td class="amount">{{.OpeningCredit}}</td><td class="amount">{{.Debit}}</td><td class="amount">{{.Credit}}</td><td class="amount">{{.ClosingDebit}}</td><td class="amount">{{.ClosingCredit}}</td></tr></tfoot></table></body></html>`))

func (s *Server) printReport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	f, ok := s.filter(w, r)
	if !ok {
		return
	}
	f.Page = 1
	f.PageSize = 100000
	result, err := s.Ledger.Report(r.Context(), f)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	if result.Total > 100000 {
		s.failure(w, r, accounting.Fail("EXPORT_LIMIT", "Thu hẹp bộ lọc xuống tối đa 100.000 dòng trước khi in."))
		return
	}
	var body bytes.Buffer
	if err = reportTemplate.Execute(&body, result); err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}
