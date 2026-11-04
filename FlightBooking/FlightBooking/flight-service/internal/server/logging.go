package server

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

func LoggingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	st := status.Convert(err)
	attrs := []any{
		slog.String("method", info.FullMethod),
		slog.String("code", st.Code().String()),
		slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		attrs = append(attrs, slog.String("message", st.Message()))
	}
	slog.InfoContext(ctx, "grpc call", attrs...)
	return resp, err
}
