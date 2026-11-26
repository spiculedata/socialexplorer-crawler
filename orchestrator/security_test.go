package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/researchaccelerator-hub/telegram-scraper/distributed"
)

// TestHandleStatusMessageBoundsWorkerRegistry verifies that the orchestrator's
// workers map cannot grow without limit: once the registry is full, status
// messages from unknown worker IDs are ignored instead of registering new
// entries. Anyone who can publish to the status topic must not be able to
// exhaust orchestrator memory with unique worker IDs.
func TestHandleStatusMessageBoundsWorkerRegistry(t *testing.T) {
	orch := &Orchestrator{
		workers:    make(map[string]*WorkerInfo),
		maxWorkers: 3,
	}

	// Register up to the cap.
	for i := 0; i < orch.maxWorkers; i++ {
		msg := newTestStatusMessage(t, "worker-registered")
		msg.WorkerID = "worker-" + string(rune('a'+i))
		assert.NoError(t, orch.handleStatusMessage(t.Context(), msg))
	}
	assert.Len(t, orch.workers, orch.maxWorkers)

	// Additional unique worker IDs must not grow the registry.
	for i := 0; i < 10; i++ {
		msg := newTestStatusMessage(t, "worker-registered")
		msg.WorkerID = "intruder-" + string(rune('a'+i))
		assert.NoError(t, orch.handleStatusMessage(t.Context(), msg))
	}
	assert.Len(t, orch.workers, orch.maxWorkers)

	// Known workers keep updating normally.
	msg := newTestStatusMessage(t, "worker-registered")
	msg.WorkerID = "worker-a"
	msg.TasksProcessed = 7
	assert.NoError(t, orch.handleStatusMessage(t.Context(), msg))
	assert.Equal(t, 7, orch.workers["worker-a"].TasksTotal)
}

// TestDefaultMaxWorkersIsPositive guards the default registry cap.
func TestDefaultMaxWorkersIsPositive(t *testing.T) {
	assert.Greater(t, DefaultMaxWorkers, 0)
}

func newTestStatusMessage(t *testing.T, status string) distributed.StatusMessage {
	t.Helper()
	msg := distributed.NewStatusMessage(
		"worker-registered",
		distributed.MessageTypeHeartbeat,
		status,
		1, 1, 0,
		0,
	)
	require.NotNil(t, msg)
	return msg
}
