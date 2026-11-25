package aggregation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed daily.sql
var dailySQL string

type Service struct {
	DB            *pgxpool.Pool
	ClickHouseURL string
	User          string
	Password      string
}

type metricRow struct {
	Metric     string    `json:"metric"`
	MovieID    string    `json:"movie_id"`
	Value      *float64  `json:"value"`
	ComputedAt time.Time `json:"computed_at"`
}

func (s *Service) query(ctx context.Context, day, sql string) ([]byte, error) {
	params := url.Values{"param_day": {day}, "date_time_output_format": {"iso"}, "wait_end_of_query": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.ClickHouseURL+"/?"+params.Encode(), strings.NewReader(sql))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(s.User, s.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ClickHouse %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return body, nil
}

func (s *Service) recalculate(ctx context.Context, day string) (int64, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(($1::date - DATE '1970-01-01')::bigint)", day); err != nil {
		return 0, err
	}
	for _, sql := range []string{
		"ALTER TABLE cinema.daily_metrics_stage DROP PARTITION {day:Date}",
		dailySQL,
		"ALTER TABLE cinema.daily_metrics REPLACE PARTITION {day:Date} FROM cinema.daily_metrics_stage",
		"ALTER TABLE cinema.daily_metrics_stage DROP PARTITION {day:Date}",
	} {
		if _, err := s.query(ctx, day, sql); err != nil {
			return 0, err
		}
	}
	body, err := s.query(ctx, day, "SELECT metric, movie_id, value, computed_at FROM cinema.daily_metrics WHERE day = {day:Date} FORMAT JSON")
	if err != nil {
		return 0, err
	}
	var result struct {
		Data []metricRow `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM daily_metrics WHERE day = $1::date", day); err != nil {
		return 0, err
	}
	var processed int64
	for _, row := range result.Data {
		if row.Metric == "events_processed" {
			processed = int64(*row.Value)
		}
		_, err := tx.Exec(ctx, "INSERT INTO daily_metrics(day, metric, movie_id, value, computed_at) VALUES ($1::date,$2,$3,$4,$5)", day, row.Metric, row.MovieID, row.Value, row.ComputedAt)
		if err != nil {
			return processed, err
		}
	}
	return processed, tx.Commit(ctx)
}

func (s *Service) Run(ctx context.Context, date time.Time) (int64, error) {
	day := date.Format(time.DateOnly)
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	slog.Info("aggregation started", "date", day)
	var processed int64
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		processed, err = s.recalculate(ctx, day)
		if err == nil {
			break
		}
		slog.Error("aggregation attempt failed", "date", day, "attempt", attempt, "error", err)
		if attempt < 3 && ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		if ctx.Err() != nil {
			break
		}
	}
	slog.Info("aggregation finished", "date", day, "events", processed, "duration", time.Since(start), "success", err == nil)
	return processed, err
}
