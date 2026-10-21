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

func OpenAPIValidator(doc *openapi3.T) (func(http.Handler) http.Handler, error) {
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
			input := &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options:    &openapi3filter.Options{MultiError: true},
			}
			if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				writeValidationError(w, fieldErrors(err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func fieldErrors(err error) []FieldError {
	var result []FieldError
	for _, e := range unwrapMulti(err) {
		var reqErr *openapi3filter.RequestError
		if !errors.As(e, &reqErr) {
			result = append(result, FieldError{Message: e.Error()})
			continue
		}
		param := ""
		if reqErr.Parameter != nil {
			param = reqErr.Parameter.Name
		}
		inner := unwrapMulti(reqErr.Err)
		if len(inner) == 0 {
			result = append(result, FieldError{Field: fieldOrBody(param), Message: reqErr.Reason})
			continue
		}
		for _, ie := range inner {
			var schemaErr *openapi3.SchemaError
			if errors.As(ie, &schemaErr) {
				field := strings.Join(schemaErr.JSONPointer(), ".")
				if param != "" {
					field = param
				}
				result = append(result, FieldError{Field: fieldOrBody(field), Message: schemaErr.Reason})
				continue
			}
			result = append(result, FieldError{Field: fieldOrBody(param), Message: ie.Error()})
		}
	}
	return result
}

func unwrapMulti(err error) []error {
	if err == nil {
		return nil
	}
	if multi, ok := err.(openapi3.MultiError); ok {
		var all []error
		for _, e := range multi {
			all = append(all, unwrapMulti(e)...)
		}
		return all
	}
	return []error{err}
}

func fieldOrBody(field string) string {
	if field == "" {
		return "body"
	}
	return field
}
