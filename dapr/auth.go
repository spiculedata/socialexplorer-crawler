// Package dapr provides Dapr-related functionality
package dapr

import (
	"context"
	"os"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// AppAPITokenEnvVar is the environment variable holding the shared secret
	// between the Dapr sidecar and this application. When set, Dapr attaches it
	// to every service invocation it forwards to the app (as the
	// "dapr-api-token" gRPC metadata key), so the app can verify that a call
	// really came from the sidecar rather than from an arbitrary network peer.
	AppAPITokenEnvVar = "APP_API_TOKEN"

	// AppAPITokenMetadataKey is the gRPC metadata key Dapr uses to carry the
	// app API token on service invocations.
	AppAPITokenMetadataKey = "dapr-api-token"

	// onInvokeMethod is the full method name of the Dapr AppCallback service
	// invocation entry point, the only callback Dapr always authenticates
	// with the app API token.
	onInvokeMethod = "/dapr.proto.runtime.v1.AppCallback/OnInvoke"
)

// CheckAppAPIToken verifies that the incoming gRPC context carries the app API
// token configured via the APP_API_TOKEN environment variable.
//
// Behaviour:
//   - If APP_API_TOKEN is not set, the check passes (loopback binding of the
//     app server is the primary protection in that case; a warning is logged
//     at startup by WarnIfNoAppAPIToken).
//   - If APP_API_TOKEN is set, the request must carry matching
//     "dapr-api-token" metadata, otherwise PermissionDenied is returned.
func CheckAppAPIToken(ctx context.Context) error {
	token := os.Getenv(AppAPITokenEnvVar)
	if token == "" {
		return nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.PermissionDenied, "missing app API token")
	}

	values := md.Get(AppAPITokenMetadataKey)
	if len(values) == 0 {
		return status.Error(codes.PermissionDenied, "missing app API token")
	}
	if values[0] != token {
		return status.Error(codes.PermissionDenied, "invalid app API token")
	}
	return nil
}

// AppAPITokenUnaryInterceptor is a gRPC unary server interceptor that enforces
// the app API token check on the Dapr service invocation entry point (OnInvoke).
//
// Other AppCallback methods (health check, topic events, job events) are
// delivered by the Dapr sidecar over channels that do not carry the app API
// token (verified against Dapr v1.14), so enforcing the check there would
// break legitimate sidecar callbacks. Those methods are protected by binding
// the app server to the loopback interface, which the sidecar shares via the
// pod's network namespace.
func AppAPITokenUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info.FullMethod == onInvokeMethod {
			if err := CheckAppAPIToken(ctx); err != nil {
				return nil, err
			}
		}
		return handler(ctx, req)
	}
}

// AppAPITokenStreamInterceptor is a gRPC stream server interceptor that
// enforces the app API token check on streaming calls.
func AppAPITokenStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := CheckAppAPIToken(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}

// WarnIfNoAppAPIToken logs a warning when APP_API_TOKEN is unset so operators
// know the app server is only protected by its loopback binding.
func WarnIfNoAppAPIToken() {
	if os.Getenv(AppAPITokenEnvVar) == "" {
		log.Warn().
			Str("env", AppAPITokenEnvVar).
			Msgf("%s is not set; the app server relies on its loopback binding for protection. Set %s (and the matching sidecar configuration) so the app can reject direct service invocations", AppAPITokenEnvVar, AppAPITokenEnvVar)
	}
}
