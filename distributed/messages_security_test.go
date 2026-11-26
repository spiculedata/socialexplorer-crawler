package distributed

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestWorkItemConfigJSONExcludesCredentials verifies that a work item
// serialized for the pub/sub work queue contains neither a storage root nor
// an API key: workers take both from their own configuration, so no
// credential or filesystem path is ever published to the topic.
func TestWorkItemConfigJSONExcludesCredentials(t *testing.T) {
	item := NewWorkItem("https://t.me/testchannel", 1, "parent", "crawl-id", "telegram", WorkItemConfig{
		Concurrency:    2,
		Timeout:        30,
		SamplingMethod: "random",
	})

	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal work item: %v", err)
	}

	payloadStr := string(payload)
	if strings.Contains(payloadStr, "storage_root") {
		t.Errorf("work item JSON must not contain 'storage_root': %s", payloadStr)
	}
	if strings.Contains(payloadStr, "youtube_api_key") {
		t.Errorf("work item JSON must not contain 'youtube_api_key': %s", payloadStr)
	}
}

// TestWorkItemConfigIgnoresLegacyCredentialFields verifies that work items
// published by producers still using the old wire format — where config
// carried storage_root and youtube_api_key — are decoded without those
// values: unknown JSON fields do not populate WorkItemConfig, and the
// re-marshaled message contains no credential fields.
func TestWorkItemConfigIgnoresLegacyCredentialFields(t *testing.T) {
	legacy := `{
		"message_type": "work_item",
		"work_item": {
			"id": "item-1",
			"url": "https://t.me/testchannel",
			"depth": 1,
			"crawl_id": "crawl-1",
			"platform": "telegram",
			"config": {
				"storage_root": "/attacker/chosen/path",
				"youtube_api_key": "attacker-key",
				"concurrency": 2
			}
		}
	}`

	var message WorkQueueMessage
	if err := json.Unmarshal([]byte(legacy), &message); err != nil {
		t.Fatalf("failed to unmarshal legacy work queue message: %v", err)
	}

	if message.WorkItem.Config.Concurrency != 2 {
		t.Errorf("expected concurrency 2 to be decoded, got %d", message.WorkItem.Config.Concurrency)
	}

	repacked, err := json.Marshal(message.WorkItem)
	if err != nil {
		t.Fatalf("failed to re-marshal work item: %v", err)
	}

	repackedStr := string(repacked)
	if strings.Contains(repackedStr, "/attacker/chosen/path") {
		t.Errorf("legacy storage_root must not survive decoding: %s", repackedStr)
	}
	if strings.Contains(repackedStr, "attacker-key") {
		t.Errorf("legacy API key must not survive decoding: %s", repackedStr)
	}
}
