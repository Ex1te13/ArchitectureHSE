package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrFlightNotScheduled  = errors.New("flight is not scheduled")
	ErrNotEnoughSeats      = errors.New("not enough seats")
	ErrReservationConflict = errors.New("reservation for booking already exists with other parameters")
	ErrReservationExpired  = errors.New("reservation expired")
	ErrConstraint          = errors.New("constraint violation")
)

const (
	pgUniqueViolation = "23505"
	pgCheckViolation  = "23514"
)

type Flight struct {
	ID             uuid.UUID
	AirlineCode    string
	AirlineName    string
	FlightNumber   string
	Origin         string
	Destination    string
	DepartureTime  time.Time
	ArrivalTime    time.Time
	TotalSeats     int32
	AvailableSeats int32
	PriceAmount    int64
	Currency       string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Reservation struct {
	ID        uuid.UUID
	BookingID uuid.UUID
	FlightID  uuid.UUID
	SeatCount int32
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SearchFilter struct {
	Origin      string
	Destination string
	Day         *time.Time
}

type FlightUpdate struct {
	Status        *string
	PriceAmount   *int64
	Currency      *string
	DepartureTime *time.Time
	ArrivalTime   *time.Time
}

type FlightRepository struct {
	pool *pgxpool.Pool
}

func NewFlightRepository(pool *pgxpool.Pool) *FlightRepository {
	return &FlightRepository{pool: pool}
}

const flightColumns = `f.id, f.airline_code, a.name, f.flight_number, f.origin, f.destination,
	f.departure_time, f.arrival_time, f.total_seats, f.available_seats,
	f.price_amount, f.currency, f.status::text, f.created_at, f.updated_at`

const reservationColumns = `id, booking_id, flight_id, seat_count, status::text, created_at, updated_at`

func scanFlight(row pgx.Row) (Flight, error) {
	var f Flight
	err := row.Scan(&f.ID, &f.AirlineCode, &f.AirlineName, &f.FlightNumber, &f.Origin, &f.Destination,
		&f.DepartureTime, &f.ArrivalTime, &f.TotalSeats, &f.AvailableSeats,
		&f.PriceAmount, &f.Currency, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func scanReservation(row pgx.Row) (Reservation, error) {
	var r Reservation
	err := row.Scan(&r.ID, &r.BookingID, &r.FlightID, &r.SeatCount, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (r *FlightRepository) Search(ctx context.Context, filter SearchFilter) ([]Flight, error) {
	q := `SELECT ` + flightColumns + ` FROM flights f JOIN airlines a ON a.code = f.airline_code
		WHERE f.origin = $1 AND f.destination = $2 AND f.status = 'SCHEDULED'`
	args := []any{filter.Origin, filter.Destination}
	if filter.Day != nil {
		q += ` AND f.departure_time >= $3 AND f.departure_time < $4`
		args = append(args, *filter.Day, filter.Day.Add(24*time.Hour))
	}
	q += ` ORDER BY f.departure_time`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search flights: %w", err)
	}
	flights, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Flight, error) {
		return scanFlight(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan flights: %w", err)
	}
	return flights, nil
}

func (r *FlightRepository) GetByID(ctx context.Context, id uuid.UUID) (Flight, error) {
	q := `SELECT ` + flightColumns + ` FROM flights f JOIN airlines a ON a.code = f.airline_code WHERE f.id = $1`
	f, err := scanFlight(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Flight{}, ErrNotFound
	}
	if err != nil {
		return Flight{}, fmt.Errorf("select flight: %w", err)
	}
	return f, nil
}

func (r *FlightRepository) ReserveSeats(ctx context.Context, flightID, bookingID uuid.UUID, seatCount int32) (Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	var available int32
	var flightStatus string
	err = tx.QueryRow(ctx,
		`SELECT available_seats, status::text FROM flights WHERE id = $1 FOR UPDATE`, flightID,
	).Scan(&available, &flightStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, fmt.Errorf("lock flight: %w", err)
	}
	existing, err := scanReservation(tx.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM seat_reservations WHERE booking_id = $1`, bookingID))
	switch {
	case err == nil:
		if existing.FlightID == flightID && existing.SeatCount == seatCount {
			return existing, nil
		}
		return Reservation{}, ErrReservationConflict
	case !errors.Is(err, pgx.ErrNoRows):
		return Reservation{}, fmt.Errorf("select reservation: %w", err)
	}
	if flightStatus != "SCHEDULED" {
		return Reservation{}, ErrFlightNotScheduled
	}
	if available < seatCount {
		return Reservation{}, ErrNotEnoughSeats
	}
	if _, err := tx.Exec(ctx,
		`UPDATE flights SET available_seats = available_seats - $2, updated_at = now() WHERE id = $1`,
		flightID, seatCount,
	); err != nil {
		return Reservation{}, fmt.Errorf("decrease seats: %w", err)
	}
	res, err := scanReservation(tx.QueryRow(ctx,
		`INSERT INTO seat_reservations (flight_id, booking_id, seat_count) VALUES ($1, $2, $3)
		RETURNING `+reservationColumns,
		flightID, bookingID, seatCount))
	if isPgError(err, pgUniqueViolation) {
		return Reservation{}, ErrReservationConflict
	}
	if err != nil {
		return Reservation{}, fmt.Errorf("insert reservation: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, fmt.Errorf("commit: %w", err)
	}
	return res, nil
}

func (r *FlightRepository) ReleaseReservation(ctx context.Context, bookingID uuid.UUID) (Reservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	res, err := scanReservation(tx.QueryRow(ctx,
		`SELECT `+reservationColumns+` FROM seat_reservations WHERE booking_id = $1 FOR UPDATE`, bookingID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrNotFound
	}
	if err != nil {
		return Reservation{}, fmt.Errorf("lock reservation: %w", err)
	}
	switch res.Status {
	case "RELEASED":
		return res, nil
	case "EXPIRED":
		return Reservation{}, ErrReservationExpired
	}
	if _, err := tx.Exec(ctx,
		`UPDATE flights SET available_seats = available_seats + $2, updated_at = now() WHERE id = $1`,
		res.FlightID, res.SeatCount,
	); err != nil {
		return Reservation{}, fmt.Errorf("return seats: %w", err)
	}
	released, err := scanReservation(tx.QueryRow(ctx,
		`UPDATE seat_reservations SET status = 'RELEASED', updated_at = now() WHERE id = $1
		RETURNING `+reservationColumns,
		res.ID))
	if err != nil {
		return Reservation{}, fmt.Errorf("release reservation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, fmt.Errorf("commit: %w", err)
	}
	return released, nil
}

func (r *FlightRepository) Update(ctx context.Context, id uuid.UUID, upd FlightUpdate) (Flight, error) {
	var sets []string
	args := []any{id}
	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if upd.Status != nil {
		args = append(args, *upd.Status)
		sets = append(sets, fmt.Sprintf("status = $%d::flight_status", len(args)))
	}
	if upd.PriceAmount != nil {
		add("price_amount", *upd.PriceAmount)
	}
	if upd.Currency != nil {
		add("currency", *upd.Currency)
	}
	if upd.DepartureTime != nil {
		add("departure_time", *upd.DepartureTime)
	}
	if upd.ArrivalTime != nil {
		add("arrival_time", *upd.ArrivalTime)
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id)
	}

	q := `WITH f AS (
			UPDATE flights SET ` + strings.Join(sets, ", ") + `, updated_at = now() WHERE id = $1 RETURNING *
		)
		SELECT ` + flightColumns + ` FROM f JOIN airlines a ON a.code = f.airline_code`
	f, err := scanFlight(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Flight{}, ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == pgCheckViolation || pgErr.Code == pgUniqueViolation) {
		return Flight{}, fmt.Errorf("%w: %s", ErrConstraint, pgErr.ConstraintName)
	}
	if err != nil {
		return Flight{}, fmt.Errorf("update flight: %w", err)
	}
	return f, nil
}

func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
