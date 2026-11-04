package flightclient

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	AttemptTimeout time.Duration
}

func retryable(code codes.Code) bool {
	return code == codes.Unavailable || code == codes.DeadlineExceeded
}

func RetryInterceptor(cfg RetryConfig) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		backoff := cfg.InitialBackoff
		for attempt := 1; ; attempt++ {
			attemptCtx, cancel := context.WithTimeout(ctx, cfg.AttemptTimeout)
			err := invoker(attemptCtx, method, req, reply, cc, opts...)
			cancel()
			if err == nil {
				if attempt > 1 {
					slog.InfoContext(ctx, "flight service call succeeded after retry",
						slog.String("method", method), slog.Int("attempt", attempt))
				}
				return nil
			}

			code := status.Code(err)
			if !retryable(code) || ctx.Err() != nil {
				return err
			}
			if attempt >= cfg.MaxAttempts {
				slog.WarnContext(ctx, "flight service call failed, no attempts left",
					slog.String("method", method), slog.Int("attempts", attempt), slog.String("code", code.String()))
				return err
			}

			slog.WarnContext(ctx, "retrying flight service call",
				slog.String("method", method),
				slog.Int("attempt", attempt),
				slog.String("code", code.String()),
				slog.String("backoff", backoff.String()),
			)
			if !sleep(ctx, backoff) {
				return err
			}
			backoff *= 2
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
