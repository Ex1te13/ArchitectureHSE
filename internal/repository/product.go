package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var ErrNotFound = errors.New("not found")

type Product struct {
	ID          uuid.UUID
	Name        string
	Description *string
	Price       decimal.Decimal
	Stock       int
	Category    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

func (r *ProductRepository) Create(ctx context.Context, p Product) (Product, error) {
	const q = `
		INSERT INTO products (name, description, price, stock, category, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`
	err := r.pool.QueryRow(ctx, q,
		p.Name, p.Description, p.Price, p.Stock, p.Category, p.Status,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Product{}, fmt.Errorf("insert product: %w", err)
	}
	return p, nil
}

func (r *ProductRepository) GetByID(ctx context.Context, id uuid.UUID) (Product, error) {
	const q = `
		SELECT id, name, description, price, stock, category, status, created_at, updated_at FROM products WHERE id = $1;`
	p := Product{}
	err := r.pool.QueryRow(ctx, q, id).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock,
		&p.Category, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	} else if err != nil {
		return Product{}, fmt.Errorf("select product: %w", err)
	}
	return p, nil
}
