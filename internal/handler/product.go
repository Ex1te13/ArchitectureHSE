package handler

import (
	"context"
	"errors"

	"marketplace/internal/api"
	"marketplace/internal/repository"
)

const (
	defaultPage = 0
	defaultSize = 20
)

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

func productNotFound() api.NotFoundJSONResponse {
	return api.NotFoundJSONResponse{
		ErrorCode: api.PRODUCTNOTFOUND,
		Message:   "product not found",
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
	err := h.repo.Archive(ctx, request.Id)
	if errors.Is(err, repository.ErrNotFound) {
		return api.DeleteProduct404JSONResponse{NotFoundJSONResponse: productNotFound()}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.DeleteProduct204Response{}, nil
}

func (h *ProductHandler) GetProducts(ctx context.Context, request api.GetProductsRequestObject) (api.GetProductsResponseObject, error) {
	params := request.Params
	page := defaultPage
	if params.Page != nil {
		page = *params.Page
	}
	size := defaultSize
	if params.Size != nil {
		size = *params.Size
	}
	filter := repository.ProductFilter{
		Category: params.Category,
		Limit:    size,
		Offset:   page * size,
	}
	if params.Status != nil {
		status := string(*params.Status)
		filter.Status = &status
	}
	products, total, err := h.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]api.ProductResponse, 0, len(products))
	for _, p := range products {
		items = append(items, toResponse(p))
	}
	return api.GetProducts200JSONResponse{
		Products:      items,
		TotalElements: total,
		Page:          page,
		Size:          size,
	}, nil
}

func (h *ProductHandler) UpdateProduct(ctx context.Context, request api.UpdateProductRequestObject) (api.UpdateProductResponseObject, error) {
	body := request.Body
	updated, err := h.repo.Update(ctx, repository.Product{
		ID:          request.Id,
		Name:        body.Name,
		Category:    body.Category,
		Description: body.Description,
		Price:       body.Price,
		Stock:       body.Stock,
		Status:      string(body.Status),
	})
	if errors.Is(err, repository.ErrNotFound) {
		return api.UpdateProduct404JSONResponse{NotFoundJSONResponse: productNotFound()}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateProduct200JSONResponse(toResponse(updated)), nil
}

func (h *ProductHandler) GetProduct(ctx context.Context, request api.GetProductRequestObject) (api.GetProductResponseObject, error) {
	p, err := h.repo.GetByID(ctx, request.Id)
	if errors.Is(err, repository.ErrNotFound) {
		return api.GetProduct404JSONResponse{NotFoundJSONResponse: productNotFound()}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetProduct200JSONResponse(toResponse(p)), nil
}
