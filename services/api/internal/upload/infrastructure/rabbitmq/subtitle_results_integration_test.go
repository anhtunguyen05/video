package rabbitmq

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rabbitmq/amqp091-go"
)

func TestSubtitleResultProjectionIsIdempotentAndOwnerScoped(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires isolated TEST_DATABASE_URL")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	job, video, owner, event, run := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	_, err = db.ExecContext(ctx, `INSERT INTO videos (id,owner_id,title,status) VALUES ($1,$2,'subtitle projection','UPLOADED')`, video, owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM subtitle_requests WHERE id=$1`, job)
		_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id=$1`, video)
	})
	_, err = db.ExecContext(ctx, `INSERT INTO subtitle_requests (id,video_id,owner_id,source_version,language,status) VALUES ($1,$2,$3,1,'vi','PENDING')`, job, video, owner)
	if err != nil {
		t.Fatal(err)
	}
	key := "users/" + owner + "/videos/" + video + "/subtitles/v1/runs/" + run + "/vi.vtt"
	encode := func(status string, output any) []byte {
		body, _ := json.Marshal(map[string]any{"message_id": event, "message_type": "subtitle.completed.v1", "schema_version": 1,
			"payload": map[string]any{"job_id": job, "video_id": video, "owner_id": owner, "source_version": 1, "language": "vi", "status": status, "output_object_key": output}})
		return body
	}
	ready := amqp091.Delivery{MessageId: event, Body: encode("READY", key)}
	wrongOwner := uuid.NewString()
	wrongBody, _ := json.Marshal(map[string]any{"message_id": event, "message_type": "subtitle.completed.v1", "schema_version": 1,
		"payload": map[string]any{"job_id": job, "video_id": video, "owner_id": wrongOwner, "source_version": 1, "language": "vi", "status": "FAILED", "output_object_key": nil}})
	if err := applySubtitleResult(ctx, db, amqp091.Delivery{MessageId: event, Body: wrongBody}); err != nil {
		t.Fatal(err)
	}
	var initial string
	if err := db.QueryRowContext(ctx, `SELECT status FROM subtitle_requests WHERE id=$1`, job).Scan(&initial); err != nil || initial != "PENDING" {
		t.Fatalf("wrong owner changed status: %s %v", initial, err)
	}
	if err := applySubtitleResult(ctx, db, ready); err != nil {
		t.Fatal(err)
	}
	if err := applySubtitleResult(ctx, db, ready); err != nil {
		t.Fatal(err)
	}
	if err := applySubtitleResult(ctx, db, amqp091.Delivery{MessageId: event, Body: encode("FAILED", nil)}); err != nil {
		t.Fatal(err)
	}
	var status, gotKey string
	if err := db.QueryRowContext(ctx, `SELECT status,output_object_key FROM subtitle_requests WHERE id=$1`, job).Scan(&status, &gotKey); err != nil {
		t.Fatal(err)
	}
	if status != "READY" || gotKey != key {
		t.Fatalf("unexpected projection: %s %s", status, gotKey)
	}
}
