package httpserver

import (
	"net/http"
	"strconv"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
)

func auditPage(r *http.Request) (int, int, error) {
	page, size := 1, 20
	for name, target := range map[string]*int{"page": &page, "pageSize": &size} {
		if value := r.URL.Query().Get(name); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || name == "page" && n > 1000000 || name == "pageSize" && n > 100 {
				return 0, 0, accounting.Fail("BAD_REQUEST", "Phân trang nhật ký không hợp lệ.")
			}
			*target = n
		}
	}
	return page, size, nil
}
func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	page, size, err := auditPage(r)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	q := r.URL.Query()
	out, err := s.Ledger.Audit(r.Context(), ledger.AuditFilter{Query: q.Get("q"), Actor: q.Get("actor"), Action: q.Get("action"), From: q.Get("from"), To: q.Get("to"), Page: page, PageSize: size})
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
func (s *Server) auditDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, true); !ok {
		return
	}
	id, err := ledger.AuditID(r.URL.Query().Get("id"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	page, size, err := auditPage(r)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	e, err := s.Ledger.AuditDetail(r.Context(), id, page, size)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	allowed, why := operations.AuditRestoreSupport(e)
	s.respond(w, 200, struct {
		ledger.AuditEntry
		CanRestore    bool   `json:"canRestore"`
		RestoreReason string `json:"restoreReason"`
	}{e, allowed, why})
}
func (s *Server) previewAuditRestore(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input operations.AuditRestoreInput
	if !s.decode(w, r, &input) {
		return
	}
	out, err := s.Operations.PreviewAuditRestore(r.Context(), s.command(r, u), input)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
func (s *Server) restoreAudit(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input operations.AuditRestoreInput
	if !s.decode(w, r, &input) {
		return
	}
	result, err := s.Operations.RestoreAudit(r.Context(), s.command(r, u), input)
	s.result(w, r, result, err)
}
