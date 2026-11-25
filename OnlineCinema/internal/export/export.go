package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const bucket = "movie-analytics"

var ErrNoMetrics = errors.New("no metrics for this date")

type Exporter struct {
	db *pgxpool.Pool
	s3 *minio.Client
	mu sync.Mutex
}

func New(db *pgxpool.Pool, endpoint, accessKey, secretKey string) (*Exporter, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Region: "us-east-1",
	})
	if err != nil {
		return nil, err
	}
	return &Exporter{db: db, s3: client}, nil
}

func (e *Exporter) Write(ctx context.Context, day string, onlyMissing bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	exists, err := e.s3.BucketExists(ctx, bucket)
	if err != nil {
		return "", err
	}
	if !exists {
		if err := e.s3.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return "", err
		}
	}
	key := "daily/" + day + "/aggregates.csv"
	if onlyMissing {
		_, err := e.s3.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
		if err == nil {
			return key, nil
		}
		if minio.ToErrorResponse(err).StatusCode != http.StatusNotFound {
			return "", err
		}
	}
	rows, err := e.db.Query(ctx, "SELECT metric, movie_id, value, computed_at FROM daily_metrics WHERE day=$1::date ORDER BY metric,movie_id", day)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var body bytes.Buffer
	w := csv.NewWriter(&body)
	w.Write([]string{"date", "metric", "movie_id", "value", "computed_at"})
	count := 0
	for rows.Next() {
		var metric, movieID string
		var value *float64
		var computed time.Time
		if err := rows.Scan(&metric, &movieID, &value, &computed); err != nil {
			return "", err
		}
		number := ""
		if value != nil {
			number = strconv.FormatFloat(*value, 'g', -1, 64)
		}
		w.Write([]string{day, metric, movieID, number, computed.UTC().Format(time.RFC3339Nano)})
		count++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if count == 0 {
		return "", ErrNoMetrics
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	_, err = e.s3.PutObject(ctx, bucket, key, &body, int64(body.Len()), minio.PutObjectOptions{ContentType: "text/csv; charset=utf-8"})
	if err != nil {
		return "", err
	}
	slog.Info("daily report exported", "date", day, "metrics", count, "key", key)
	return key, nil
}

func (e *Exporter) Pending(ctx context.Context) error {
	rows, err := e.db.Query(ctx, "SELECT DISTINCT day::text FROM daily_metrics WHERE day < (now() AT TIME ZONE 'UTC')::date ORDER BY 1")
	if err != nil {
		return err
	}
	var days []string
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			rows.Close()
			return err
		}
		days = append(days, day)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, day := range days {
		if _, err := e.Write(ctx, day, true); err != nil {
			return fmt.Errorf("export %s: %w", day, err)
		}
	}
	return nil
}
