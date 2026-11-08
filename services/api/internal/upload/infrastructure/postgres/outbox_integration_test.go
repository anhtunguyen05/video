package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"video/services/api/internal/platform/observability"
	"video/services/api/internal/upload/ports"
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
	requestID := uuid.NewString()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previousPropagator) })
	traceparent := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	requestCtx := observability.Extract(ctx, traceparent)
	session, err := repository.CompleteOwned(observability.WithRequestID(requestCtx, requestID), ownerID, uploadID, 1024, now)
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
		MessageID     string `json:"message_id"`
		CorrelationID string `json:"correlation_id"`
		Traceparent   string `json:"traceparent"`
		Payload       struct {
			VideoID           string `json:"video_id"`
			OwnerID           string `json:"owner_id"`
			SourceObjectKey   string `json:"source_object_key"`
			ProcessingVersion int    `json:"processing_version"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.MessageID == "" || event.CorrelationID != requestID || event.Traceparent != traceparent || event.Payload.VideoID != videoID || event.Payload.OwnerID != ownerID ||
		event.Payload.SourceObjectKey != key || event.Payload.ProcessingVersion != 1 {
		t.Fatalf("unexpected outbox payload: %s", payload)
	}
}

func TestSubtitleRequestFollowsUploadChoice(t *testing.T) {
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
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			videoID, ownerID, uploadID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			key := "users/" + ownerID + "/videos/" + videoID + "/source/source.mp4"
			now := time.Now().UTC()
			if _, err := db.ExecContext(ctx, `INSERT INTO videos (id, owner_id, title, status) VALUES ($1,$2,'subtitle test','UPLOADING')`, videoID, ownerID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE event_key IN ($1,$2)`, videoID+":UPLOAD:v1", videoID+":SUBTITLE:vi:v1")
				_, _ = db.ExecContext(ctx, `DELETE FROM subtitle_requests WHERE video_id = $1`, videoID)
				_, _ = db.ExecContext(ctx, `DELETE FROM video_uploads WHERE id = $1`, uploadID)
				_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id = $1`, videoID)
			})
			if _, err := db.ExecContext(ctx, `INSERT INTO video_uploads (id,video_id,object_key,status,expected_content_type,expected_size_bytes,expires_at,generate_subtitles) VALUES ($1,$2,$3,'UPLOADING','video/mp4',1024,$4,$5)`, uploadID, videoID, key, now.Add(time.Hour), enabled); err != nil {
				t.Fatal(err)
			}
			repo := NewRepository(db)
			session, err := repo.GetOwned(ctx, ownerID, uploadID)
			if err != nil || session.GenerateSubtitles != enabled {
				t.Fatalf("choice = %v, err = %v", session.GenerateSubtitles, err)
			}
			if _, err := repo.CompleteOwned(ctx, ownerID, uploadID, 1024, now); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.CompleteOwned(ctx, ownerID, uploadID, 1024, now); !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("repeat complete error = %v, want not found at repository layer", err)
			}
			var requests, events int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM subtitle_requests WHERE video_id=$1 AND owner_id=$2 AND status='PENDING'`, videoID, ownerID).Scan(&requests); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE event_key=$1 AND routing_key='subtitle.requested.v1'`, videoID+":SUBTITLE:vi:v1").Scan(&events); err != nil {
				t.Fatal(err)
			}
			want := 0
			if enabled {
				want = 1
			}
			if requests != want || events != want {
				t.Fatalf("requests/events = %d/%d, want %d", requests, events, want)
			}
			if enabled {
				var event struct {
					MessageID   string `json:"message_id"`
					MessageType string `json:"message_type"`
					Payload     struct {
						JobID           string `json:"job_id"`
						VideoID         string `json:"video_id"`
						OwnerID         string `json:"owner_id"`
						SourceObjectKey string `json:"source_object_key"`
						SourceVersion   int    `json:"source_version"`
						Language        string `json:"language"`
					} `json:"payload"`
				}
				var body []byte
				if err := db.QueryRowContext(ctx, `SELECT payload FROM outbox_events WHERE event_key=$1`, videoID+":SUBTITLE:vi:v1").Scan(&body); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(body, &event); err != nil {
					t.Fatal(err)
				}
				if event.MessageID == "" || event.Payload.JobID != event.MessageID || event.MessageType != "subtitle.requested.v1" || event.Payload.VideoID != videoID || event.Payload.OwnerID != ownerID || event.Payload.SourceObjectKey != key || event.Payload.SourceVersion != 1 || event.Payload.Language != "vi" {
					t.Fatalf("invalid subtitle event: %s", body)
				}
			}
		})
	}
}

func TestSubtitleOutboxFailureRollsBackUploadCompletion(t *testing.T) {
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
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO videos (id, owner_id, title, status) VALUES ($1,$2,'rollback test','UPLOADING')`, videoID, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE event_key IN ($1,$2)`, videoID+":UPLOAD:v1", videoID+":SUBTITLE:vi:v1")
		_, _ = db.ExecContext(ctx, `DELETE FROM subtitle_requests WHERE video_id=$1`, videoID)
		_, _ = db.ExecContext(ctx, `DELETE FROM video_uploads WHERE id=$1`, uploadID)
		_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id=$1`, videoID)
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO video_uploads (id,video_id,object_key,status,expected_content_type,expected_size_bytes,expires_at,generate_subtitles) VALUES ($1,$2,'source','UPLOADING','video/mp4',1024,$3,true)`, uploadID, videoID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// A conflicting outbox key forces the subtitle event insert to fail.
	if _, err := db.ExecContext(ctx, `INSERT INTO outbox_events (id,event_key,exchange_name,routing_key,payload) VALUES ($1,$2,'video.events','subtitle.requested.v1','{}')`, uuid.NewString(), videoID+":SUBTITLE:vi:v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepository(db).CompleteOwned(ctx, ownerID, uploadID, 1024, now); err == nil {
		t.Fatal("expected outbox insert failure")
	}
	var videoStatus, uploadStatus string
	var requestCount, uploadEvents int
	if err := db.QueryRowContext(ctx, `SELECT status FROM videos WHERE id=$1`, videoID).Scan(&videoStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM video_uploads WHERE id=$1`, uploadID).Scan(&uploadStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM subtitle_requests WHERE video_id=$1`, videoID).Scan(&requestCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE event_key=$1`, videoID+":UPLOAD:v1").Scan(&uploadEvents); err != nil {
		t.Fatal(err)
	}
	if videoStatus != "UPLOADING" || uploadStatus != "UPLOADING" || requestCount != 0 || uploadEvents != 0 {
		t.Fatalf("rollback failed: video=%s upload=%s requests=%d video_events=%d", videoStatus, uploadStatus, requestCount, uploadEvents)
	}
}
