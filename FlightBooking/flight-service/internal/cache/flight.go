package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"flightbooking/flight-service/internal/repository"
)

type FlightRepository struct {
	repo *repository.FlightRepository
	rdb  redis.UniversalClient
	ttl  time.Duration
}

func NewFlightRepository(repo *repository.FlightRepository, rdb redis.UniversalClient, ttl time.Duration) *FlightRepository {
	return &FlightRepository{repo: repo, rdb: rdb, ttl: ttl}
}

func flightKey(id uuid.UUID) string {
	return "flight:" + id.String()
}

func searchKey(origin, destination string, day *time.Time) string {
	date := "any"
	if day != nil {
		date = day.UTC().Format(time.DateOnly)
	}
	return fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
}

func (c *FlightRepository) GetByID(ctx context.Context, id uuid.UUID) (repository.Flight, error) {
	key := flightKey(id)
	var f repository.Flight
	hit, available := c.get(ctx, key, &f)
	if hit {
		return f, nil
	}
	f, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return repository.Flight{}, err
	}
	if available {
		c.set(ctx, key, f)
	}
	return f, nil
}

func (c *FlightRepository) Search(ctx context.Context, filter repository.SearchFilter) ([]repository.Flight, error) {
	key := searchKey(filter.Origin, filter.Destination, filter.Day)
	var flights []repository.Flight
	hit, available := c.get(ctx, key, &flights)
	if hit {
		return flights, nil
	}
	flights, err := c.repo.Search(ctx, filter)
	if err != nil {
		return nil, err
	}
	if available {
		c.set(ctx, key, flights)
	}
	return flights, nil
}

func (c *FlightRepository) ReserveSeats(ctx context.Context, flightID, bookingID uuid.UUID, seatCount int32) (repository.Reservation, error) {
	res, err := c.repo.ReserveSeats(ctx, flightID, bookingID, seatCount)
	if err != nil {
		return repository.Reservation{}, err
	}
	c.invalidateFlight(ctx, flightID)
	return res, nil
}

func (c *FlightRepository) ReleaseReservation(ctx context.Context, bookingID uuid.UUID) (repository.Reservation, error) {
	res, err := c.repo.ReleaseReservation(ctx, bookingID)
	if err != nil {
		return repository.Reservation{}, err
	}
	c.invalidateFlight(ctx, res.FlightID)
	return res, nil
}

func (c *FlightRepository) Update(ctx context.Context, id uuid.UUID, upd repository.FlightUpdate) (repository.Flight, error) {
	before, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return repository.Flight{}, err
	}
	after, err := c.repo.Update(ctx, id, upd)
	if err != nil {
		return repository.Flight{}, err
	}
	c.delete(ctx, append(flightKeys(before), flightKeys(after)...))
	return after, nil
}

func (c *FlightRepository) invalidateFlight(ctx context.Context, id uuid.UUID) {
	f, err := c.repo.GetByID(ctx, id)
	if err != nil {
		slog.WarnContext(ctx, "cache invalidation: cannot load flight",
			slog.String("flight_id", id.String()), slog.String("error", err.Error()))
		c.delete(ctx, []string{flightKey(id)})
		return
	}
	c.delete(ctx, flightKeys(f))
}

func flightKeys(f repository.Flight) []string {
	day := f.DepartureTime
	return []string{
		flightKey(f.ID),
		searchKey(f.Origin, f.Destination, &day),
		searchKey(f.Origin, f.Destination, nil),
	}
}

func (c *FlightRepository) get(ctx context.Context, key string, dst any) (hit, available bool) {
	data, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		slog.InfoContext(ctx, "cache miss", slog.String("key", key))
		return false, true
	}
	if err != nil {
		slog.WarnContext(ctx, "cache get failed", slog.String("key", key), slog.String("error", err.Error()))
		return false, false
	}
	if err := json.Unmarshal(data, dst); err != nil {
		slog.WarnContext(ctx, "cache value is corrupted", slog.String("key", key), slog.String("error", err.Error()))
		return false, true
	}
	slog.InfoContext(ctx, "cache hit", slog.String("key", key))
	return true, true
}

func (c *FlightRepository) set(ctx context.Context, key string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		slog.WarnContext(ctx, "cache marshal failed", slog.String("key", key), slog.String("error", err.Error()))
		return
	}
	if err := c.rdb.Set(ctx, key, data, c.ttl).Err(); err != nil {
		slog.WarnContext(ctx, "cache set failed", slog.String("key", key), slog.String("error", err.Error()))
	}
}

func (c *FlightRepository) delete(ctx context.Context, keys []string) {
	ctx = context.WithoutCancel(ctx)
	for _, key := range keys {
		if err := c.rdb.Del(ctx, key).Err(); err != nil {
			slog.WarnContext(ctx, "cache delete failed", slog.String("key", key), slog.String("error", err.Error()))
			return
		}
	}
	slog.InfoContext(ctx, "cache invalidated", slog.Any("keys", keys))
}
