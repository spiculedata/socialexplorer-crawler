package telegramhelper

import (
	"context"
	"strings"
	"testing"

	"github.com/researchaccelerator-hub/telegram-scraper/common"
	"github.com/researchaccelerator-hub/telegram-scraper/crawler"
	"github.com/rs/zerolog"
	zerologlog "github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

// TestPoolLogsDoNotContainSessionArchiveURL verifies that the connection pool
// never writes the TDLib session-archive URL (a bearer-style link to a
// logged-in Telegram session) to the captured log output, across pool
// preloading, fresh connection creation and error paths.
func TestPoolLogsDoNotContainSessionArchiveURL(t *testing.T) {
	const markerURL = "http://127.0.0.1:9/MARKER-SESSION-ARCHIVE.tgz"

	var buf strings.Builder

	origLogger := zerologlog.Logger
	zerologlog.Logger = zerolog.New(&buf).With().Timestamp().Logger()
	defer func() { zerologlog.Logger = origLogger }()

	// Successful preload: pool adds a pre-seeded connection.
	mockService := &MockPoolTelegramService{}
	pool := &ConnectionPool{
		availableConns: make(map[string]crawler.TDLibClient),
		inUseConns:     make(map[string]crawler.TDLibClient),
		maxSize:        2,
		service:        mockService,
		storagePrefix:  "test",
		defaultConfig: common.CrawlerConfig{
			TDLibDatabaseURLs: []string{markerURL},
		},
		connDirMap: make(map[string]string),
	}
	pool.PreloadConnections([]string{markerURL})

	ctx := context.Background()
	_, connID, err := pool.GetConnection(ctx)
	assert.NoError(t, err)
	// Force a fresh connection after an error: cycles the archive URL again.
	_, _, err = pool.HandleConnectionError(ctx, connID)
	assert.NoError(t, err)

	// Failing init: the error path must not include the URL either.
	failingService := &MockPoolTelegramService{CreateError: assert.AnError}
	failingPool := &ConnectionPool{
		availableConns: make(map[string]crawler.TDLibClient),
		inUseConns:     make(map[string]crawler.TDLibClient),
		maxSize:        1,
		service:        failingService,
		storagePrefix:  "test",
		defaultConfig: common.CrawlerConfig{
			TDLibDatabaseURLs: []string{markerURL},
		},
		connDirMap: make(map[string]string),
	}
	failingPool.PreloadConnections([]string{markerURL})

	out := buf.String()

	// Positive controls: the pool code paths actually ran and logged.
	assert.Contains(t, out, "Added connection to pool")
	assert.Contains(t, out, "Creating fresh connection after error")
	assert.Contains(t, out, "Failed to initialize client for pool")

	assert.NotContains(t, out, markerURL,
		"the session archive URL must never appear in log output")
}
