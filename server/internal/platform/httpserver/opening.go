package httpserver

import (
	"net/http"
	"oanh.local/accounting/internal/accounting"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
	"strconv"
)

func (s *Server) openingGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	out, err := s.Operations.Opening(r.Context())
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
func (s *Server) openingSave(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var in operations.OpeningInput
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.Operations.SaveOpening(r.Context(), s.command(r, u), in)
	s.result(w, r, out, err)
}
func (s *Server) openingPost(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var in ledger.Post
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.Operations.PostOpening(r.Context(), s.command(r, u), in)
	s.result(w, r, out, err)
}
func (s *Server) openingDiscard(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var in ledger.Post
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.Operations.DiscardOpening(r.Context(), s.command(r, u), in)
	s.result(w, r, out, err)
}

func (s *Server) openingAdjustmentSave(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var in operations.OpeningAdjustmentInput
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.Operations.SaveOpeningAdjustment(r.Context(), s.command(r, u), in)
	s.result(w, r, out, err)
}
func (s *Server) openingAdjustmentConfirm(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var in operations.OpeningAdjustmentPost
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.Operations.ConfirmOpeningAdjustment(r.Context(), s.command(r, u), in)
	s.result(w, r, out, err)
}
func (s *Server) openingAdjustmentDiscard(w http.ResponseWriter, r *http.Request) {
	u, ok := s.user(w, r, false)
	if !ok {
		return
	}
	var in ledger.Post
	if !s.decode(w, r, &in) {
		return
	}
	out, err := s.Operations.DiscardOpeningAdjustment(r.Context(), s.command(r, u), in)
	s.result(w, r, out, err)
}
func (s *Server) openingAdjustmentGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	out, err := s.Operations.OpeningAdjustment(r.Context(), r.PathValue("id"))
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
func (s *Server) openingHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.user(w, r, false); !ok {
		return
	}
	page, size := 1, 20
	var err error
	if v := r.URL.Query().Get("page"); v != "" {
		page, err = strconv.Atoi(v)
	}
	if err != nil {
		s.failure(w, r, accounting.Fail("VALIDATION", "Trang lịch sử không hợp lệ."))
		return
	}
	out, err := s.Operations.OpeningHistory(r.Context(), page, size)
	if err != nil {
		s.failure(w, r, err)
		return
	}
	s.respond(w, 200, out)
}
