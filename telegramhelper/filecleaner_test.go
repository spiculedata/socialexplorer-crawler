package telegramhelper

import (
	"runtime"
	"testing"
	"time"
)

// waitForGoroutines polls until the goroutine count is at or below the
// baseline (or the timeout expires) and reports whether it settled.
func waitForGoroutines(t *testing.T, baseline int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline+1 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return runtime.NumGoroutine() <= baseline+1
}

// TestFileCleanerStopTerminatesGoroutine verifies that no cleaner goroutine
// outlives a Stop: after Stop returns the goroutine must be gone, the cleaner
// must be restartable, and a redundant Stop must be a no-op (issue 390).
func TestFileCleanerStopTerminatesGoroutine(t *testing.T) {
	baseline := runtime.NumGoroutine()

	fc := NewFileCleaner(t.TempDir(), nil, 1, 1)
	if err := fc.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	fc.Stop()

	if !waitForGoroutines(t, baseline, 2*time.Second) {
		t.Fatalf("cleaner goroutine outlived Stop: baseline=%d now=%d",
			baseline, runtime.NumGoroutine())
	}

	// A stopped cleaner must be restartable.
	if err := fc.Start(); err != nil {
		t.Fatalf("Start after Stop failed: %v", err)
	}
	fc.Stop()

	// A redundant Stop must not panic or hang.
	fc.Stop()
}
