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
		body := readBody(r)
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

func readBody(r *http.Request) any {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil
	}
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 {
		return nil
	}
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		return "<invalid json>"
	}
	mask(body)
	return body
}

func mask(v any) {
	switch val := v.(type) {
	case map[string]any:
		for key, inner := range val {
			if sensitiveKeys[strings.ToLower(key)] {
				val[key] = "***"
			} else {
				mask(inner)
			}
		}
	case []any:
		for _, item := range val {
			mask(item)
		}
	}
}
