package api

import (
	"net/http"
	"strconv"

	"github.com/dbpilot/dbpilot/internal/audit"
)

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	entries, err := s.audit.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, ErrInternal(err))
		return
	}
	if entries == nil {
		entries = []audit.Entry{}
	}
	writeOK(w, entries)
}
