package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type APIError struct {
	Code     string
	Message  string
	Status   int
	Internal error
}

func (e *APIError) Error() string {
	if e.Internal != nil {
		return e.Message + ": " + e.Internal.Error()
	}
	return e.Message
}

func NewError(status int, code, message string) *APIError {
	return &APIError{Code: code, Message: message, Status: status}
}

func ErrBadRequest(code, message string) *APIError {
	return NewError(http.StatusBadRequest, code, message)
}

func ErrUnauthorized() *APIError {
	return NewError(http.StatusUnauthorized, "unauthorized", "unauthorized")
}

func ErrForbidden() *APIError {
	return NewError(http.StatusForbidden, "forbidden", "forbidden")
}

func ErrNotFound(code, message string) *APIError {
	return NewError(http.StatusNotFound, code, message)
}

func ErrConflict(code, message string) *APIError {
	return NewError(http.StatusConflict, code, message)
}

func ErrInternal(err error) *APIError {
	return &APIError{Code: "internal_error", Message: "internal error", Status: http.StatusInternalServerError, Internal: err}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, data)
}

func writeError(w http.ResponseWriter, err error) {
	apiErr, ok := err.(*APIError)
	if !ok {
		slog.Error("unexpected error", "error", err)
		apiErr = ErrInternal(err)
	}
	if apiErr.Status >= 500 {
		slog.Error("api error", "code", apiErr.Code, "status", apiErr.Status, "error", apiErr.Internal)
	}
	writeJSON(w, apiErr.Status, ErrorBody{Code: apiErr.Code, Message: apiErr.Message})
}
