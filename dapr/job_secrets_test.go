package dapr

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dapr/go-sdk/service/common"
	"github.com/rs/zerolog"
	zerologlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

// TestHandleJobDoesNotLogSecrets verifies that processing a job whose payload
// carries a marker API key and a marker TDLib session-archive URL never writes
// either marker to the captured log output.
func TestHandleJobDoesNotLogSecrets(t *testing.T) {
	markerKey := "MARKER-SECRET-API-KEY-0123456789"
	// Fragments: the old code logged key[:8]...key[len-4:]; neither may appear.
	markerKeyPrefix := markerKey[:8]
	markerKeySuffix := markerKey[len(markerKey)-4:]
	markerURL := "http://127.0.0.1:9/MARKER-SESSION-ARCHIVE.tgz"
	markerURLPrefix := markerURL[:20]

	tests := []struct {
		name    string
		jobType string
		data    map[string]interface{}
	}{
		{
			name:    "youtube crawl job with marker key and marker archive URL",
			jobType: "youtube-crawl",
			data: map[string]interface{}{
				"jobName":           "youtube-crawl",
				"task":              "crawl",
				"platform":          "youtube",
				"urls":              []string{"https://www.youtube.com/channel/UC-example"},
				"youtubeApiKey":     markerKey,
				"tdlibDatabaseUrls": []string{markerURL},
				"crawlId":           "secrets-test-crawl",
				"concurrency":       1,
			},
		},
		{
			name:    "generic job with marker key and marker archive URL",
			jobType: "unknown-job-type",
			data: map[string]interface{}{
				"jobName":           "unknown-job-type",
				"task":              "maintain",
				"platform":          "youtube",
				"youtubeApiKey":     markerKey,
				"tdlibDatabaseUrls": []string{markerURL},
				"crawlId":           "secrets-test-crawl",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.data)
			assert.NoError(t, err)

			var buf strings.Builder

			// Capture everything the global zerolog logger emits, including
			// debug-level output.
			origLogger := zerologlog.Logger
			zerologlog.Logger = zerolog.New(&buf).With().Timestamp().Logger()
			defer func() { zerologlog.Logger = origLogger }()

			// The job is expected to fail (there is no DAPR sidecar in unit
			// tests); what matters is that nothing along the way leaks the
			// marker key or the marker archive URL.
			_ = handleJob(context.Background(), &common.JobEvent{
				JobType: tt.jobType,
				Data:    data,
			})

			out := buf.String()

			// Positive control: the job event was actually handled and logged.
			assert.Contains(t, out, "JOB EVENT RECEIVED")

			assert.NotContains(t, out, markerKey,
				"the API key must never appear in log output")
			assert.NotContains(t, out, markerKeyPrefix,
				"even a fragment of the API key must never appear in log output")
			assert.NotContains(t, out, markerKeySuffix,
				"even a fragment of the API key must never appear in log output")
			assert.NotContains(t, out, markerURL,
				"the session archive URL must never appear in log output")
			assert.NotContains(t, out, markerURLPrefix,
				"even a fragment of the session archive URL must never appear in log output")
		})
	}
}

// TestHandleJobDoesNotLogRawPayloadOnParseError verifies that the parse-error
// path does not echo the raw job payload, which may carry credentials.
func TestHandleJobDoesNotLogRawPayloadOnParseError(t *testing.T) {
	const markerKey = "MARKER-SECRET-API-KEY-0123456789"

	var buf strings.Builder
	origLogger := zerologlog.Logger
	zerologlog.Logger = zerolog.New(&buf).With().Timestamp().Logger()
	defer func() { zerologlog.Logger = origLogger }()

	raw := []byte(`{"youtubeApiKey": "` + markerKey + `", "broken":`)
	err := handleJob(context.Background(), &common.JobEvent{
		JobType: "youtube-crawl",
		Data:    raw,
	})
	assert.Error(t, err)

	out := buf.String()
	assert.NotContains(t, out, markerKey,
		"the raw payload must not be echoed on parse errors")
	assert.NotContains(t, out, `"broken":`,
		"the raw payload must not be echoed on parse errors")
}
