package orchestrator

import (
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/common"
)

// TestNewOrchestratorRefusesInvalidCrawlID checks that a crawl ID that could
// escape the crawl's namespace (path separators, "..") is refused before any
// state keys, storage paths or pub/sub resources are created
// (spiculedata/social-explorer#394).
func TestNewOrchestratorRefusesInvalidCrawlID(t *testing.T) {
	for _, crawlID := range []string{"../other", "other/../state", "a/b", "crawl:1", "crawl id", ""} {
		_, err := NewOrchestrator(crawlID, common.CrawlerConfig{})
		if err == nil {
			t.Errorf("NewOrchestrator(crawlID=%q) unexpectedly accepted an unsafe crawl ID", crawlID)
		}
	}
}
