package httpserver

import (
	"net/http"
	"oanh.local/accounting/internal/modules/ledger"
	"oanh.local/accounting/internal/modules/operations"
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
