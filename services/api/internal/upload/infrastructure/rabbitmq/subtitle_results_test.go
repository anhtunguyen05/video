package rabbitmq

import (
	"encoding/json"
	"testing"
)

func TestParseSubtitleResultChecksIdentityAndTrackPath(t *testing.T) {
	const eventID = "11111111-1111-4111-8111-111111111111"
	const jobID = "22222222-2222-4222-8222-222222222222"
	const owner = "33333333-3333-4333-8333-333333333333"
	const video = "44444444-4444-4444-8444-444444444444"
	const run = "55555555-5555-4555-8555-555555555555"
	path := "users/" + owner + "/videos/" + video + "/subtitles/v1/runs/" + run + "/vi.vtt"
	body, _ := json.Marshal(map[string]any{"message_id": eventID, "message_type": "subtitle.completed.v1", "schema_version": 1,
		"payload": map[string]any{"job_id": jobID, "video_id": video, "owner_id": owner, "source_version": 1, "language": "vi", "status": "READY", "output_object_key": path}})
	if _, err := parseSubtitleResult(body, eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSubtitleResult(body, jobID); err == nil {
		t.Fatal("accepted wrong broker identity")
	}
	bad := []byte(string(body) + " garbage")
	if _, err := parseSubtitleResult(bad, eventID); err == nil {
		t.Fatal("accepted invalid json")
	}
	var event map[string]any
	_ = json.Unmarshal(body, &event)
	event["payload"].(map[string]any)["output_object_key"] = "users/other/track.vtt"
	bad, _ = json.Marshal(event)
	if _, err := parseSubtitleResult(bad, eventID); err == nil {
		t.Fatal("accepted another owner's output")
	}
}
