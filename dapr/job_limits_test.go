package dapr

import (
	"context"
	"testing"
	"time"

	"github.com/researchaccelerator-hub/telegram-scraper/common"
	"github.com/stretchr/testify/assert"
)

// TestMergeConfigWithJobDataClampsJobLimits verifies that every numeric field a
// job payload controls is clamped to operator-set bounds: an out-of-range
// value must never reach the crawler unbounded (issue 390).
func TestMergeConfigWithJobDataClampsJobLimits(t *testing.T) {
	base := common.CrawlerConfig{
		Platform:    "telegram",
		Concurrency: 4,
		MaxDepth:    2,
		MaxPosts:    500,
		MaxPages:    1000,
		SampleSize:  10,
	}

	tests := []struct {
		name string
		job  JobData
		want common.CrawlerConfig
	}{
		{
			name: "huge maxDepth is clamped",
			job:  JobData{MaxDepth: 1000000},
			want: func() common.CrawlerConfig { c := base; c.MaxDepth = maxJobMaxDepth; return c }(),
		},
		{
			name: "negative maxDepth cannot request unlimited depth",
			job:  JobData{MaxDepth: -5},
			want: func() common.CrawlerConfig { c := base; c.MaxDepth = 1; return c }(),
		},
		{
			name: "huge concurrency is clamped",
			job:  JobData{Concurrency: 1000000},
			want: func() common.CrawlerConfig { c := base; c.Concurrency = maxJobConcurrency; return c }(),
		},
		{
			name: "negative concurrency cannot break the pool",
			job:  JobData{Concurrency: -3},
			want: func() common.CrawlerConfig { c := base; c.Concurrency = 1; return c }(),
		},
		{
			name: "negative maxPosts no longer means unlimited history",
			job:  JobData{MaxPosts: -1},
			want: func() common.CrawlerConfig { c := base; c.MaxPosts = 1; return c }(),
		},
		{
			name: "huge maxPosts is clamped",
			job:  JobData{MaxPosts: 500000000},
			want: func() common.CrawlerConfig { c := base; c.MaxPosts = maxJobMaxPosts; return c }(),
		},
		{
			name: "huge maxPages is clamped",
			job:  JobData{MaxPages: 2000000},
			want: func() common.CrawlerConfig { c := base; c.MaxPages = maxJobMaxPages; return c }(),
		},
		{
			name: "negative maxPages cannot disable the page cap",
			job:  JobData{MaxPages: -2},
			want: func() common.CrawlerConfig { c := base; c.MaxPages = 1; return c }(),
		},
		{
			name: "huge sampleSize is clamped",
			job:  JobData{SampleSize: 2000000000},
			want: func() common.CrawlerConfig { c := base; c.SampleSize = maxJobSampleSize; return c }(),
		},
		{
			name: "negative sampleSize disables sampling instead of misbehaving",
			job:  JobData{SampleSize: -4},
			want: func() common.CrawlerConfig { c := base; c.SampleSize = 0; return c }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeConfigWithJobData(base, tt.job)
			assert.Equal(t, tt.want.MaxDepth, got.MaxDepth, "MaxDepth")
			assert.Equal(t, tt.want.Concurrency, got.Concurrency, "Concurrency")
			assert.Equal(t, tt.want.MaxPosts, got.MaxPosts, "MaxPosts")
			assert.Equal(t, tt.want.MaxPages, got.MaxPages, "MaxPages")
			assert.Equal(t, tt.want.SampleSize, got.SampleSize, "SampleSize")
		})
	}
}

// TestMergeConfigWithJobDataKeepsInBoundsValuesAndCliBase verifies that
// in-bounds job values are used as-is and that a job leaving a field at zero
// keeps the operator's CLI value (which is not clamped).
func TestMergeConfigWithJobDataKeepsInBoundsValuesAndCliBase(t *testing.T) {
	base := common.CrawlerConfig{
		Platform:    "telegram",
		Concurrency: 4,
		MaxDepth:    2,
		MaxPosts:    500,
		MaxPages:    1000,
		SampleSize:  10,
	}

	job := JobData{
		MaxDepth:    3,
		Concurrency: 6,
		MaxPosts:    750,
		MaxPages:    5000,
		SampleSize:  100,
	}

	got := mergeConfigWithJobData(base, job)
	assert.Equal(t, 3, got.MaxDepth)
	assert.Equal(t, 6, got.Concurrency)
	assert.Equal(t, 750, got.MaxPosts)
	assert.Equal(t, 5000, got.MaxPages)
	assert.Equal(t, 100, got.SampleSize)

	// A job that leaves a field unset must not override the CLI base.
	got = mergeConfigWithJobData(base, JobData{})
	assert.Equal(t, base, got)
}

// TestCrawlJobsRunOneAtATime verifies that a second crawl job cannot start
// (and therefore cannot share or close the global connection pool) while a
// first crawl is still running: executeCrawlJob blocks on the crawl lock.
func TestCrawlJobsRunOneAtATime(t *testing.T) {
	crawlMu.Lock()

	done := make(chan error, 1)
	go func() {
		// No URLs: with the lock held this must never even be reached.
		done <- executeCrawlJob(context.Background(), "telegram-crawl", JobData{
			Task:     "crawl",
			Platform: "telegram",
		})
	}()

	select {
	case err := <-done:
		t.Fatalf("executeCrawlJob ran while another crawl holds the crawl lock (err=%v)", err)
	case <-time.After(300 * time.Millisecond):
		// Still blocked, as required.
	}

	crawlMu.Unlock()

	select {
	case err := <-done:
		// Once the lock is released the job proceeds and fails at the
		// expected validation point (no URLs provided).
		assert.ErrorContains(t, err, "no URLs provided")
	case <-time.After(5 * time.Second):
		t.Fatal("executeCrawlJob did not finish after the crawl lock was released")
	}
}
