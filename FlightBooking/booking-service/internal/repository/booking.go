package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Booking struct {
	ID             uuid.UUID
	UserID         string
	FlightID       uuid.UUID
	PassengerName  string
	PassengerEmail string
	SeatCount      int32
	TotalPrice     int64
	Currency       string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type BookingRepository struct {
	pool *pgxpool.Pool
}

func NewBookingRepository(pool *pgxpool.Pool) *BookingRepository {
	return &BookingRepository{pool: pool}
}

const bookingColumns = `id, user_id, flight_id, passenger_name, passenger_email, seat_count,
	total_price, currency, status::text, created_at, updated_at`

func scanBooking(row pgx.Row) (Booking, error) {
	var b Booking
	err := row.Scan(&b.ID, &b.UserID, &b.FlightID, &b.PassengerName, &b.PassengerEmail, &b.SeatCount,
		&b.TotalPrice, &b.Currency, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

func (r *BookingRepository) Create(ctx context.Context, b Booking) (Booking, error) {
	q := `
		INSERT INTO bookings (id, user_id, flight_id, passenger_name, passenger_email, seat_count, total_price, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + bookingColumns
	created, err := scanBooking(r.pool.QueryRow(ctx, q,
		b.ID, b.UserID, b.FlightID, b.PassengerName, b.PassengerEmail, b.SeatCount, b.TotalPrice, b.Currency))
	if err != nil {
		return Booking{}, fmt.Errorf("insert booking: %w", err)
	}
	return created, nil
}

func (r *BookingRepository) GetByID(ctx context.Context, id uuid.UUID) (Booking, error) {
	b, err := scanBooking(r.pool.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("select booking: %w", err)
	}
	return b, nil
}

func (r *BookingRepository) ListByUser(ctx context.Context, userID string) ([]Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+bookingColumns+` FROM bookings WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("select bookings: %w", err)
	}
	bookings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Booking, error) {
		return scanBooking(row)
	})
	if err != nil {
		return nil, fmt.Errorf("scan bookings: %w", err)
	}
	return bookings, nil
}

func (r *BookingRepository) Cancel(ctx context.Context, id uuid.UUID) (Booking, error) {
	b, err := scanBooking(r.pool.QueryRow(ctx,
		`UPDATE bookings SET status = 'CANCELLED', updated_at = now()
		WHERE id = $1 AND status = 'CONFIRMED'
		RETURNING `+bookingColumns, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("cancel booking: %w", err)
	}
	return b, nil
}
