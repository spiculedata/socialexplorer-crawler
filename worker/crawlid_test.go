package worker

import (
	"context"
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/distributed"
)

// TestProcessWorkItemRefusesInvalidCrawlID checks that a work item carrying a
// crawl ID that could escape the crawl's namespace (path separators, "..") is
// refused instead of being processed (spiculedata/social-explorer#394).
func TestProcessWorkItemRefusesInvalidCrawlID(t *testing.T) {
	w := &Worker{ID: "worker-test"}

	for _, crawlID := range []string{"../other", "a/b", "crawl:1", "crawl id"} {
		_, err := w.ProcessWorkItem(context.Background(), distributed.WorkItem{
			ID:       "wi-1",
			URL:      "somechannel",
			CrawlID:  crawlID,
			Platform: "telegram",
		})
		if err == nil {
			t.Errorf("ProcessWorkItem(crawlID=%q) unexpectedly accepted an unsafe crawl ID", crawlID)
		}
	}
}

func TestProcessWorkItemAcceptsSafeCrawlIDFormat(t *testing.T) {
	// The refusal happens before any crawl work, so a supported platform whose
	// processing is not wired up (YouTube in distributed mode) still proves the
	// crawl ID format check passed.
	w := &Worker{ID: "worker-test"}
	_, err := w.ProcessWorkItem(context.Background(), distributed.WorkItem{
		ID:       "wi-2",
		URL:      "UC1234567890abcdefghij",
		CrawlID:  "daily-youtube",
		Platform: "youtube",
	})
	if err == nil {
		t.Fatal("expected the platform error, got nil")
	}
	if err.Error() != "YouTube processing in distributed mode not yet implemented" {
		t.Errorf("expected the platform-not-implemented error (crawl ID accepted), got: %v", err)
	}
}
