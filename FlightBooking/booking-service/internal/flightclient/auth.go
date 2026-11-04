package flightclient

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const apiKeyHeader = "x-api-key"

func APIKeyInterceptor(apiKey string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, apiKeyHeader, apiKey)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
