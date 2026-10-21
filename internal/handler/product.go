package handler

import (
	"context"
	"errors"

	"marketplace/internal/api"
)

// ProductHandler отвечает на запросы про товары
type ProductHandler struct{}

var _ api.StrictServerInterface = (*ProductHandler)(nil)

func (h *ProductHandler) CreateProduct(ctx context.Context, request api.CreateProductRequestObject) (api.CreateProductResponseObject, error) {
	return nil, errors.New("not implemented")
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
	return nil, errors.New("not implemented")
}
