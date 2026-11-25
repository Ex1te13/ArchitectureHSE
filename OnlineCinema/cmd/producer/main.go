package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"onlinecinema/internal/producer"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	brokers := os.Getenv("KAFKA_BROKERS")
	registryURL := os.Getenv("SCHEMA_REGISTRY_URL")
	if brokers == "" || registryURL == "" {
		log.Fatal("KAFKA_BROKERS and SCHEMA_REGISTRY_URL are required")
	}
	p, err := producer.New(strings.Split(brokers, ","), registryURL)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("GENERATE_EVENTS") == "true" {
		interval := 5 * time.Second
		if value := os.Getenv("GENERATOR_INTERVAL"); value != "" {
			interval, err = time.ParseDuration(value)
			if err != nil || interval <= 0 {
				log.Fatal("GENERATOR_INTERVAL must be a positive duration")
			}
		}
		go p.Generate(ctx, interval)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /events", p.ServeEvent)
	server := &http.Server{Addr: ":8080", Handler: mux, ReadTimeout: 5 * time.Second, WriteTimeout: 25 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()
	slog.Info("producer started", "addr", server.Addr)
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("cannot shut down HTTP server", "error", err)
		}
	}
}
