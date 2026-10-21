package middleware

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"marketplace/internal/api"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, status int, code api.ErrorCode, message string, details map[string]interface{}) {
	body := api.Error{ErrorCode: code, Message: message}
	if details != nil {
		body.Details = &details
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write error response: %v", err)
	}
}

func writeValidationError(w http.ResponseWriter, fields []FieldError) {
	WriteError(w, http.StatusBadRequest, api.VALIDATIONERROR, "request validation failed",
		map[string]interface{}{"fields": fields})
}

func ParamErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	field := ""
	var invalid *api.InvalidParamFormatError
	if errors.As(err, &invalid) {
		field = invalid.ParamName
	}
	writeValidationError(w, []FieldError{{Field: field, Message: err.Error()}})
}

func RequestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	writeValidationError(w, []FieldError{{Field: "body", Message: err.Error()}})
}

func ResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("internal error: %s %s: %v", r.Method, r.URL.Path, err)
	WriteError(w, http.StatusInternalServerError, api.INTERNALERROR, "internal server error", nil)
}
