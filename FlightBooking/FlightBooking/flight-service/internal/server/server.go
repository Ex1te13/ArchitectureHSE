package server

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"flightbooking/flight-service/internal/repository"
	flightv1 "flightbooking/gen/flight/v1"
)

var (
	iataRe     = regexp.MustCompile(`^[A-Z]{3}$`)
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
)

type FlightStore interface {
	Search(ctx context.Context, filter repository.SearchFilter) ([]repository.Flight, error)
	GetByID(ctx context.Context, id uuid.UUID) (repository.Flight, error)
	ReserveSeats(ctx context.Context, flightID, bookingID uuid.UUID, seatCount int32) (repository.Reservation, error)
	ReleaseReservation(ctx context.Context, bookingID uuid.UUID) (repository.Reservation, error)
	Update(ctx context.Context, id uuid.UUID, upd repository.FlightUpdate) (repository.Flight, error)
}

type FlightServer struct {
	flightv1.UnimplementedFlightServiceServer
	repo FlightStore
}

func NewFlightServer(repo FlightStore) *FlightServer {
	return &FlightServer{repo: repo}
}

func (s *FlightServer) SearchFlights(ctx context.Context, req *flightv1.SearchFlightsRequest) (*flightv1.SearchFlightsResponse, error) {
	if !iataRe.MatchString(req.GetOrigin()) || !iataRe.MatchString(req.GetDestination()) {
		return nil, status.Error(codes.InvalidArgument, "origin and destination must be IATA codes (3 uppercase letters)")
	}
	if req.GetOrigin() == req.GetDestination() {
		return nil, status.Error(codes.InvalidArgument, "origin and destination must differ")
	}
	filter := repository.SearchFilter{Origin: req.GetOrigin(), Destination: req.GetDestination()}
	if req.Date != nil {
		if err := req.Date.CheckValid(); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid date")
		}
		t := req.Date.AsTime().UTC()
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		filter.Day = &day
	}

	flights, err := s.repo.Search(ctx, filter)
	if err != nil {
		return nil, internal(ctx, err)
	}
	resp := &flightv1.SearchFlightsResponse{Flights: make([]*flightv1.Flight, 0, len(flights))}
	for _, f := range flights {
		resp.Flights = append(resp.Flights, toProtoFlight(f))
	}
	return resp, nil
}

func (s *FlightServer) GetFlight(ctx context.Context, req *flightv1.GetFlightRequest) (*flightv1.GetFlightResponse, error) {
	id, err := parseUUID("flight_id", req.GetFlightId())
	if err != nil {
		return nil, err
	}
	f, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, status.Errorf(codes.NotFound, "flight %s not found", id)
	}
	if err != nil {
		return nil, internal(ctx, err)
	}
	return &flightv1.GetFlightResponse{Flight: toProtoFlight(f)}, nil
}

func (s *FlightServer) ReserveSeats(ctx context.Context, req *flightv1.ReserveSeatsRequest) (*flightv1.ReserveSeatsResponse, error) {
	flightID, err := parseUUID("flight_id", req.GetFlightId())
	if err != nil {
		return nil, err
	}
	bookingID, err := parseUUID("booking_id", req.GetBookingId())
	if err != nil {
		return nil, err
	}
	if req.GetSeatCount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "seat_count must be positive")
	}

	res, err := s.repo.ReserveSeats(ctx, flightID, bookingID, req.GetSeatCount())
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "flight %s not found", flightID)
	case errors.Is(err, repository.ErrFlightNotScheduled):
		return nil, status.Errorf(codes.FailedPrecondition, "flight %s is not scheduled", flightID)
	case errors.Is(err, repository.ErrNotEnoughSeats):
		return nil, status.Errorf(codes.ResourceExhausted, "not enough seats on flight %s", flightID)
	case errors.Is(err, repository.ErrReservationConflict):
		return nil, status.Errorf(codes.AlreadyExists, "booking %s already has a reservation with other parameters", bookingID)
	case err != nil:
		return nil, internal(ctx, err)
	}
	return &flightv1.ReserveSeatsResponse{Reservation: toProtoReservation(res)}, nil
}

func (s *FlightServer) ReleaseReservation(ctx context.Context, req *flightv1.ReleaseReservationRequest) (*flightv1.ReleaseReservationResponse, error) {
	bookingID, err := parseUUID("booking_id", req.GetBookingId())
	if err != nil {
		return nil, err
	}

	res, err := s.repo.ReleaseReservation(ctx, bookingID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "reservation for booking %s not found", bookingID)
	case errors.Is(err, repository.ErrReservationExpired):
		return nil, status.Errorf(codes.FailedPrecondition, "reservation for booking %s expired", bookingID)
	case err != nil:
		return nil, internal(ctx, err)
	}
	return &flightv1.ReleaseReservationResponse{Reservation: toProtoReservation(res)}, nil
}

func (s *FlightServer) UpdateFlight(ctx context.Context, req *flightv1.UpdateFlightRequest) (*flightv1.UpdateFlightResponse, error) {
	id, err := parseUUID("flight_id", req.GetFlightId())
	if err != nil {
		return nil, err
	}

	var upd repository.FlightUpdate
	if req.Status != nil {
		if req.GetStatus() == flightv1.FlightStatus_FLIGHT_STATUS_UNSPECIFIED {
			return nil, status.Error(codes.InvalidArgument, "status must be specified")
		}
		name := strings.TrimPrefix(req.GetStatus().String(), "FLIGHT_STATUS_")
		upd.Status = &name
	}
	if req.Price != nil {
		amount := req.GetPrice().GetAmount()
		if amount <= 0 {
			return nil, status.Error(codes.InvalidArgument, "price must be positive")
		}
		upd.PriceAmount = &amount
		if currency := req.GetPrice().GetCurrency(); currency != "" {
			if !currencyRe.MatchString(currency) {
				return nil, status.Error(codes.InvalidArgument, "currency must be an ISO 4217 code")
			}
			upd.Currency = &currency
		}
	}
	if req.DepartureTime != nil {
		if err := req.DepartureTime.CheckValid(); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid departure_time")
		}
		t := req.DepartureTime.AsTime()
		upd.DepartureTime = &t
	}
	if req.ArrivalTime != nil {
		if err := req.ArrivalTime.CheckValid(); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid arrival_time")
		}
		t := req.ArrivalTime.AsTime()
		upd.ArrivalTime = &t
	}

	f, err := s.repo.Update(ctx, id, upd)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "flight %s not found", id)
	case errors.Is(err, repository.ErrConstraint):
		return nil, status.Errorf(codes.InvalidArgument, "invalid update: %v", err)
	case err != nil:
		return nil, internal(ctx, err)
	}
	return &flightv1.UpdateFlightResponse{Flight: toProtoFlight(f)}, nil
}

func parseUUID(field, value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "%s must be a UUID", field)
	}
	return id, nil
}

func internal(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "internal error", slog.String("error", err.Error()))
	return status.Error(codes.Internal, "internal error")
}

func toProtoFlight(f repository.Flight) *flightv1.Flight {
	return &flightv1.Flight{
		Id:             f.ID.String(),
		FlightNumber:   f.AirlineCode + f.FlightNumber,
		AirlineCode:    f.AirlineCode,
		AirlineName:    f.AirlineName,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureTime:  timestamppb.New(f.DepartureTime),
		ArrivalTime:    timestamppb.New(f.ArrivalTime),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		Price:          &flightv1.Money{Amount: f.PriceAmount, Currency: f.Currency},
		Status:         flightv1.FlightStatus(flightv1.FlightStatus_value["FLIGHT_STATUS_"+f.Status]),
	}
}

func toProtoReservation(r repository.Reservation) *flightv1.Reservation {
	return &flightv1.Reservation{
		Id:        r.ID.String(),
		BookingId: r.BookingID.String(),
		FlightId:  r.FlightID.String(),
		SeatCount: r.SeatCount,
		Status:    flightv1.ReservationStatus(flightv1.ReservationStatus_value["RESERVATION_STATUS_"+r.Status]),
		CreatedAt: timestamppb.New(r.CreatedAt),
		UpdatedAt: timestamppb.New(r.UpdatedAt),
	}
}
