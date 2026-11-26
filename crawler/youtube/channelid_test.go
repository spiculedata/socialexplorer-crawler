package youtube

import (
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/crawler"
)

// TestValidateTargetRefusesUnsafeChannelID checks that a target ID that could
// escape the channel's storage directory under the crawl is refused
// (spiculedata/social-explorer#394).
func TestValidateTargetRefusesUnsafeChannelID(t *testing.T) {
	c := &YouTubeCrawler{}
	for _, id := range []string{"../other", "sub/dir", "..", "UC has spaces"} {
		if err := c.ValidateTarget(crawler.CrawlTarget{Type: crawler.PlatformYouTube, ID: id}); err == nil {
			t.Errorf("ValidateTarget(%q) unexpectedly accepted an unsafe channel ID", id)
		}
	}
}

func TestValidateTargetAcceptsSafeChannelIDs(t *testing.T) {
	c := &YouTubeCrawler{}
	for _, id := range []string{"UC1234567890abcdefghij", "@somehandle", "somechannel"} {
		if err := c.ValidateTarget(crawler.CrawlTarget{Type: crawler.PlatformYouTube, ID: id}); err != nil {
			t.Errorf("ValidateTarget(%q) returned error: %v", id, err)
		}
	}
}
