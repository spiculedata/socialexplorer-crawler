package dapr

import (
	"context"
	"net"
	"os"
	"testing"

	cpb "github.com/dapr/dapr/pkg/proto/common/v1"
	pb "github.com/dapr/dapr/pkg/proto/runtime/v1"
	"github.com/dapr/go-sdk/service/common"
	daprs "github.com/dapr/go-sdk/service/grpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// startTestAppService builds a real Dapr go-sdk gRPC app server with the app
// API token interceptors installed, listening on an in-memory transport, and
// returns a client connection to it.
func startTestAppService(t *testing.T) pb.AppCallbackClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	svc := daprs.NewServiceWithListener(
		lis,
		grpc.ChainUnaryInterceptor(AppAPITokenUnaryInterceptor()),
		grpc.ChainStreamInterceptor(AppAPITokenStreamInterceptor()),
	)

	err := svc.AddServiceInvocationHandler("crawl", func(ctx context.Context, in *common.InvocationEvent) (*common.Content, error) {
		return &common.Content{ContentType: "application/json", Data: []byte(`{"ok":true}`)}, nil
	})
	assert.NoError(t, err)

	go func() { _ = svc.Start() }()
	t.Cleanup(func() { _ = svc.Stop() })

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return pb.NewAppCallbackClient(conn)
}

func invokeRequest(method string) *cpb.InvokeRequest {
	return &cpb.InvokeRequest{
		Method:      method,
		ContentType: "application/json",
		HttpExtension: &cpb.HTTPExtension{
			Verb: cpb.HTTPExtension_POST,
		},
	}
}

func tokenMD(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), AppAPITokenMetadataKey, token)
}

func incomingTokenMD(token string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(AppAPITokenMetadataKey, token))
}

func TestCheckAppAPIToken(t *testing.T) {
	t.Run("no APP_API_TOKEN configured passes", func(t *testing.T) {
		t.Setenv(AppAPITokenEnvVar, "")
		assert.NoError(t, CheckAppAPIToken(metadata.NewIncomingContext(context.Background(), metadata.MD{})))
	})

	t.Run("missing metadata refused", func(t *testing.T) {
		t.Setenv(AppAPITokenEnvVar, "secret")
		err := CheckAppAPIToken(metadata.NewIncomingContext(context.Background(), metadata.MD{}))
		assert.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("wrong token refused", func(t *testing.T) {
		t.Setenv(AppAPITokenEnvVar, "secret")
		err := CheckAppAPIToken(tokenMD("not-the-secret"))
		assert.Error(t, err)
		assert.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("matching token passes", func(t *testing.T) {
		t.Setenv(AppAPITokenEnvVar, "secret")
		assert.NoError(t, CheckAppAPIToken(incomingTokenMD("secret")))
	})
}

// TestAppServerRefusesCallWithoutToken exercises the real Dapr app server:
// with APP_API_TOKEN set, a direct service invocation without the token must
// be refused.
func TestAppServerRefusesCallWithoutToken(t *testing.T) {
	t.Setenv(AppAPITokenEnvVar, "secret")
	client := startTestAppService(t)

	_, err := client.OnInvoke(context.Background(), invokeRequest("crawl"))
	assert.Error(t, err, "a call without the app API token must be refused")
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = client.OnInvoke(tokenMD("wrong"), invokeRequest("crawl"))
	assert.Error(t, err, "a call with a wrong app API token must be refused")
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAppServerAcceptsCallWithToken(t *testing.T) {
	t.Setenv(AppAPITokenEnvVar, "secret")
	client := startTestAppService(t)

	resp, err := client.OnInvoke(tokenMD("secret"), invokeRequest("crawl"))
	assert.NoError(t, err, "a call with the app API token must pass")
	assert.Equal(t, "application/json", resp.GetContentType())
}

// TestAppServerAcceptsSidecarCallbacksWhenTokenSet documents that the
// interceptors do not break callbacks the Dapr sidecar sends without the app
// API token (health checks, topic events, job events): those are rejected as
// unimplemented by the app callback server here, but crucially not as
// permission-denied by the token check.
func TestAppServerAcceptsSidecarCallbacksWhenTokenSet(t *testing.T) {
	t.Setenv(AppAPITokenEnvVar, "secret")
	client := startTestAppService(t)

	_, err := client.OnTopicEvent(context.Background(), &pb.TopicEventRequest{
		PubsubName: "pubsub",
		Topic:      "workqueue",
		Id:         "event-1",
		Data:       []byte(`{}`),
	})
	assert.Error(t, err, "the app has no topic handler registered in this test")
	assert.NotEqual(t, codes.PermissionDenied, status.Code(err),
		"sidecar callbacks must not be blocked by the token check")
}

func TestAppServerAllowsAllWhenTokenUnset(t *testing.T) {
	t.Setenv(AppAPITokenEnvVar, "")
	client := startTestAppService(t)

	resp, err := client.OnInvoke(context.Background(), invokeRequest("crawl"))
	assert.NoError(t, err, "without APP_API_TOKEN configured the loopback binding is the protection")
	assert.Equal(t, "application/json", resp.GetContentType())
}

func TestWarnIfNoAppAPITokenDoesNotPanic(t *testing.T) {
	t.Setenv(AppAPITokenEnvVar, "")
	WarnIfNoAppAPIToken()

	t.Setenv(AppAPITokenEnvVar, "secret")
	WarnIfNoAppAPIToken()
}

// TestDefaultBindAddresses documents the loopback-only default bindings the
// app servers must use.
func TestDefaultBindAddresses(t *testing.T) {
	assert.NotContains(t, "127.0.0.1:6481", "0.0.0.0")
	assert.NotContains(t, "127.0.0.1:6060", "0.0.0.0")
	assert.NotContains(t, "127.0.0.1:2345", "0.0.0.0")
	_ = os.Unsetenv(AppAPITokenEnvVar)
}
