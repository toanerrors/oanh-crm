package httpserver

import (
	"math/big"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

func (s *Server) filter(w http.ResponseWriter, r *http.Request) (ledger.Filter, bool) {
	q := r.URL.Query()
	f := ledger.Filter{Period: q.Get("period"), Source: q.Get("source"), BankAccount: q.Get("bank"), Kind: q.Get("kind"), Account: q.Get("account"), Status: q.Get("status"), Type: q.Get("type"), Query: q.Get("q"), Sort: q.Get("sort"), Desc: q.Get("direction") != "asc", Page: 1, PageSize: 10}
	if !accounting.Period(f.Period) || len(f.Query) > 300 || f.Source != "" && f.Source != "closing" || f.BankAccount != "" && !accounting.ID(f.BankAccount) {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Kỳ hoặc bộ lọc không hợp lệ."))
		return f, false
	}
	if f.Sort != "" && f.Sort != "date" && f.Sort != "amount" || q.Get("direction") != "" && q.Get("direction") != "asc" && q.Get("direction") != "desc" {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Thứ tự sắp xếp không hợp lệ."))
		return f, false
	}
	for name, target := range map[string]*int{"page": &f.Page, "pageSize": &f.PageSize} {
		if q.Get(name) != "" {
			n, e := strconv.Atoi(q.Get(name))
			if e != nil || n < 1 || name == "pageSize" && n > 100 || name == "page" && n > 1000000 {
				s.failure(w, r, accounting.Fail("BAD_REQUEST", "Phân trang không hợp lệ (tối đa 100 dòng/trang)."))
				return f, false
			}
			*target = n
		}
	}
	if q.Get("revision") != "" {
		revision, e := strconv.ParseInt(q.Get("revision"), 10, 64)
		if e != nil || revision < 0 {
			s.failure(w, r, accounting.Fail("BAD_REQUEST", "Phiên bản dữ liệu không hợp lệ."))
			return f, false
		}
		f.Revision = &revision
	}
	if f.Kind != "" && !strings.Contains("|journal|ledger|trial|debt|income|", "|"+f.Kind+"|") {
		s.failure(w, r, accounting.Fail("BAD_REQUEST", "Loại sổ không hợp lệ."))
		return f, false
	}
	return f, true
}
func (s *Server) documents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	f, ok := s.filter(w, r)
	if !ok {
		return
	}
	value, e := s.Ledger.Documents(r.Context(), f)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	f, ok := s.filter(w, r)
	if !ok {
		return
	}
	value, e := s.Ledger.Report(r.Context(), f)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}
func (s *Server) exportDocuments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	f, ok := s.filter(w, r)
	if !ok {
		return
	}
	f.Page = 1
	f.PageSize = 100000
	result, e := s.Ledger.Documents(r.Context(), f)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	if result.Total > 100000 {
		s.failure(w, r, accounting.Fail("EXPORT_LIMIT", "Tập xuất vượt 100.000 chứng từ, hãy thu hẹp bộ lọc."))
		return
	}
	rows := [][]string{}
	for _, d := range result.Documents {
		sum := new(big.Int)
		for _, l := range d.Lines {
			n, _ := new(big.Int).SetString(l.Debit, 10)
			sum.Add(sum, n)
		}
		rows = append(rows, []string{d.Reference, d.Date, d.Type, d.Description, sum.String() + " VND", d.Status, d.SourceType, d.SourceID})
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "chung-tu-" + f.Period + ".csv"}))
	result.Context.Period = f.Period
	if e = ledger.ExportRows(w, result.Context, []string{"Số chứng từ", "Ngày", "Loại", "Diễn giải", "Tổng Nợ", "Trạng thái", "Loại nguồn", "Mã nguồn"}, rows); e != nil {
		return
	}
}
func (s *Server) exportReport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	f, ok := s.filter(w, r)
	if !ok {
		return
	}
	f.Page = 1
	f.PageSize = 100000
	result, e := s.Ledger.Report(r.Context(), f)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	if result.Total > 100000 {
		s.failure(w, r, accounting.Fail("EXPORT_LIMIT", "Tập xuất vượt 100.000 dòng, hãy thu hẹp bộ lọc."))
		return
	}
	rows := [][]string{}
	for _, row := range result.Rows {
		rows = append(rows, []string{row.Code, row.Date, row.Name, row.Description, row.Account, row.OpeningDebit + " VND", row.OpeningCredit + " VND", row.Debit + " VND", row.Credit + " VND", row.ClosingDebit + " VND", row.ClosingCredit + " VND"})
	}
	result.Context.Period = f.Period
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "so-" + f.Kind + "-" + f.Period + ".csv"}))
	if e = ledger.ExportRows(w, result.Context, []string{"Mã", "Ngày", "Tên", "Diễn giải", "Tài khoản", "Dư đầu Nợ", "Dư đầu Có", "Phát sinh Nợ", "Phát sinh Có", "Dư cuối Nợ", "Dư cuối Có"}, rows); e != nil {
		return
	}
}
