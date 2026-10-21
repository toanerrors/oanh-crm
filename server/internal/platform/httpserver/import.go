package httpserver

import (
	"net/http"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
)

func (s *Server) previewImport(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	if u.Role == "viewer" {
		s.failure(w, r, accounting.Fail("FORBIDDEN", "Không có quyền nhập dữ liệu."))
		return
	}
	var input ledger.ImportInput
	if !s.decode(w, r, &input) {
		return
	}
	result, e := s.Ledger.PreviewImport(r.Context(), s.command(r, u), input)
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, result)
}
func (s *Server) importDocuments(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input ledger.ImportInput
	if !s.decode(w, r, &input) {
		return
	}
	result, e := s.Ledger.Import(r.Context(), s.command(r, u), input)
	s.result(w, r, result, e)
}
