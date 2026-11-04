package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"flightbooking/flight-service/internal/cache"
	"flightbooking/flight-service/internal/repository"
	"flightbooking/flight-service/internal/server"
	flightv1 "flightbooking/gen/flight/v1"
	"flightbooking/internal/logging"
)

func main() {
	slog.SetDefault(logging.New("flight-service"))

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	apiKey := os.Getenv("FLIGHT_API_KEY")
	if apiKey == "" {
		log.Fatal("FLIGHT_API_KEY is not set")
	}
	redisAddrs := os.Getenv("REDIS_ADDRS")
	if redisAddrs == "" {
		log.Fatal("REDIS_ADDRS is not set")
	}
	redisMaster := os.Getenv("REDIS_MASTER_NAME")
	cacheTTL, err := time.ParseDuration(getenv("CACHE_TTL", "5m"))
	if err != nil || cacheTTL <= 0 {
		log.Fatal("invalid CACHE_TTL: ", getenv("CACHE_TTL", "5m"))
	}
	port := getenv("GRPC_PORT", "50051")

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
	rdb := redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs:        strings.Split(redisAddrs, ","),
		MasterName:   redisMaster,
		DialTimeout:  200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
		MaxRetries:   -1,
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis is unavailable, working without cache until it is up", slog.String("error", err.Error()))
	} else {
		slog.Info("redis connected", slog.String("addrs", redisAddrs), slog.String("master_name", redisMaster))
	}
	store := cache.NewFlightRepository(repository.NewFlightRepository(pool), rdb, cacheTTL)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal("cannot listen: ", err)
	}

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		server.LoggingInterceptor,
		server.AuthInterceptor(apiKey),
	))
	flightv1.RegisterFlightServiceServer(srv, server.NewFlightServer(store))
	reflection.Register(srv)

	go func() {
		<-ctx.Done()
		slog.Info("shutting down")
		srv.GracefulStop()
	}()

	slog.Info("grpc server started", slog.String("addr", lis.Addr().String()))
	if err := srv.Serve(lis); err != nil {
		log.Fatal("grpc server failed: ", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
