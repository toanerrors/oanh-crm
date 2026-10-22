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

var voucherTemplate = template.Must(template.New("voucher").Parse(`<!doctype html><html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Chứng từ {{.Document.Reference}}</title><style>body{font:13px Arial,sans-serif;color:#172b23;margin:28px}h1{font-size:22px}h2{font-size:17px;text-align:center}table{border-collapse:collapse;width:100%;margin-top:18px}th,td{border:1px solid #bac6bf;padding:8px;text-align:left}td.money{text-align:right;white-space:nowrap;font-variant-numeric:tabular-nums}.meta{display:grid;grid-template-columns:1fr 1fr;gap:8px}.muted{color:#52655a;font-size:12px}.notes{white-space:pre-wrap;overflow-wrap:anywhere}@media print{@page{size:A4 landscape;margin:12mm}body{margin:0}thead{display:table-header-group}tr{break-inside:avoid}}</style></head><body><h1>{{.Context.CompanyName}}</h1><h2>CHỨNG TỪ KẾ TOÁN {{.Document.Reference}}</h2><div class="meta"><span>Ngày hạch toán: {{.Document.Date}}</span><span>Ngày chứng từ: {{.Input.Voucher.DocumentDate}}</span><span>Trạng thái: {{if eq .Document.Status "posted"}}Đã ghi sổ{{else}}Bản nháp — chưa tác động đến sổ{{end}}</span><span>Đơn vị: VND</span><span>Người nộp / nhận / liên hệ: {{.Input.Voucher.Contact}}</span><span>Địa chỉ: {{.Input.Voucher.Address}}</span><span>Nhân viên: {{.Input.Voucher.Employee}}</span><span>Tham chiếu: {{.Input.Voucher.Reference}}</span></div><p>Diễn giải: {{.Document.Description}}</p>{{if .Input.InvoiceNumber}}<p>Hóa đơn: {{.Input.InvoiceSeries}} {{.Input.InvoiceNumber}} · Ngày {{.Input.InvoiceDate}}</p>{{end}}{{if .Input.Voucher.BankNumber}}<p>Tài khoản ngân hàng: {{.Input.Voucher.BankNumber}} · {{.Input.Voucher.BankName}}</p>{{end}}{{if .Input.Voucher.DestinationBankNumber}}<p>Tài khoản nhận: {{.Input.Voucher.DestinationBankNumber}} · {{.Input.Voucher.DestinationBankName}}</p>{{end}}<table><thead><tr><th>Diễn giải</th><th>Tài khoản</th><th>Tên tài khoản tại thời điểm lưu</th><th>Đối tượng</th><th>Nợ (VND)</th><th>Có (VND)</th></tr></thead><tbody>{{range .Document.Lines}}<tr><td>{{.Description}}</td><td>{{.AccountID}}</td><td>{{.AccountName}}</td><td>{{.PartnerName}}</td><td class="money">{{.Debit}}</td><td class="money">{{.Credit}}</td></tr>{{end}}</tbody><tfoot><tr><th colspan="4">Tổng</th><td class="money">{{.Debit}}</td><td class="money">{{.Credit}}</td></tr></tfoot></table>{{if .Calculation}}{{with index .Calculation.Amounts "total"}}<p>Tổng tiền thanh toán: <strong>{{.}} VND</strong></p>{{end}}{{end}}{{if .Input.Voucher.Notes}}<p class="notes">Ghi chú: {{.Input.Voucher.Notes}}</p>{{end}}<p class="muted">Phiên dữ liệu: {{.Context.Epoch}} · Revision {{.Context.Revision}} · Phiên bản chứng từ {{.Document.Version}} · Nguồn {{.Document.SourceID}}</p></body></html>`))

func (s *Server) printVoucher(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	value, err := s.Operations.Voucher(r.Context(), r.PathValue("id"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	var body bytes.Buffer
	if err = voucherTemplate.Execute(&body, value); err != nil {
		s.failure(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}
