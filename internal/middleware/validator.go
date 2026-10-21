package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
)

func init() {
	openapi3.DefineStringFormat("uuid",
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
}

func Validator(doc *openapi3.T) (func(http.Handler) http.Handler, error) {
	doc.Servers = nil
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("build openapi router: %w", err)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options:    &openapi3filter.Options{MultiError: true},
			})
			if err != nil {
				writeValidationError(w, toFieldErrors(err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func toFieldErrors(err error) []FieldError {
	var fields []FieldError
	for _, e := range splitErrors(err) {
		fields = append(fields, toFieldError(e))
	}
	return fields
}

func splitErrors(err error) []error {
	if multi, ok := err.(openapi3.MultiError); ok {
		var all []error
		for _, e := range multi {
			all = append(all, splitErrors(e)...)
		}
		return all
	}
	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) {
		if multi, ok := reqErr.Err.(openapi3.MultiError); ok {
			var all []error
			for _, e := range multi {
				all = append(all, splitErrors(&openapi3filter.RequestError{Parameter: reqErr.Parameter, Err: e})...)
			}
			return all
		}
	}
	return []error{err}
}

func toFieldError(err error) FieldError {
	field, message := "body", err.Error()
	var reqErr *openapi3filter.RequestError
	if errors.As(err, &reqErr) && reqErr.Parameter != nil {
		field = reqErr.Parameter.Name
	}
	var schemaErr *openapi3.SchemaError
	if errors.As(err, &schemaErr) {
		message = schemaErr.Reason
		if path := schemaErr.JSONPointer(); field == "body" && len(path) > 0 {
			field = strings.Join(path, ".")
		}
	}
	return FieldError{Field: field, Message: message}
}
