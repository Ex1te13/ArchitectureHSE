package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maskedValue = "***"

var sensitiveKeys = map[string]bool{
	"password":      true,
	"access_token":  true,
	"refresh_token": true,
	"token":         true,
}

type requestInfoKey struct{}

type requestInfo struct {
	id     string
	userID *string
}

func RequestID(ctx context.Context) string {
	if info, ok := ctx.Value(requestInfoKey{}).(*requestInfo); ok {
		return info.id
	}
	return ""
}

func SetUserID(ctx context.Context, userID string) {
	if info, ok := ctx.Value(requestInfoKey{}).(*requestInfo); ok {
		info.userID = &userID
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &requestInfo{id: uuid.NewString()}
		r = r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info))
		w.Header().Set("X-Request-Id", info.id)

		var body any
		if isMutating(r.Method) && r.Body != nil {
			raw, err := io.ReadAll(r.Body)
			if err == nil {
				r.Body = io.NopCloser(bytes.NewReader(raw))
				body = maskBody(raw)
			}
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			slog.String("request_id", info.id),
			slog.String("method", r.Method),
			slog.String("endpoint", r.URL.Path),
			slog.Int("status_code", rec.status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.Any("user_id", info.userID),
		}
		if body != nil {
			attrs = append(attrs, slog.Any("request_body", body))
		}
		slog.Info("request", attrs...)
	})
}

func isMutating(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodPatch || method == http.MethodDelete
}

func maskBody(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "<invalid json>"
	}
	return mask(parsed)
}

func mask(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, inner := range val {
			if sensitiveKeys[strings.ToLower(k)] {
				val[k] = maskedValue
			} else {
				val[k] = mask(inner)
			}
		}
		return val
	case []any:
		for i, inner := range val {
			val[i] = mask(inner)
		}
		return val
	default:
		return v
	}
}
