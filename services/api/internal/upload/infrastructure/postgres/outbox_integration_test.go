package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestCompleteUploadCommitsOneOutboxEvent(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	videoID, ownerID, uploadID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	key := "users/" + ownerID + "/videos/" + videoID + "/source/source.mp4"
	if _, err := db.ExecContext(ctx, `INSERT INTO videos (id, owner_id, title, status)
		VALUES ($1, $2, 'outbox test', 'UPLOADING')`, videoID, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE event_key = $1`, videoID+":UPLOAD:v1")
		_, _ = db.ExecContext(ctx, `DELETE FROM video_uploads WHERE id = $1`, uploadID)
		_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id = $1`, videoID)
	})
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO video_uploads
		(id, video_id, object_key, status, expected_content_type, expected_size_bytes, expires_at)
		VALUES ($1, $2, $3, 'UPLOADING', 'video/mp4', 1024, $4)`, uploadID, videoID, key, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	repository := NewRepository(db)
	session, err := repository.CompleteOwned(ctx, ownerID, uploadID, 1024, now)
	if err != nil {
		t.Fatal(err)
	}
	if string(session.Status) != "COMPLETED" {
		t.Fatalf("upload status = %s", session.Status)
	}
	var count int
	var payload []byte
	if err := db.QueryRowContext(ctx, `SELECT count(*), min(payload::text) FROM outbox_events WHERE event_key = $1`,
		videoID+":UPLOAD:v1").Scan(&count, &payload); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("outbox events = %d", count)
	}
	var event struct {
		MessageID string `json:"message_id"`
		Payload   struct {
			VideoID           string `json:"video_id"`
			OwnerID           string `json:"owner_id"`
			SourceObjectKey   string `json:"source_object_key"`
			ProcessingVersion int    `json:"processing_version"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.MessageID == "" || event.Payload.VideoID != videoID || event.Payload.OwnerID != ownerID ||
		event.Payload.SourceObjectKey != key || event.Payload.ProcessingVersion != 1 {
		t.Fatalf("unexpected outbox payload: %s", payload)
	}
}
