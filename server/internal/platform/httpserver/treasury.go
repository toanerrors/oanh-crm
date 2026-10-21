package httpserver

import (
	"net/http"
	"strconv"

	"oanh.local/accounting/internal/accounting"
)

func (s *Server) invoices(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	q := r.URL.Query()
	page, size := 1, 10
	for name, target := range map[string]*int{"page": &page, "pageSize": &size} {
		if q.Get(name) != "" {
			n, err := strconv.Atoi(q.Get(name))
			if err != nil {
				s.failure(w, r, accounting.Fail("BAD_REQUEST", "Phân trang không hợp lệ."))
				return
			}
			*target = n
		}
	}
	result, err := s.Operations.Invoices(r.Context(), q.Get("asOf"), q.Get("kind"), q.Get("partner"), page, size)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, result)
}
