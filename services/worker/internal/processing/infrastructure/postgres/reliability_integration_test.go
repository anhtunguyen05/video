package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

func TestReliableJobLifecycleAgainstPostgres(t *testing.T) {
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
	videoID, ownerID := uuid.NewString(), uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO videos (id, owner_id, title, status, source_object_key)
		VALUES ($1, $2, 'reliability test', 'UPLOADED', 'source.mp4')`, videoID, ownerID); err != nil {
		t.Fatal(err)
	}
	jobID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM outbox_events WHERE event_key IN ($1, $2)`, jobID+":dead", "invalid:test")
		_, _ = db.ExecContext(ctx, `DELETE FROM processing_dead_letters WHERE job_id = $1`, jobID)
		_, _ = db.ExecContext(ctx, `DELETE FROM processing_jobs WHERE video_id = $1`, videoID)
		_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id = $1`, videoID)
	})
	repository := NewRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	job, err := domain.NewQueuedJob(domain.VideoUploaded{VideoID: videoID, OwnerID: ownerID,
		SourceObjectKey: "source.mp4", ProcessingVersion: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	job.ID = jobID
	job.OriginalMessage = []byte(`{"message_type":"video.uploaded.v1"}`)
	if err := repository.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_jobs WHERE video_id = $1`, videoID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate event created %d jobs: %v", count, err)
	}
	for attempt := 1; attempt <= 4; attempt++ {
		claimed, ok, err := repository.ClaimNext(ctx, now)
		if err != nil || !ok || claimed.Attempt != attempt || claimed.RunID == "" {
			t.Fatalf("attempt %d claim = %#v, %v, %v", attempt, claimed, ok, err)
		}
		if err := repository.RecordOperation(ctx, claimed, "METADATA", "default", "", now); err != nil {
			t.Fatal(err)
		}
		var operationKey, operationRun string
		if err := db.QueryRowContext(ctx, `SELECT operation_key, run_id::text FROM processing_operations
			WHERE job_id = $1`, jobID).Scan(&operationKey, &operationRun); err != nil {
			t.Fatal(err)
		}
		if operationKey != videoID+":METADATA:v1:default" || operationRun != claimed.RunID {
			t.Fatalf("operation identity/run = %s/%s", operationKey, operationRun)
		}
		if err := repository.RetryOrFail(ctx, claimed, "PROCESS_INTERRUPTED", "test interruption", now); err != nil {
			t.Fatal(err)
		}
		if ok, err := repository.RenewLease(ctx, claimed, now); err != nil || ok {
			t.Fatalf("stale run renewed lease: %v, %v", ok, err)
		}
		if attempt < 4 {
			if _, ok, err := repository.ClaimNext(ctx, now); err != nil || ok {
				t.Fatalf("retry claimed before delay: %v, %v", ok, err)
			}
			now = now.Add(retryDelay(attempt) + time.Second)
		}
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM videos WHERE id = $1`, videoID).Scan(&status); err != nil || status != "FAILED" {
		t.Fatalf("video status = %s, %v", status, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM processing_dead_letters WHERE job_id = $1`, jobID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dead letters = %d, %v", count, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM outbox_events WHERE event_key = $1`, jobID+":dead").Scan(&count); err != nil || count != 1 {
		t.Fatalf("dead outbox events = %d, %v", count, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processing_runs SET finished_at = $2 WHERE job_id = $1`,
		jobID, now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	runsForVideo := func(runs []ports.RunCleanup) []ports.RunCleanup {
		filtered := make([]ports.RunCleanup, 0)
		for _, run := range runs {
			if strings.Contains(run.Prefix, "/videos/"+videoID+"/") {
				filtered = append(filtered, run)
			}
		}
		return filtered
	}
	if runs, err := repository.CleanupCandidates(ctx, now.Add(-26*time.Hour)); err != nil || len(runsForVideo(runs)) != 0 {
		t.Fatalf("recent runs selected for cleanup: %d, %v", len(runsForVideo(runs)), err)
	}
	runs, err := repository.CleanupCandidates(ctx, now)
	runs = runsForVideo(runs)
	if err != nil || len(runs) != 4 {
		t.Fatalf("aged failed runs = %d, %v", len(runs), err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO assets
		(id, video_id, asset_type, variant, object_key, content_type, size_bytes)
		VALUES ($1, $2, 'THUMBNAIL', 'default', $3, 'image/jpeg', 1)`,
		uuid.NewString(), videoID, runs[0].Prefix+"referenced.jpg"); err != nil {
		t.Fatal(err)
	}
	runsWithReference, err := repository.CleanupCandidates(ctx, now)
	runsWithReference = runsForVideo(runsWithReference)
	if err != nil || len(runsWithReference) != 3 {
		t.Fatalf("referenced run selected for cleanup: %d, %v", len(runsWithReference), err)
	}
}

func TestFinalStateRequiresCurrentRunAndVersion(t *testing.T) {
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
	videoID, ownerID := uuid.NewString(), uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO videos (id, owner_id, title, status, source_object_key)
		VALUES ($1, $2, 'final state test', 'UPLOADED', 'source.mp4')`, videoID, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processing_jobs WHERE video_id = $1`, videoID)
		_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id = $1`, videoID)
	})
	repository := NewRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	job, err := domain.NewQueuedJob(domain.VideoUploaded{VideoID: videoID, OwnerID: ownerID,
		SourceObjectKey: "source.mp4", ProcessingVersion: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repository.ClaimNext(ctx, now)
	if err != nil || !ok {
		t.Fatalf("claim: %v, %v", ok, err)
	}
	metadata := ports.VideoMetadata{DurationMS: 6000, Width: 640, Height: 360, Codec: "h264", Container: "mp4"}
	if err := repository.PersistMetadata(ctx, claimed, metadata, 1024, now); err != nil {
		t.Fatal(err)
	}
	asset := func(kind, variant, suffix, contentType string) ports.GeneratedAsset {
		return ports.GeneratedAsset{ID: uuid.NewString(), VideoID: videoID, AssetType: kind,
			Variant: variant, ObjectKey: runObjectPrefix(claimed) + suffix, ContentType: contentType, SizeBytes: 100}
	}
	rendition := domain.Rendition{Name: "360p", Width: 640, Height: 360, Codec: "h264"}
	if err := repository.PrepareTranscoding(ctx, claimed,
		asset("THUMBNAIL", "default", "thumbnails/default.jpg", "image/jpeg"), []domain.Rendition{rendition}, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.BeginRendition(ctx, claimed, rendition, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteRendition(ctx, claimed,
		asset("RENDITION", "360p", "renditions/360p/video.mp4", "video/mp4"), rendition, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.BeginPackaging(ctx, claimed, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteHLS(ctx, claimed,
		asset("HLS_MASTER", "default", "hls/master.m3u8", "application/vnd.apple.mpegurl"),
		[]ports.GeneratedAsset{asset("HLS_VARIANT", "360p", "hls/360p/index.m3u8", "application/vnd.apple.mpegurl")}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE videos SET processing_version = 2 WHERE id = $1`, videoID); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkSucceeded(ctx, claimed, now); err == nil {
		t.Fatal("stale version reached READY")
	}
	if _, err := db.ExecContext(ctx, `UPDATE videos SET processing_version = 1 WHERE id = $1`, videoID); err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkSucceeded(ctx, claimed, now); err != nil {
		t.Fatal(err)
	}
	var videoStatus, jobStatus string
	if err := db.QueryRowContext(ctx, `SELECT v.status, j.status FROM videos v JOIN processing_jobs j ON j.video_id = v.id
		WHERE v.id = $1`, videoID).Scan(&videoStatus, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if videoStatus != "READY" || jobStatus != "SUCCEEDED" {
		t.Fatalf("final state = %s/%s", videoStatus, jobStatus)
	}
}

func TestExpiredLeaseAndStaleVersionDoNotLeaveQueuedJob(t *testing.T) {
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
	videoID, ownerID := uuid.NewString(), uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO videos (id, owner_id, title, status, source_object_key)
		VALUES ($1, $2, 'lease test', 'UPLOADED', 'source.mp4')`, videoID, ownerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM processing_jobs WHERE video_id = $1`, videoID)
		_, _ = db.ExecContext(ctx, `DELETE FROM videos WHERE id = $1`, videoID)
	})
	repository := NewRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	job, err := domain.NewQueuedJob(domain.VideoUploaded{VideoID: videoID, OwnerID: ownerID,
		SourceObjectKey: "source.mp4", ProcessingVersion: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateQueued(ctx, job); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repository.ClaimNext(ctx, now)
	if err != nil || !ok {
		t.Fatalf("claim: %v, %v", ok, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE processing_jobs SET lease_until = $2 WHERE id = $1`, claimed.ID, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverExpired(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordOperation(ctx, claimed, "METADATA", "", "", now); err == nil {
		t.Fatal("expired run changed operation state")
	}
	if _, err := db.ExecContext(ctx, `UPDATE videos SET processing_version = 2 WHERE id = $1`, videoID); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverExpired(ctx, now); err != nil {
		t.Fatal(err)
	}
	var status, errorCode string
	if err := db.QueryRowContext(ctx, `SELECT status, error_code FROM processing_jobs WHERE id = $1`, claimed.ID).Scan(&status, &errorCode); err != nil {
		t.Fatal(err)
	}
	if status != "FAILED" || errorCode != "STALE_PROCESSING_VERSION" {
		t.Fatalf("stale job = %s/%s", status, errorCode)
	}
}
