package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"flightbooking/booking-service/internal/repository"
	flightv1 "flightbooking/gen/flight/v1"
)

type Handler struct {
	repo    *repository.BookingRepository
	flights flightv1.FlightServiceClient
	timeout time.Duration
}

func New(repo *repository.BookingRepository, flights flightv1.FlightServiceClient, timeout time.Duration) *Handler {
	return &Handler{repo: repo, flights: flights, timeout: timeout}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /flights", h.SearchFlights)
	mux.HandleFunc("GET /flights/{id}", h.GetFlight)
	mux.HandleFunc("POST /bookings", h.CreateBooking)
	mux.HandleFunc("GET /bookings", h.ListBookings)
	mux.HandleFunc("GET /bookings/{id}", h.GetBooking)
	mux.HandleFunc("POST /bookings/{id}/cancel", h.CancelBooking)
	return Logging(mux)
}

func (h *Handler) callCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, h.timeout)
}

type moneyJSON struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type flightJSON struct {
	ID             string    `json:"id"`
	FlightNumber   string    `json:"flight_number"`
	AirlineCode    string    `json:"airline_code"`
	AirlineName    string    `json:"airline_name"`
	Origin         string    `json:"origin"`
	Destination    string    `json:"destination"`
	DepartureTime  time.Time `json:"departure_time"`
	ArrivalTime    time.Time `json:"arrival_time"`
	TotalSeats     int32     `json:"total_seats"`
	AvailableSeats int32     `json:"available_seats"`
	Price          moneyJSON `json:"price"`
	Status         string    `json:"status"`
}

type bookingJSON struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	FlightID       string    `json:"flight_id"`
	PassengerName  string    `json:"passenger_name"`
	PassengerEmail string    `json:"passenger_email"`
	SeatCount      int32     `json:"seat_count"`
	TotalPrice     moneyJSON `json:"total_price"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func toFlightJSON(f *flightv1.Flight) flightJSON {
	return flightJSON{
		ID:             f.GetId(),
		FlightNumber:   f.GetFlightNumber(),
		AirlineCode:    f.GetAirlineCode(),
		AirlineName:    f.GetAirlineName(),
		Origin:         f.GetOrigin(),
		Destination:    f.GetDestination(),
		DepartureTime:  f.GetDepartureTime().AsTime(),
		ArrivalTime:    f.GetArrivalTime().AsTime(),
		TotalSeats:     f.GetTotalSeats(),
		AvailableSeats: f.GetAvailableSeats(),
		Price:          moneyJSON{Amount: f.GetPrice().GetAmount(), Currency: f.GetPrice().GetCurrency()},
		Status:         strings.TrimPrefix(f.GetStatus().String(), "FLIGHT_STATUS_"),
	}
}

func toBookingJSON(b repository.Booking) bookingJSON {
	return bookingJSON{
		ID:             b.ID.String(),
		UserID:         b.UserID,
		FlightID:       b.FlightID.String(),
		PassengerName:  b.PassengerName,
		PassengerEmail: b.PassengerEmail,
		SeatCount:      b.SeatCount,
		TotalPrice:     moneyJSON{Amount: b.TotalPrice, Currency: b.Currency},
		Status:         b.Status,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}
