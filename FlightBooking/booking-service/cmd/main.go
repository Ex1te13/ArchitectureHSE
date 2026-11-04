package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"flightbooking/booking-service/internal/flightclient"
	"flightbooking/booking-service/internal/handler"
	"flightbooking/booking-service/internal/repository"
	flightv1 "flightbooking/gen/flight/v1"
	"flightbooking/internal/logging"
)

func main() {
	slog.SetDefault(logging.New("booking-service"))

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	flightAddr := os.Getenv("FLIGHT_SERVICE_ADDR")
	if flightAddr == "" {
		log.Fatal("FLIGHT_SERVICE_ADDR is not set")
	}
	apiKey := os.Getenv("FLIGHT_API_KEY")
	if apiKey == "" {
		log.Fatal("FLIGHT_API_KEY is not set")
	}
	port := getenv("HTTP_PORT", "8080")
	timeout := getDuration("FLIGHT_SERVICE_TIMEOUT", "5s")
	retry := flightclient.RetryConfig{
		MaxAttempts:    getInt("RETRY_MAX_ATTEMPTS", "3"),
		InitialBackoff: getDuration("RETRY_INITIAL_BACKOFF", "100ms"),
		AttemptTimeout: getDuration("RETRY_ATTEMPT_TIMEOUT", "1s"),
	}
	breaker := flightclient.NewBreaker(flightclient.BreakerConfig{
		FailureThreshold: getInt("CB_FAILURE_THRESHOLD", "5"),
		Window:           getDuration("CB_WINDOW", "30s"),
		OpenTimeout:      getDuration("CB_OPEN_TIMEOUT", "15s"),
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal("cannot create pool: ", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal("cannot connect to database: ", err)
	}

	conn, err := grpc.NewClient(flightAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithChainUnaryInterceptor(
			breaker.Interceptor(),
			flightclient.RetryInterceptor(retry),
			flightclient.APIKeyInterceptor(apiKey),
		),
	)
	if err != nil {
		log.Fatal("cannot create flight service client: ", err)
	}
	defer conn.Close()

	h := handler.New(repository.NewBookingRepository(pool), flightv1.NewFlightServiceClient(conn), timeout)
	srv := &http.Server{Addr: ":" + port, Handler: h.Routes()}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("http server started", slog.String("addr", srv.Addr), slog.String("flight_service", flightAddr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("http server failed: ", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key, fallback string) time.Duration {
	d, err := time.ParseDuration(getenv(key, fallback))
	if err != nil || d <= 0 {
		log.Fatalf("invalid %s: %q", key, getenv(key, fallback))
	}
	return d
}

func getInt(key, fallback string) int {
	n, err := strconv.Atoi(getenv(key, fallback))
	if err != nil || n < 1 {
		log.Fatalf("invalid %s: %q", key, getenv(key, fallback))
	}
	return n
}
