package handler

import (
	"context"
	"errors"

	"marketplace/internal/api"
	"marketplace/internal/repository"
)

// ProductHandler отвечает на запросы про товары
type ProductHandler struct {
	repo *repository.ProductRepository
}

var _ api.StrictServerInterface = (*ProductHandler)(nil)

func NewProductHandler(repo *repository.ProductRepository) *ProductHandler {
	return &ProductHandler{repo: repo}
}

func toResponse(p repository.Product) api.ProductResponse {
	return api.ProductResponse{
		Id:          p.ID,
		Name:        p.Name,
		Category:    p.Category,
		CreatedAt:   p.CreatedAt,
		Description: p.Description,
		Price:       p.Price,
		Stock:       p.Stock,
		UpdatedAt:   p.UpdatedAt,
		Status:      api.ProductStatus(p.Status),
	}
}

func (h *ProductHandler) CreateProduct(ctx context.Context, request api.CreateProductRequestObject) (api.CreateProductResponseObject, error) {
	body := request.Body
	created, err := h.repo.Create(ctx, repository.Product{
		Name:        body.Name,
		Category:    body.Category,
		Description: body.Description,
		Price:       body.Price,
		Stock:       body.Stock,
		Status:      string(body.Status),
	})
	if err != nil {
		return nil, err
	}
	return api.CreateProduct201JSONResponse(toResponse(created)), nil
}

func (h *ProductHandler) DeleteProduct(ctx context.Context, request api.DeleteProductRequestObject) (api.DeleteProductResponseObject, error) {
	return nil, errors.New("not implemented")
}

func (h *ProductHandler) GetProducts(ctx context.Context, request api.GetProductsRequestObject) (api.GetProductsResponseObject, error) {
	return nil, errors.New("not implemented")
}

func (h *ProductHandler) UpdateProduct(ctx context.Context, request api.UpdateProductRequestObject) (api.UpdateProductResponseObject, error) {
	return nil, errors.New("not implemented")
}

func (h *ProductHandler) GetProduct(ctx context.Context, request api.GetProductRequestObject) (api.GetProductResponseObject, error) {
	p, err := h.repo.GetByID(ctx, request.Id)
	if errors.Is(err, repository.ErrNotFound) {
		return api.GetProduct404JSONResponse{
			NotFoundJSONResponse: api.NotFoundJSONResponse{
				ErrorCode: api.PRODUCTNOTFOUND,
				Message:   "product not found",
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetProduct200JSONResponse(toResponse(p)), nil
}
