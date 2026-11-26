package dapr

import (
	"encoding/json"
	"testing"

	common2 "github.com/researchaccelerator-hub/telegram-scraper/common"
	"github.com/stretchr/testify/assert"
)

// TestJobHandling tests that job data is properly parsed and validated
func TestJobHandling(t *testing.T) {
	// Test proper job data
	t.Run("Valid Job Data", func(t *testing.T) {
		jobData := JobData{
			DueTime:     "1h",
			JobName:     "telegram-crawl",
			Task:        "crawl",
			URLs:        []string{"https://t.me/channel1"},
			CrawlID:     "test-crawl-id",
			MaxDepth:    2,
			Concurrency: 3,
		}

		jobDataBytes, err := json.Marshal(jobData)
		assert.NoError(t, err, "Should marshal job data without error")
		assert.NotNil(t, jobDataBytes, "Serialized job data should not be nil")

		// Unmarshal to verify roundtrip
		var parsedJobData JobData
		err = json.Unmarshal(jobDataBytes, &parsedJobData)
		assert.NoError(t, err, "Should unmarshal job data without error")
		
		// Verify all fields are preserved
		assert.Equal(t, jobData.DueTime, parsedJobData.DueTime)
		assert.Equal(t, jobData.JobName, parsedJobData.JobName)
		assert.Equal(t, jobData.Task, parsedJobData.Task)
		assert.Equal(t, jobData.URLs, parsedJobData.URLs)
		assert.Equal(t, jobData.CrawlID, parsedJobData.CrawlID)
		assert.Equal(t, jobData.MaxDepth, parsedJobData.MaxDepth)
		assert.Equal(t, jobData.Concurrency, parsedJobData.Concurrency)
	})

	// Test that a job payload carrying tdlibDatabaseUrls cannot set the TDLib
	// session-archive URLs. These URLs are downloaded and extracted by the
	// crawler and also supply the Telegram session credentials, so they must
	// come from operator configuration only (see security issue #386).
	t.Run("Job Payload Cannot Set TDLibDatabaseURLs", func(t *testing.T) {
		payload := []byte(`{"dueTime":"1h","jobName":"telegram-crawl","task":"crawl","urls":["https://t.me/channel1"],"tdlibDatabaseUrls":["https://evil.example.com/db.tgz"]}`)

		var jobData JobData
		err := json.Unmarshal(payload, &jobData)
		assert.NoError(t, err, "payload should parse; unknown keys are ignored")

		baseConfig := common2.CrawlerConfig{
			Platform:          "telegram",
			TDLibDatabaseURLs: []string{"https://archives.example.com/operator.tgz"},
		}

		merged := mergeConfigWithJobData(baseConfig, jobData)
		assert.Equal(t, []string{"https://archives.example.com/operator.tgz"}, merged.TDLibDatabaseURLs,
			"job payload must not override TDLib database URLs")
	})

	// Test with URL file instead of direct URLs
	t.Run("With URL File", func(t *testing.T) {
		jobData := JobData{
			DueTime:     "1h", 
			JobName:     "youtube-crawl",
			Task:        "crawl",
			URLFile:     "/tmp/test-urls.txt",
			CrawlID:     "test-crawl-id",
			MaxDepth:    2,
			Concurrency: 3,
		}

		jobDataBytes, err := json.Marshal(jobData)
		assert.NoError(t, err, "Should marshal job data without error")
		assert.NotNil(t, jobDataBytes, "Serialized job data should not be nil")
		
		// Unmarshal to verify roundtrip
		var parsedJobData JobData
		err = json.Unmarshal(jobDataBytes, &parsedJobData)
		assert.NoError(t, err, "Should unmarshal job data without error")
		
		// Verify URL file field is preserved
		assert.Equal(t, jobData.URLFile, parsedJobData.URLFile)
	})
}