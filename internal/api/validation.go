package api

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/dbpilot/dbpilot/internal/instance"
	"github.com/dbpilot/dbpilot/internal/provider"
)

// identifierPattern is deliberately conservative: lowercase letters, digits
// and underscores, no leading digit, up to 63 bytes (PostgreSQL NAMEDATALEN).
var identifierPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

const maxIdentifierLen = 63

func validateIdentifier(name, kind string) *APIError {
	if name == "" {
		return ErrBadRequest("invalid_name", kind+" name is required")
	}
	if len(name) > maxIdentifierLen {
		return ErrBadRequest("invalid_name", kind+" name too long")
	}
	if !identifierPattern.MatchString(name) {
		return ErrBadRequest("invalid_name", kind+" name must be lowercase letters, digits or underscore and not start with a digit")
	}
	return nil
}

func pathParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

func (s *Server) connParams(r *http.Request, instanceID int64) (provider.ConnParams, *APIError) {
	params, err := s.instances.ConnParams(r.Context(), instanceID)
	if err != nil {
		if errors.Is(err, instance.ErrNotFound) {
			return params, ErrNotFound("instance_not_found", "instance not found")
		}
		return params, ErrInternal(err)
	}
	return params, nil
}
