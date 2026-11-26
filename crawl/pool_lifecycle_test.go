package crawl

import (
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/common"
)

// TestConnectionPoolRefcountLifecycle verifies that the connection pool is
// only really closed when every user of it has released it, so one finishing
// job can no longer close the pool out from under a concurrent job (issue 390).
func TestConnectionPoolRefcountLifecycle(t *testing.T) {
	resetPoolForTest := func() {
		poolMu.Lock()
		connectionPool = nil
		poolUseCount = 0
		poolMu.Unlock()
	}
	resetPoolForTest()
	defer resetPoolForTest()

	cfg := common.CrawlerConfig{}

	// First job takes the pool.
	InitConnectionPool(2, t.TempDir(), cfg)
	if !IsConnectionPoolInitialized() {
		t.Fatal("pool should be initialized after InitConnectionPool")
	}

	// Second overlapping job shares the lifecycle and takes its own reference.
	InitConnectionPool(2, t.TempDir(), cfg)
	if !IsConnectionPoolInitialized() {
		t.Fatal("pool should still be initialized after the second InitConnectionPool")
	}

	// The first job finishes and closes "its" pool...
	CloseConnectionPool()
	if !IsConnectionPoolInitialized() {
		t.Fatal("pool was closed while a second job still holds a reference")
	}

	// ...the second job finishes and the pool finally goes away.
	CloseConnectionPool()
	if IsConnectionPoolInitialized() {
		t.Fatal("pool should be closed once every job has released it")
	}

	// Closing an already-closed pool must be a no-op.
	CloseConnectionPool()
	if IsConnectionPoolInitialized() {
		t.Fatal("pool must stay closed after a redundant CloseConnectionPool")
	}
}
