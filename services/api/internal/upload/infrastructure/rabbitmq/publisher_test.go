package rabbitmq

import (
	"encoding/json"
	"testing"

	"video/services/api/internal/upload/ports"
)

func TestEnvelopeMatchesVideoUploadedContract(t *testing.T) {
	payload := ports.VideoUploaded{
		VideoID:           "video-1",
		OwnerID:           "owner-1",
		SourceObjectKey:   "users/owner-1/videos/video-1/source/source.mp4",
		ProcessingVersion: 2,
		CorrelationID:     "correlation-1",
		CausationID:       "upload-1",
	}

	encoded, err := json.Marshal(envelope{
		MessageID:     "message-1",
		MessageType:   RoutingKey,
		SchemaVersion: 1,
		CorrelationID: payload.CorrelationID,
		CausationID:   payload.CausationID,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	payloadObject := decoded["payload"].(map[string]any)
	if payloadObject["video_id"] != "video-1" || payloadObject["owner_id"] != "owner-1" || payloadObject["source_object_key"] != payload.SourceObjectKey || payloadObject["processing_version"] != float64(2) {
		t.Fatalf("unexpected payload: %#v", payloadObject)
	}
	if _, ok := payloadObject["CorrelationID"]; ok {
		t.Fatal("payload must not include envelope correlation fields")
	}
}
