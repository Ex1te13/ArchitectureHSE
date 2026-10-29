package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"marketplace/internal/api"
	"marketplace/internal/handler"
	"marketplace/internal/middleware"
	"marketplace/internal/repository"
)

func main() {
	slog.SetDefault(newJSONLogger())
	decimal.MarshalJSONWithoutQuotes = true

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatal("cannot create pool: ", err)
	}
	defer pool.Close()
	if err := pool.Ping(context.Background()); err != nil {
		log.Fatal("cannot connect to database: ", err)
	}

	repo := repository.NewProductRepository(pool)
	h := handler.NewProductHandler(repo)

	strict := api.NewStrictHandlerWithOptions(h, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  middleware.BadRequestHandler,
		ResponseErrorHandlerFunc: middleware.InternalErrorHandler,
	})
	router := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		ErrorHandlerFunc: middleware.BadRequestHandler,
	})

	spec, err := api.GetSwagger()
	if err != nil {
		log.Fatal("cannot load openapi spec: ", err)
	}
	validator, err := middleware.Validator(spec)
	if err != nil {
		log.Fatal("cannot create validator: ", err)
	}
	app := middleware.Logging(validator(router))

	slog.Info("server started", slog.String("addr", ":8080"))
	log.Fatal(http.ListenAndServe(":8080", app))
}

func newJSONLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Key = "timestamp"
			}
			return a
		},
	}))
}