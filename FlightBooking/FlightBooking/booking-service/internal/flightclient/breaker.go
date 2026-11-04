package flightclient

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type BreakerConfig struct {
	FailureThreshold int
	Window           time.Duration
	OpenTimeout      time.Duration
}

type state string

const (
	closed   state = "CLOSED"
	open     state = "OPEN"
	halfOpen state = "HALF_OPEN"
)

var ErrCircuitOpen = status.Error(codes.Unavailable, "circuit breaker is open: flight service is unavailable, try again later")

type Breaker struct {
	cfg BreakerConfig

	mu          sync.Mutex
	state       state
	failures    int
	windowStart time.Time
	openedAt    time.Time
	probing     bool
}

func NewBreaker(cfg BreakerConfig) *Breaker {
	return &Breaker{cfg: cfg, state: closed}
}

func (b *Breaker) Interceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !b.allow() {
			return ErrCircuitOpen
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		b.record(isFailure(err))
		return err
	}
}

func isFailure(err error) bool {
	code := status.Code(err)
	return code == codes.Unavailable || code == codes.DeadlineExceeded
}

func (b *Breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case open:
		if time.Since(b.openedAt) < b.cfg.OpenTimeout {
			return false
		}
		b.setState(halfOpen)
		b.probing = true
		return true
	case halfOpen:
		if b.probing {
			return false
		}
		b.probing = true
		return true
	default:
		return true
	}
}

func (b *Breaker) record(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case halfOpen:
		b.probing = false
		if failed {
			b.trip()
		} else {
			b.failures = 0
			b.setState(closed)
		}
	case closed:
		if !failed {
			b.failures = 0
			return
		}
		now := time.Now()
		if b.failures == 0 || now.Sub(b.windowStart) > b.cfg.Window {
			b.failures = 0
			b.windowStart = now
		}
		b.failures++
		if b.failures >= b.cfg.FailureThreshold {
			b.trip()
		}
	}
}

func (b *Breaker) trip() {
	b.failures = 0
	b.openedAt = time.Now()
	b.setState(open)
}

func (b *Breaker) setState(s state) {
	slog.Warn("circuit breaker state changed", slog.String("from", string(b.state)), slog.String("to", string(s)))
	b.state = s
}
