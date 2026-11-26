package common

import (
	"fmt"
	"regexp"
	"strings"
)

// crawlIDPattern matches crawl identifiers that are safe to use as a single
// storage-path segment or state-store key: 1-64 characters of letters, digits,
// underscore or hyphen. Generated crawl IDs (a "YYYYMMDDHHMMSS" timestamp) and
// operator-chosen IDs such as "telegram-daily" match it; anything that could
// traverse into another crawl's namespace ("../other", "a/b", "a:b", "a b")
// does not.
var crawlIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// telegramChannelNamePattern matches Telegram channel usernames. Telegram
// itself allows [A-Za-z0-9_]{4,32}; the crawler's storage-safe superset also
// permits hyphens and the full 1-64 length.
var telegramChannelNamePattern = crawlIDPattern

// youTubeChannelIDPattern matches YouTube channel identifiers the crawler
// accepts: canonical "UC..." channel IDs, "@handle" usernames and plain
// alphanumeric names. Every accepted form is a safe storage-path segment.
var youTubeChannelIDPattern = regexp.MustCompile(`^(UC[A-Za-z0-9_-]{10,64}|@[A-Za-z0-9._-]{3,64}|[A-Za-z0-9_-]{1,64})$`)

// ValidateCrawlID checks a crawl ID before it is concatenated into storage
// paths or used as a state-store key (state keys "crawlID/metadata",
// "crawlID/layer_map", "active_crawl/crawlID" and the file and blob names
// passed to the storage binding). A crawl ID that does not match
// ^[A-Za-z0-9_-]{1,64}$ is refused: such an ID could carry "/" or ".."
// segments and write into, or overwrite the resume state of, another crawl.
func ValidateCrawlID(crawlID string) error {
	if crawlID == "" {
		return fmt.Errorf("crawl ID is empty")
	}
	if !crawlIDPattern.MatchString(crawlID) {
		return fmt.Errorf("invalid crawl ID %q: must be 1-64 characters of letters, digits, '_' or '-' (^[A-Za-z0-9_-]{1,64}$); '/' and '..' are not allowed", crawlID)
	}
	return nil
}

// ValidateCrawlExecutionID checks a crawl execution ID with the same rules as
// ValidateCrawlID. An empty execution ID is allowed for callers that do not
// track executions (the state layer then falls back to legacy behaviour).
func ValidateCrawlExecutionID(executionID string) error {
	if executionID == "" {
		return nil
	}
	return ValidateCrawlID(executionID)
}

// CheckCrawlOwnership decides whether a run may start against a crawl ID's
// existing state. owner is the job name recorded in the crawl's active_crawl
// entry (empty when the crawl has no recorded owner, e.g. a legacy entry);
// jobName is the name of the job requesting to run now (empty for manual,
// non-scheduler runs). The run is refused when the crawl was last owned by a
// DIFFERENT scheduler job, so one job cannot overwrite another job's resume
// state. Re-running the same job and manual resumes are allowed.
func CheckCrawlOwnership(owner, jobName string) error {
	if owner != "" && jobName != "" && owner != jobName {
		return fmt.Errorf("crawl is owned by job %q; refusing to start job %q against it (choose a different crawl ID)", owner, jobName)
	}
	return nil
}

// ValidateChannelName checks a channel or seed identifier before it becomes a
// path segment under the crawl's storage directory. It is deliberately more
// permissive than ValidateCrawlID because channel identifiers are
// platform-specific: Bluesky DIDs contain ":", YouTube handles start with "@"
// and Bluesky handles contain ".". What it must refuse everywhere is anything
// that can escape the crawl directory: path separators, "." and ".." segments,
// control characters and whitespace padding.
func ValidateChannelName(channelID string) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is empty")
	}
	if len(channelID) > 200 {
		return fmt.Errorf("channel ID %q is too long (over 200 characters)", channelID)
	}
	if strings.TrimSpace(channelID) != channelID {
		return fmt.Errorf("channel ID %q must not start or end with whitespace", channelID)
	}
	if strings.ContainsAny(channelID, "/\\") {
		return fmt.Errorf("channel ID %q must not contain path separators", channelID)
	}
	if channelID == "." || channelID == ".." {
		return fmt.Errorf("channel ID %q is not a usable path segment", channelID)
	}
	for _, r := range channelID {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("channel ID %q must not contain control characters", channelID)
		}
	}
	return nil
}

// ValidateTelegramChannelName checks a Telegram channel username before it is
// resolved against TDLib and used as the channel's storage directory. Telegram
// usernames are [A-Za-z0-9_]; a URL or anything containing "/" or other
// punctuation is refused ("https://t.me/name" is not a username).
func ValidateTelegramChannelName(channelName string) error {
	if !telegramChannelNamePattern.MatchString(channelName) {
		return fmt.Errorf("invalid Telegram channel name %q: must be 1-64 characters of letters, digits, '_' or '-'", channelName)
	}
	return nil
}

// ValidateYouTubeChannelID checks a YouTube channel identifier before it is
// used with the YouTube API and as the channel's storage directory.
func ValidateYouTubeChannelID(channelID string) error {
	if !youTubeChannelIDPattern.MatchString(channelID) {
		return fmt.Errorf("invalid YouTube channel ID %q: must be a \"UC...\" channel ID, an \"@handle\", or 1-64 characters of letters, digits, '_' or '-'", channelID)
	}
	return nil
}
