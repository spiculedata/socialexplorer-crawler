package bluesky

import (
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/crawler"
)

// TestValidateTargetRefusesUnsafeTargetID checks that a target ID that could
// escape the channel's storage directory under the crawl is refused
// (spiculedata/social-explorer#394).
func TestValidateTargetRefusesUnsafeTargetID(t *testing.T) {
	c := &BlueskyCrawler{}
	for _, id := range []string{
		"did:plc:../../other", // traversal inside a DID
		"did:web:sub/dir",     // path separator inside a DID
		"user..bsky.social/../escape",
		"user.bsky.social/sub",
	} {
		if err := c.ValidateTarget(crawler.CrawlTarget{Type: crawler.PlatformBluesky, ID: id}); err == nil {
			t.Errorf("ValidateTarget(%q) unexpectedly accepted an unsafe target ID", id)
		}
	}
}

func TestValidateTargetAcceptsSafeTargetIDs(t *testing.T) {
	c := &BlueskyCrawler{}
	for _, id := range []string{
		"firehose",
		"did:plc:z72i7hdynmk6r22z27h6tvur",
		"did:web:example.com",
		"user.bsky.social",
	} {
		if err := c.ValidateTarget(crawler.CrawlTarget{Type: crawler.PlatformBluesky, ID: id}); err != nil {
			t.Errorf("ValidateTarget(%q) returned error: %v", id, err)
		}
	}
}
