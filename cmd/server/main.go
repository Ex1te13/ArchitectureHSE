package main

import (
	"context"
	"log"
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
	decimal.MarshalJSONWithoutQuotes = true
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatal("cannot create pool: ", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal("cannot connect to database: ", err)
	}
	repo := repository.NewProductRepository(pool)
	h := handler.NewProductHandler(repo)
	strict := api.NewStrictHandlerWithOptions(h, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  middleware.RequestErrorHandler,
		ResponseErrorHandlerFunc: middleware.ResponseErrorHandler,
	})
	router := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseRouter:       http.NewServeMux(),
		ErrorHandlerFunc: middleware.ParamErrorHandler,
	})
	spec, err := api.GetSwagger()
	if err != nil {
		log.Fatal("cannot load openapi spec: ", err)
	}
	validate, err := middleware.OpenAPIValidator(spec)
	if err != nil {
		log.Fatal("cannot create validator: ", err)
	}
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", validate(router)))
}
