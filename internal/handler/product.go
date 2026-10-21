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

func (h *ProductHandler) CreateProduct(ctx context.Context, request api.CreateProductRequestObject) (api.CreateProductResponseObject, error) {
	created, err := h.repo.Create(ctx, fromRequest(*request.Body))
	if err != nil {
		return nil, err
	}
	return api.CreateProduct201JSONResponse(toResponse(created)), nil
}

func (h *ProductHandler) GetProduct(ctx context.Context, request api.GetProductRequestObject) (api.GetProductResponseObject, error) {
	p, err := h.repo.GetByID(ctx, request.Id)
	if errors.Is(err, repository.ErrNotFound) {
		return api.GetProduct404JSONResponse{NotFoundJSONResponse: productNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetProduct200JSONResponse(toResponse(p)), nil
}

func (h *ProductHandler) GetProducts(ctx context.Context, request api.GetProductsRequestObject) (api.GetProductsResponseObject, error) {
	params := request.Params
	page := valueOr(params.Page, defaultPage)
	size := valueOr(params.Size, defaultSize)
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
	items := []api.ProductResponse{}
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
	p := fromRequest(*request.Body)
	p.ID = request.Id
	updated, err := h.repo.Update(ctx, p)
	if errors.Is(err, repository.ErrNotFound) {
		return api.UpdateProduct404JSONResponse{NotFoundJSONResponse: productNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateProduct200JSONResponse(toResponse(updated)), nil
}

func (h *ProductHandler) DeleteProduct(ctx context.Context, request api.DeleteProductRequestObject) (api.DeleteProductResponseObject, error) {
	err := h.repo.Archive(ctx, request.Id)
	if errors.Is(err, repository.ErrNotFound) {
		return api.DeleteProduct404JSONResponse{NotFoundJSONResponse: productNotFound}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.DeleteProduct204Response{}, nil
}

var productNotFound = api.NotFoundJSONResponse{
	ErrorCode: api.PRODUCTNOTFOUND,
	Message:   "product not found",
}

func fromRequest(body api.ProductCreate) repository.Product {
	return repository.Product{
		Name:        body.Name,
		Description: body.Description,
		Price:       body.Price,
		Stock:       body.Stock,
		Category:    body.Category,
		Status:      string(body.Status),
	}
}

func toResponse(p repository.Product) api.ProductResponse {
	return api.ProductResponse{
		Id:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		Stock:       p.Stock,
		Category:    p.Category,
		Status:      api.ProductStatus(p.Status),
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func valueOr(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}
