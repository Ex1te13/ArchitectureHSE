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

type ProductFilter struct {
	Status   *string
	Category *string
	Limit    int
	Offset   int
}

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

const productColumns = `id, name, description, price, stock, category, status, created_at, updated_at`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock,
		&p.Category, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (r *ProductRepository) Create(ctx context.Context, p Product) (Product, error) {
	q := `
		INSERT INTO products (name, description, price, stock, category, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + productColumns
	created, err := scanProduct(r.pool.QueryRow(ctx, q,
		p.Name, p.Description, p.Price, p.Stock, p.Category, p.Status))
	if err != nil {
		return Product{}, fmt.Errorf("insert product: %w", err)
	}
	return created, nil
}

func (r *ProductRepository) GetByID(ctx context.Context, id uuid.UUID) (Product, error) {
	q := `SELECT ` + productColumns + ` FROM products WHERE id = $1`
	p, err := scanProduct(r.pool.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("select product: %w", err)
	}
	return p, nil
}

func (r *ProductRepository) Update(ctx context.Context, p Product) (Product, error) {
	q := `
		UPDATE products
		SET name = $2, description = $3, price = $4, stock = $5, category = $6, status = $7
		WHERE id = $1
		RETURNING ` + productColumns
	updated, err := scanProduct(r.pool.QueryRow(ctx, q,
		p.ID, p.Name, p.Description, p.Price, p.Stock, p.Category, p.Status))
	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("update product: %w", err)
	}
	return updated, nil
}

func (r *ProductRepository) Archive(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE products SET status = 'ARCHIVED' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("archive product: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

const listFilter = `
	WHERE ($1::product_status IS NULL OR status = $1)
	  AND ($1::product_status IS NOT NULL OR status <> 'ARCHIVED')
	  AND ($2::text IS NULL OR category = $2)`

func (r *ProductRepository) List(ctx context.Context, f ProductFilter) ([]Product, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM products`+listFilter,
		f.Status, f.Category).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}
	q := `SELECT ` + productColumns + ` FROM products` + listFilter + `
		ORDER BY created_at DESC, id
		LIMIT $3 OFFSET $4`
	rows, err := r.pool.Query(ctx, q, f.Status, f.Category, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	products := []Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, p)
	}
	return products, total, rows.Err()
}
