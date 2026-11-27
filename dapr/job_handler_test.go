package dapr

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"

	common2 "github.com/researchaccelerator-hub/telegram-scraper/common"
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
// TestMergeConfigWithJobDataIgnoresTDLibDatabaseURLs verifies that a job
// payload cannot set the TDLib session archive URLs: the archive is downloaded
// and extracted with the crawler's own privileges and also supplies the
// Telegram credentials the crawl runs under, so only operator configuration
// (CLI flags / config file) may provide it.
func TestMergeConfigWithJobDataIgnoresTDLibDatabaseURLs(t *testing.T) {
	operatorURLs := []string{"https://operator.example.com/seed.tar.gz"}
	baseConfig := common2.CrawlerConfig{
		Platform:          "telegram",
		TDLibDatabaseURLs: operatorURLs,
	}

	jobData := JobData{
		DueTime:           "1h",
		JobName:           "telegram-crawl",
		Task:              "crawl",
		URLs:              []string{"https://t.me/channel1"},
		TDLibDatabaseURLs: []string{"https://attacker.example.com/seed.tar.gz"},
	}

	merged := mergeConfigWithJobData(baseConfig, jobData)

	assert.Equal(t, operatorURLs, merged.TDLibDatabaseURLs,
		"job payload must not override TDLibDatabaseURLs")
}
