package dapr

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	daprc "github.com/dapr/go-sdk/client"
	"github.com/dapr/go-sdk/service/common"
	common2 "github.com/researchaccelerator-hub/telegram-scraper/common"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

// TestValidateJobData covers the input validation that keeps a malformed job
// from crashing the worker.
func TestValidateJobData(t *testing.T) {
	t.Run("Short YouTube API key is rejected", func(t *testing.T) {
		jobData := JobData{
			JobName:       "youtube-crawl",
			Task:          "crawl",
			YouTubeAPIKey: "abc", // non-empty but shorter than 8 chars
			Concurrency:   2,
		}
		err := validateJobData(jobData)
		assert.Error(t, err, "a too-short API key must return an error, not a slice-bounds panic")
		assert.Contains(t, err.Error(), "too short")
	})

	t.Run("Valid YouTube API key passes", func(t *testing.T) {
		jobData := JobData{
			JobName:       "youtube-crawl",
			Task:          "crawl",
			YouTubeAPIKey: "AIzaSyExample0123456789",
			Concurrency:   2,
		}
		assert.NoError(t, validateJobData(jobData))
	})

	t.Run("Empty API key passes", func(t *testing.T) {
		assert.NoError(t, validateJobData(JobData{JobName: "telegram-crawl", Task: "crawl"}))
	})

	t.Run("Negative concurrency is rejected", func(t *testing.T) {
		err := validateJobData(JobData{JobName: "telegram-crawl", Task: "crawl", Concurrency: -1})
		assert.Error(t, err, "a negative concurrency must return an error, not panic in make(chan struct{}, -1)")
		assert.Contains(t, err.Error(), "concurrency")
	})

	t.Run("Concurrency of one or more passes", func(t *testing.T) {
		assert.NoError(t, validateJobData(JobData{JobName: "telegram-crawl", Task: "crawl", Concurrency: 1}))
		assert.NoError(t, validateJobData(JobData{JobName: "telegram-crawl", Task: "crawl", Concurrency: 8}))
	})
}

// TestHandleJob_MalformedInput verifies that handleJob returns an error
// (instead of panicking) for malformed job payloads.
func TestHandleJob_MalformedInput(t *testing.T) {
	// Keep the previous app state; tests below install their own.
	defer func(prev App) { app = prev }(app)

	t.Run("Short API key returns error without panicking", func(t *testing.T) {
		app = App{baseConfig: newTestBaseConfig()}

		jobData := JobData{
			JobName:       "youtube-crawl",
			Task:          "crawl",
			YouTubeAPIKey: "k", // 1-character key
			Concurrency:   2,
		}
		raw, err := json.Marshal(jobData)
		assert.NoError(t, err)

		job := &common.JobEvent{Data: raw, JobType: "youtube-crawl"}

		var returned error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handleJob panicked on a short API key: %v", r)
				}
			}()
			returned = handleJob(context.Background(), job)
		}()
		assert.Error(t, returned, "a 1-character API key must return an error")
		assert.Contains(t, returned.Error(), "too short")
	})

	t.Run("Negative concurrency returns error without panicking", func(t *testing.T) {
		app = App{baseConfig: newTestBaseConfig()}

		jobData := JobData{
			JobName:     "telegram-crawl",
			Task:        "crawl",
			Concurrency: -1,
		}
		raw, err := json.Marshal(jobData)
		assert.NoError(t, err)

		job := &common.JobEvent{Data: raw, JobType: "telegram-crawl"}

		var returned error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handleJob panicked on negative concurrency: %v", r)
				}
			}()
			returned = handleJob(context.Background(), job)
		}()
		assert.Error(t, returned, "concurrency of -1 must return an error")
		assert.Contains(t, returned.Error(), "concurrency")
	})

	t.Run("Zero concurrency with no usable base value returns error without panicking", func(t *testing.T) {
		// Base configuration without concurrency: the merged config must not
		// reach processLayerInParallel with 0 workers.
		app = App{baseConfig: newTestBaseConfig()}

		jobData := JobData{
			JobName: "telegram-crawl",
			Task:    "crawl",
			// Concurrency omitted: falls back to the (also empty) CLI value.
		}
		raw, err := json.Marshal(jobData)
		assert.NoError(t, err)

		job := &common.JobEvent{Data: raw, JobType: "telegram-crawl"}

		var returned error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handleJob panicked on zero concurrency: %v", r)
				}
			}()
			returned = handleJob(context.Background(), job)
		}()
		assert.Error(t, returned, "concurrency of 0 (with no CLI fallback) must return an error")
		assert.Contains(t, returned.Error(), "concurrency")
	})

	t.Run("Valid job input passes validation", func(t *testing.T) {
		app = App{baseConfig: newTestBaseConfig()}

		jobData := JobData{
			JobName:     "telegram-crawl",
			Task:        "crawl",
			Concurrency: 2,
			MaxDepth:    1,
			MaxPages:    1,
		}
		raw, err := json.Marshal(jobData)
		assert.NoError(t, err)

		job := &common.JobEvent{Data: raw, JobType: "telegram-crawl"}

		// The job proceeds past validation (it will fail later on missing
		// state/Dapr infrastructure, which is fine — what matters is that it
		// gets there without an input-validation error or a panic).
		var returned error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handleJob panicked on a valid job: %v", r)
				}
			}()
			returned = handleJob(context.Background(), job)
		}()
		if returned != nil {
			assert.NotContains(t, returned.Error(), "concurrency", "a valid concurrency value must not be rejected")
			assert.NotContains(t, returned.Error(), "too short")
		}
	})
}

// TestGetJob_UnknownJobName verifies that getJob returns an error (instead of
// dereferencing a nil job) when the Dapr client cannot find the job.
func TestGetJob_UnknownJobName(t *testing.T) {
	defer func(prev App) { app = prev }(app)

	// A real client whose calls fail without a running sidecar: exactly the
	// "unknown job / failed get" path that used to panic on a nil job.
	client, err := daprc.NewClient()
	if err != nil {
		t.Skipf("could not create a Dapr client in this environment: %v", err)
	}
	app = App{daprClient: client}

	var out *common.Content
	var returned error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("getJob panicked on an unknown job name: %v", r)
			}
		}()
		out, returned = getJob(context.Background(), &common.InvocationEvent{Data: []byte("no-such-job-name")})
	}()

	assert.Error(t, returned, "an unknown job name must return an error")
	assert.Nil(t, out, "no content must be returned on error")
}

// TestRecoveryUnaryInterceptor verifies that the gRPC recovery interceptor
// converts a handler panic into a gRPC error instead of crashing the process.
func TestRecoveryUnaryInterceptor(t *testing.T) {
	t.Run("Recovers from a panicking handler", func(t *testing.T) {
		resp, err := recoveryUnaryInterceptor(
			context.Background(),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/dapr.DaprService/Test"},
			func(ctx context.Context, req interface{}) (interface{}, error) {
				panic("boom")
			},
		)
		assert.Error(t, err, "a panicking handler must surface as a gRPC error")
		assert.Contains(t, err.Error(), "recovered from panic")
		assert.Nil(t, resp)
	})

	t.Run("Passes through normal handlers", func(t *testing.T) {
		resp, err := recoveryUnaryInterceptor(
			context.Background(),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/dapr.DaprService/Test"},
			func(ctx context.Context, req interface{}) (interface{}, error) {
				return "ok", nil
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, "ok", resp)
	})

	t.Run("Passes through handler errors unchanged", func(t *testing.T) {
		sentinel := fmt.Errorf("handler failure")
		_, err := recoveryUnaryInterceptor(
			context.Background(),
			nil,
			&grpc.UnaryServerInfo{FullMethod: "/dapr.DaprService/Test"},
			func(ctx context.Context, req interface{}) (interface{}, error) {
				return nil, sentinel
			},
		)
		assert.Equal(t, sentinel, err)
	})
}

// newTestBaseConfig returns a minimal base configuration with no concurrency
// set, mirroring a worker started without --concurrency.
func newTestBaseConfig() common2.CrawlerConfig {
	return common2.CrawlerConfig{}
}
