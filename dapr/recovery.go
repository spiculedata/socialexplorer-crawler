// Package dapr provides Dapr-related functionality
package dapr

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// recoveryUnaryInterceptor is a gRPC unary server interceptor that recovers
// from panics raised inside Dapr service handlers (job events, service
// invocations). Without it a single malformed job payload can panic a handler
// and take down the whole crawler process, killing any crawl in progress.
// The panic is logged and converted into a gRPC error so the Dapr runtime can
// surface it instead of losing the worker.
func recoveryUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Str("method", info.FullMethod).
				Msg("🛑 Recovered from panic in gRPC handler; returning error to caller instead of crashing")
			err = fmt.Errorf("internal error: recovered from panic in %s", info.FullMethod)
		}
	}()
	return handler(ctx, req)
}
