package httpserver

import (
	"net/http"
	"strconv"

	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
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

func (s *Server) saveVoucher(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.Input
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SaveVoucher(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) discardVoucher(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input ledger.Post
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.DiscardVoucher(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) saveAsset(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.Asset
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SaveAsset(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) voucher(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	value, e := s.Operations.Voucher(r.Context(), r.PathValue("id"))
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}

func (s *Server) referenceChart(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	s.respond(w, 200, accounting.ReferenceChart())
}
func (s *Server) applyReferenceChart(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Version string `json:"version"`
	}
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Ledger.ApplyReferenceChart(r.Context(), s.command(r, u), input.Version)
	s.result(w, r, b, e)
}

func (s *Server) banks(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	value, e := s.Operations.Banks(r.Context(), r.URL.Query().Get("period"))
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}
func (s *Server) saveBank(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.BankAccount
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SaveBank(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}

func (s *Server) warehouses(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	value, e := s.Operations.Warehouses(r.Context())
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}
func (s *Server) saveWarehouse(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var input operations.Warehouse
	if !s.decode(w, r, &input) {
		return
	}
	b, e := s.Operations.SaveWarehouse(r.Context(), s.command(r, u), input)
	s.result(w, r, b, e)
}
func (s *Server) purchaseCosts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	value, e := s.Operations.PurchaseCostSources(r.Context(), r.URL.Query().Get("asOf"))
	if e != nil {
		s.failure(w, r, e)
		return
	}
	s.respond(w, 200, value)
}
