package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"flightbooking/booking-service/internal/flightclient"
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type errorResponse struct {
	ErrorCode string         `json:"error_code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{ErrorCode: code, Message: message})
}

func writeValidationError(w http.ResponseWriter, fields []FieldError) {
	writeJSON(w, http.StatusBadRequest, errorResponse{
		ErrorCode: "VALIDATION_ERROR",
		Message:   "request validation failed",
		Details:   map[string]any{"fields": fields},
	})
}

func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "internal error",
		slog.String("request_id", RequestID(r.Context())),
		slog.String("error", err.Error()),
	)
	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func writeFlightServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, flightclient.ErrCircuitOpen) {
		writeError(w, http.StatusServiceUnavailable, "FLIGHT_SERVICE_CIRCUIT_OPEN", status.Convert(err).Message())
		return
	}
	st := status.Convert(err)
	switch st.Code() {
	case codes.InvalidArgument:
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", st.Message())
	case codes.NotFound:
		writeError(w, http.StatusNotFound, "FLIGHT_NOT_FOUND", st.Message())
	case codes.ResourceExhausted:
		writeError(w, http.StatusConflict, "NOT_ENOUGH_SEATS", st.Message())
	case codes.FailedPrecondition:
		writeError(w, http.StatusConflict, "FLIGHT_NOT_AVAILABLE", st.Message())
	case codes.AlreadyExists:
		writeError(w, http.StatusConflict, "RESERVATION_CONFLICT", st.Message())
	case codes.Unavailable:
		writeError(w, http.StatusServiceUnavailable, "FLIGHT_SERVICE_UNAVAILABLE", "flight service is unavailable")
	case codes.DeadlineExceeded:
		writeError(w, http.StatusGatewayTimeout, "FLIGHT_SERVICE_TIMEOUT", "flight service did not respond in time")
	default:
		writeInternalError(w, r, err)
	}
}
