package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"marketplace/internal/api"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, status int, code api.ErrorCode, message string, details map[string]any) {
	body := api.Error{ErrorCode: code, Message: message}
	if details != nil {
		body.Details = &details
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeValidationError(w http.ResponseWriter, fields []FieldError) {
	WriteError(w, http.StatusBadRequest, api.VALIDATIONERROR, "request validation failed",
		map[string]any{"fields": fields})
}

func BadRequestHandler(w http.ResponseWriter, r *http.Request, err error) {
	writeValidationError(w, []FieldError{{Field: "request", Message: err.Error()}})
}

func InternalErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal error",
		slog.String("request_id", RequestID(r.Context())),
		slog.String("error", err.Error()),
	)
	WriteError(w, http.StatusInternalServerError, api.INTERNALERROR, "internal server error", nil)
}
