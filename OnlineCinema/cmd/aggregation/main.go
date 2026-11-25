package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"onlinecinema/internal/aggregation"
	"onlinecinema/internal/export"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	service := aggregation.Service{DB: db, ClickHouseURL: os.Getenv("CLICKHOUSE_URL"), User: os.Getenv("CLICKHOUSE_USER"), Password: os.Getenv("CLICKHOUSE_PASSWORD")}
	exporter, err := export.New(db, os.Getenv("S3_ENDPOINT"), os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	interval, err := time.ParseDuration(os.Getenv("AGGREGATION_INTERVAL"))
	if err != nil || interval <= 0 {
		log.Fatal("AGGREGATION_INTERVAL must be a positive duration")
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			today := time.Now().UTC()
			ready := true
			for _, day := range []time.Time{today.AddDate(0, 0, -1), today} {
				if ctx.Err() != nil {
					return
				}
				_, runErr := service.Run(ctx, day)
				if runErr != nil {
					ready = false
				}
			}
			if ready {
				if err := exporter.Pending(ctx); err != nil {
					slog.Error("scheduled export failed; will retry next cycle", "error", err)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /recalculate", func(w http.ResponseWriter, r *http.Request) {
		day, err := time.Parse(time.DateOnly, r.URL.Query().Get("date"))
		if err != nil {
			http.Error(w, "date must be YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		if day.Unix() < 0 || day.Unix()/86400 > 65535 {
			http.Error(w, "date is outside the ClickHouse Date range", http.StatusBadRequest)
			return
		}
		processed, err := service.Run(r.Context(), day)
		if err != nil {
			http.Error(w, "aggregation failed", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"date": day.Format(time.DateOnly), "events_processed": processed})
	})
	mux.HandleFunc("POST /export", func(w http.ResponseWriter, r *http.Request) {
		day := r.URL.Query().Get("date")
		if _, err := time.Parse(time.DateOnly, day); err != nil {
			http.Error(w, "date must be YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		key, err := exporter.Write(r.Context(), day, false)
		if errors.Is(err, export.ErrNoMetrics) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("manual export failed", "date", day, "error", err)
			http.Error(w, "export failed", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": key})
	})
	server := &http.Server{Addr: ":8082", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	slog.Info("aggregation service started", "interval", interval.String())
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
