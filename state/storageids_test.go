package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/model"
)

// TestNewDaprStateManagerRefusesUnsafeCrawlID checks that a crawl ID carrying
// traversal or separator characters is refused before any state key or
// storage path is built from it (spiculedata/social-explorer#394).
func TestNewDaprStateManagerRefusesUnsafeCrawlID(t *testing.T) {
	for _, crawlID := range []string{
		"../other",
		"other/../state",
		"a/b",
		"crawl:1",
		"crawl id",
	} {
		_, err := NewDaprStateManager(Config{
			CrawlID:     crawlID,
			DaprConfig:  &DaprConfig{StateStoreName: "statestore", ComponentName: "statestore"},
			StorageRoot: "/tmp/storage",
		})
		if err == nil {
			t.Errorf("NewDaprStateManager(crawlID=%q) unexpectedly accepted an unsafe crawl ID", crawlID)
		}
	}
}

func TestNewDaprStateManagerRefusesUnsafeExecutionID(t *testing.T) {
	_, err := NewDaprStateManager(Config{
		CrawlID:          "good-crawl",
		CrawlExecutionID: "../other",
		DaprConfig:       &DaprConfig{StateStoreName: "statestore", ComponentName: "statestore"},
		StorageRoot:      "/tmp/storage",
	})
	if err == nil {
		t.Error("NewDaprStateManager(executionID=../other) unexpectedly accepted a traversal execution ID")
	}
}

// TestNewLocalStateManagerRefusesUnsafeCrawlID checks the same refusal for the
// local filesystem backend, where filepath.Join would resolve "../" out of the
// crawl directory directly.
func TestNewLocalStateManagerRefusesUnsafeCrawlID(t *testing.T) {
	base := t.TempDir()
	for _, crawlID := range []string{"../other", "a/b", ".."} {
		_, err := NewLocalStateManager(Config{
			CrawlID:     crawlID,
			LocalConfig: &LocalConfig{BasePath: base},
		})
		if err == nil {
			t.Errorf("NewLocalStateManager(crawlID=%q) unexpectedly accepted an unsafe crawl ID", crawlID)
		}
	}
	// The traversal must not have created directories outside the base path.
	entries, err := os.ReadDir(filepath.Dir(base))
	if err != nil {
		t.Fatalf("failed to list parent directory: %v", err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(base) {
			t.Errorf("unsafe crawl ID created a directory outside the base path: %s", e.Name())
		}
	}
}

func TestLocalStateManagerRefusesUnsafeChannelName(t *testing.T) {
	base := t.TempDir()
	lsm, err := NewLocalStateManager(Config{
		CrawlID:     "good-crawl",
		LocalConfig: &LocalConfig{BasePath: base},
	})
	if err != nil {
		t.Fatalf("NewLocalStateManager failed: %v", err)
	}

	for _, channelID := range []string{"../other", "sub/dir", "..", "chan\x00nel"} {
		if err := lsm.StorePost(channelID, model.Post{PostUID: "1-ok"}); err == nil {
			t.Errorf("StorePost(channelID=%q) unexpectedly accepted an unsafe channel ID", channelID)
		}
	}

	// A safe channel ID stores normally.
	if err := lsm.StorePost("somechannel", model.Post{PostUID: "1-ok"}); err != nil {
		t.Errorf("StorePost with a safe channel ID failed: %v", err)
	}
	// And nothing escaped the crawl directory.
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if e.Name() != "good-crawl" {
			t.Errorf("unexpected entry %q created outside the crawl directory", e.Name())
		}
	}
}

func TestLocalStateManagerRefusesUnsafeStoreFile(t *testing.T) {
	base := t.TempDir()
	lsm, err := NewLocalStateManager(Config{
		CrawlID:     "good-crawl",
		LocalConfig: &LocalConfig{BasePath: base},
	})
	if err != nil {
		t.Fatalf("NewLocalStateManager failed: %v", err)
	}

	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("data"), 0o600); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	for _, tc := range []struct{ channelID, fileName string }{
		{"../other", "file.txt"},
		{"somechannel", "../escape.txt"},
	} {
		if _, _, err := lsm.StoreFile(tc.channelID, source, tc.fileName); err == nil {
			t.Errorf("StoreFile(channelID=%q, fileName=%q) unexpectedly accepted unsafe identifiers", tc.channelID, tc.fileName)
		}
	}
}

func TestLocalStateManagerGetCrawlOwnerUnowned(t *testing.T) {
	base := t.TempDir()
	lsm, err := NewLocalStateManager(Config{
		CrawlID:     "good-crawl",
		LocalConfig: &LocalConfig{BasePath: base},
	})
	if err != nil {
		t.Fatalf("NewLocalStateManager failed: %v", err)
	}
	owner, err := lsm.GetCrawlOwner("some-crawl")
	if err != nil {
		t.Fatalf("GetCrawlOwner failed: %v", err)
	}
	if owner != "" {
		t.Errorf("GetCrawlOwner returned %q, want empty for the local backend", owner)
	}
}

func TestDaprStateManagerGenerateStoragePathRefusesUnsafeChannelName(t *testing.T) {
	dsm := &DaprStateManager{
		BaseStateManager: NewBaseStateManager(Config{
			CrawlID:     "good-crawl",
			StorageRoot: "/tmp/storage",
		}),
	}
	for _, channelID := range []string{"../other", "sub/dir", ".."} {
		_, err := dsm.generateStoragePath(channelID, "posts/1.jsonl")
		if err == nil {
			t.Errorf("generateStoragePath(%q) unexpectedly accepted an unsafe channel ID", channelID)
		}
		_, err = dsm.generateCrawlExecutableStoragePath(channelID, "posts/1.jsonl")
		if err == nil {
			t.Errorf("generateCrawlExecutableStoragePath(%q) unexpectedly accepted an unsafe channel ID", channelID)
		}
	}

	// A valid path for a Bluesky DID shows platform identifiers still work.
	path, err := dsm.generateStoragePath("did:plc:z72i7hdynmk6r22z27h6tvur", "posts/1.jsonl")
	if err != nil {
		t.Fatalf("generateStoragePath with a DID failed: %v", err)
	}
	if !strings.HasPrefix(path, "/tmp/storage/good-crawl/did:plc:z72i7hdynmk6r22z27h6tvur/") {
		t.Errorf("unexpected storage path %q", path)
	}
}
