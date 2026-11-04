package server

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const APIKeyHeader = "x-api-key"

func AuthInterceptor(apiKey string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		keys := md.Get(APIKeyHeader)
		if len(keys) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing api key")
		}
		if subtle.ConstantTimeCompare([]byte(keys[0]), []byte(apiKey)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid api key")
		}
		return handler(ctx, req)
	}
}
