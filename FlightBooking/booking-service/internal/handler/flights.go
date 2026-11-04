package handler

import (
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	flightv1 "flightbooking/gen/flight/v1"
)

func (h *Handler) SearchFlights(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	req := &flightv1.SearchFlightsRequest{
		Origin:      query.Get("origin"),
		Destination: query.Get("destination"),
	}
	var fields []FieldError
	if req.Origin == "" {
		fields = append(fields, FieldError{Field: "origin", Message: "is required"})
	}
	if req.Destination == "" {
		fields = append(fields, FieldError{Field: "destination", Message: "is required"})
	}
	if date := query.Get("date"); date != "" {
		day, err := time.Parse(time.DateOnly, date)
		if err != nil {
			fields = append(fields, FieldError{Field: "date", Message: "must be in YYYY-MM-DD format"})
		} else {
			req.Date = timestamppb.New(day)
		}
	}
	if len(fields) > 0 {
		writeValidationError(w, fields)
		return
	}
	ctx, cancel := h.callCtx(r.Context())
	defer cancel()
	resp, err := h.flights.SearchFlights(ctx, req)
	if err != nil {
		writeFlightServiceError(w, r, err)
		return
	}
	flights := make([]flightJSON, 0, len(resp.GetFlights()))
	for _, f := range resp.GetFlights() {
		flights = append(flights, toFlightJSON(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"flights": flights})
}

func (h *Handler) GetFlight(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := h.callCtx(r.Context())
	defer cancel()
	resp, err := h.flights.GetFlight(ctx, &flightv1.GetFlightRequest{FlightId: r.PathValue("id")})
	if err != nil {
		writeFlightServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toFlightJSON(resp.GetFlight()))
}
