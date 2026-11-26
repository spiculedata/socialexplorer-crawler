package common

import (
	"strings"
	"testing"
)

func TestValidateCrawlIDAcceptsSafeIDs(t *testing.T) {
	for _, id := range []string{
		"20260102150405", // generated timestamp ID
		"telegram-daily",
		"crawl_1",
		"A",
		"Abc-123_xyz",
		strings.Repeat("a", 64),
	} {
		if err := ValidateCrawlID(id); err != nil {
			t.Errorf("ValidateCrawlID(%q) returned error: %v", id, err)
		}
	}
}

func TestValidateCrawlIDRefusesTraversalAndUnsafeIDs(t *testing.T) {
	for _, id := range []string{
		"../other",              // path traversal
		"other/../state",        // path traversal
		"../../metadata",        // path traversal
		"a/b",                   // path separator splits the segment
		"sub/dir",               // path separator splits the segment
		"crawl:1",               // colon is reserved for platform identifiers
		"crawl id",              // whitespace
		" crawl",                // leading whitespace
		"crawl\ttab",            // control character
		"",                      // empty
		strings.Repeat("a", 65), // over the 64 character limit
	} {
		if err := ValidateCrawlID(id); err == nil {
			t.Errorf("ValidateCrawlID(%q) unexpectedly accepted an unsafe crawl ID", id)
		}
	}
}

func TestValidateCrawlIDRefusesDotDotAsOwnSegment(t *testing.T) {
	// The acceptance criterion calls out ".." specifically.
	if err := ValidateCrawlID(".."); err == nil {
		t.Error("ValidateCrawlID(\"..\") unexpectedly accepted '..'")
	}
}

func TestCheckCrawlOwnership(t *testing.T) {
	cases := []struct {
		name    string
		owner   string
		jobName string
		refuse  bool
	}{
		{"same job re-runs its own crawl", "daily-telegram", "daily-telegram", false},
		{"other job reuses the crawl ID", "daily-telegram", "hourly-youtube", true},
		{"legacy entry without an owner", "", "daily-telegram", false},
		{"manual run without a job name", "daily-telegram", "", false},
		{"nothing recorded", "", "", false},
	}
	for _, tc := range cases {
		err := CheckCrawlOwnership(tc.owner, tc.jobName)
		if tc.refuse && err == nil {
			t.Errorf("%s: expected a refusal, got nil", tc.name)
		}
		if !tc.refuse && err != nil {
			t.Errorf("%s: unexpected refusal: %v", tc.name, err)
		}
	}
}

func TestValidateCrawlExecutionIDAllowsEmpty(t *testing.T) {
	if err := ValidateCrawlExecutionID(""); err != nil {
		t.Errorf("ValidateCrawlExecutionID(\"\") returned error: %v", err)
	}
	if err := ValidateCrawlExecutionID("../other"); err == nil {
		t.Error("ValidateCrawlExecutionID(\"../other\") unexpectedly accepted a traversal execution ID")
	}
}

func TestValidateChannelNameAcceptsPlatformIdentifiers(t *testing.T) {
	for _, name := range []string{
		"did:plc:z72i7hdynmk6r22z27h6tvur", // Bluesky DID
		"username.bsky.social",             // Bluesky handle
		"firehose",                         // Bluesky full-network target
		"UC1234567890abcdefghij",           // YouTube channel ID
		"@somehandle",                      // YouTube handle
		"somechannel",                      // Telegram username
		"posts.jsonl-key_2",                // punctuation within a segment
	} {
		if err := ValidateChannelName(name); err != nil {
			t.Errorf("ValidateChannelName(%q) returned error: %v", name, err)
		}
	}
}

func TestValidateChannelNameRefusesEscapeAttempts(t *testing.T) {
	for _, name := range []string{
		"",                       // empty
		"..",                     // parent directory
		".",                      // current directory
		"sub/dir",                // forward slash
		"sub\\dir",               // backslash
		"../other",               // traversal prefix
		" channel",               // leading whitespace
		"channel ",               // trailing whitespace
		"chan\x00nel",            // NUL byte
		"chan\x1fnel",            // control character
		strings.Repeat("a", 201), // over the length limit
	} {
		if err := ValidateChannelName(name); err == nil {
			t.Errorf("ValidateChannelName(%q) unexpectedly accepted an unsafe channel ID", name)
		}
	}
}

func TestValidateTelegramChannelName(t *testing.T) {
	if err := ValidateTelegramChannelName("somechannel"); err != nil {
		t.Errorf("ValidateTelegramChannelName(\"somechannel\") returned error: %v", err)
	}
	for _, name := range []string{
		"https://t.me/somechannel", // URL, not a username
		"@somechannel",             // mention form
		"sub/dir",
		"..",
		"",
	} {
		if err := ValidateTelegramChannelName(name); err == nil {
			t.Errorf("ValidateTelegramChannelName(%q) unexpectedly accepted an unsafe name", name)
		}
	}
}

func TestValidateYouTubeChannelID(t *testing.T) {
	for _, id := range []string{
		"UC1234567890abcdefghij",
		"@somehandle",
		"somechannel",
	} {
		if err := ValidateYouTubeChannelID(id); err != nil {
			t.Errorf("ValidateYouTubeChannelID(%q) returned error: %v", id, err)
		}
	}
	for _, id := range []string{
		"sub/dir",
		"..",
		"UC has spaces",
		"",
		"UC1234567890abcdefghij!extra",
	} {
		if err := ValidateYouTubeChannelID(id); err == nil {
			t.Errorf("ValidateYouTubeChannelID(%q) unexpectedly accepted an unsafe ID", id)
		}
	}
}
