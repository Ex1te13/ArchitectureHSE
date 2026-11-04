package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"flightbooking/booking-service/internal/repository"
	flightv1 "flightbooking/gen/flight/v1"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type createBookingRequest struct {
	UserID         string `json:"user_id"`
	FlightID       string `json:"flight_id"`
	PassengerName  string `json:"passenger_name"`
	PassengerEmail string `json:"passenger_email"`
	SeatCount      int32  `json:"seat_count"`
}

func (req createBookingRequest) validate() []FieldError {
	var fields []FieldError
	if strings.TrimSpace(req.UserID) == "" {
		fields = append(fields, FieldError{Field: "user_id", Message: "is required"})
	}
	if _, err := uuid.Parse(req.FlightID); err != nil {
		fields = append(fields, FieldError{Field: "flight_id", Message: "must be a UUID"})
	}
	if strings.TrimSpace(req.PassengerName) == "" {
		fields = append(fields, FieldError{Field: "passenger_name", Message: "is required"})
	}
	if !emailRe.MatchString(req.PassengerEmail) {
		fields = append(fields, FieldError{Field: "passenger_email", Message: "must be a valid email"})
	}
	if req.SeatCount <= 0 {
		fields = append(fields, FieldError{Field: "seat_count", Message: "must be positive"})
	}
	return fields
}

func (h *Handler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	var req createBookingRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeValidationError(w, []FieldError{{Field: "body", Message: err.Error()}})
		return
	}
	if fields := req.validate(); len(fields) > 0 {
		writeValidationError(w, fields)
		return
	}
	bookingID := uuid.New()
	getCtx, cancelGet := h.callCtx(r.Context())
	defer cancelGet()
	flightResp, err := h.flights.GetFlight(getCtx, &flightv1.GetFlightRequest{FlightId: req.FlightID})
	if err != nil {
		writeFlightServiceError(w, r, err)
		return
	}
	price := flightResp.GetFlight().GetPrice()
	reserveCtx, cancelReserve := h.callCtx(r.Context())
	defer cancelReserve()
	_, err = h.flights.ReserveSeats(reserveCtx, &flightv1.ReserveSeatsRequest{
		FlightId:  req.FlightID,
		BookingId: bookingID.String(),
		SeatCount: req.SeatCount,
	})
	if err != nil {
		if outcomeUnknown(err) {
			h.releaseReservation(r.Context(), bookingID)
		}
		writeFlightServiceError(w, r, err)
		return
	}
	booking, err := h.repo.Create(r.Context(), repository.Booking{
		ID:             bookingID,
		UserID:         req.UserID,
		FlightID:       uuid.MustParse(req.FlightID),
		PassengerName:  strings.TrimSpace(req.PassengerName),
		PassengerEmail: req.PassengerEmail,
		SeatCount:      req.SeatCount,
		TotalPrice:     int64(req.SeatCount) * price.GetAmount(),
		Currency:       price.GetCurrency(),
	})
	if err != nil {
		h.releaseReservation(r.Context(), bookingID)
		writeInternalError(w, r, err)
		return
	}
	w.Header().Set("Location", "/bookings/"+booking.ID.String())
	writeJSON(w, http.StatusCreated, toBookingJSON(booking))
}

func (h *Handler) GetBooking(w http.ResponseWriter, r *http.Request) {
	id, ok := parseBookingID(w, r)
	if !ok {
		return
	}
	booking, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "BOOKING_NOT_FOUND", "booking not found")
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBookingJSON(booking))
}

func (h *Handler) ListBookings(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if strings.TrimSpace(userID) == "" {
		writeValidationError(w, []FieldError{{Field: "user_id", Message: "is required"}})
		return
	}
	bookings, err := h.repo.ListByUser(r.Context(), userID)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	result := make([]bookingJSON, 0, len(bookings))
	for _, b := range bookings {
		result = append(result, toBookingJSON(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"bookings": result})
}

func (h *Handler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	id, ok := parseBookingID(w, r)
	if !ok {
		return
	}
	booking, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusNotFound, "BOOKING_NOT_FOUND", "booking not found")
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	if booking.Status != "CONFIRMED" {
		writeError(w, http.StatusConflict, "BOOKING_NOT_CANCELLABLE", "only CONFIRMED booking can be cancelled")
		return
	}
	ctx, cancel := h.callCtx(r.Context())
	defer cancel()
	_, err = h.flights.ReleaseReservation(ctx, &flightv1.ReleaseReservationRequest{BookingId: id.String()})
	switch {
	case status.Code(err) == codes.NotFound:
		slog.WarnContext(r.Context(), "reservation not found, cancelling booking anyway",
			slog.String("booking_id", id.String()))
	case err != nil:
		writeFlightServiceError(w, r, err)
		return
	}
	cancelled, err := h.repo.Cancel(r.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		writeError(w, http.StatusConflict, "BOOKING_NOT_CANCELLABLE", "only CONFIRMED booking can be cancelled")
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toBookingJSON(cancelled))
}

func parseBookingID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeValidationError(w, []FieldError{{Field: "id", Message: "must be a UUID"}})
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) releaseReservation(ctx context.Context, bookingID uuid.UUID) {
	ctx, cancel := h.callCtx(context.WithoutCancel(ctx))
	defer cancel()
	_, err := h.flights.ReleaseReservation(ctx, &flightv1.ReleaseReservationRequest{BookingId: bookingID.String()})
	if status.Code(err) == codes.NotFound {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "compensation failed: reservation not released",
			slog.String("booking_id", bookingID.String()),
			slog.String("error", err.Error()),
		)
		return
	}
	slog.InfoContext(ctx, "compensation: reservation released", slog.String("booking_id", bookingID.String()))
}

func outcomeUnknown(err error) bool {
	switch status.Code(err) {
	case codes.DeadlineExceeded, codes.Unavailable, codes.Canceled, codes.Unknown:
		return true
	}
	return false
}
