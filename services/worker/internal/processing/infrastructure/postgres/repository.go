package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

type DB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
}

type Repository struct {
	db DB
}

func NewRepository(db DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) CreateQueued(ctx context.Context, job domain.Job) error {
	_, err := repository.db.ExecContext(ctx, `
		WITH inserted_job AS (
		INSERT INTO processing_jobs (
			id, video_id, owner_id, source_object_key, processing_version,
			operation_key, status, attempt, max_attempts, original_message, created_at, updated_at
		) SELECT $1, v.id, $3, $4, $5, $6, $7, $8, $9, $12, $10, $11
		  FROM videos v WHERE v.id = $2 AND v.owner_id = $3
		    AND v.processing_version = $5 AND v.source_object_key = $4
		    AND v.deleted_at IS NULL AND v.status IN ('UPLOADED', 'QUEUED', 'PROCESSING')
		ON CONFLICT (operation_key) DO NOTHING
		RETURNING video_id
		)
		UPDATE videos v SET status = 'QUEUED', updated_at = $11
		WHERE v.id IN (SELECT video_id FROM inserted_job)
		  AND v.processing_version = $5 AND v.status = 'UPLOADED'
	`, job.ID, job.VideoID, job.OwnerID, job.SourceObjectKey, job.ProcessingVersion,
		job.OperationKey, job.Status, job.Attempt, job.MaxAttempts, job.CreatedAt, job.UpdatedAt, job.OriginalMessage)
	return err
}

func (repository *Repository) ClaimQueued(ctx context.Context, operationKey string, now time.Time) (domain.Job, bool, error) {
	row := repository.db.QueryRowContext(ctx, `
		UPDATE processing_jobs
		SET status = 'RUNNING', stage = 'VALIDATE_SOURCE',
		    attempt = attempt + 1, started_at = $2, updated_at = $2
		WHERE operation_key = $1 AND status = 'QUEUED' AND attempt < max_attempts
		RETURNING id, video_id, owner_id, source_object_key, processing_version,
		          operation_key, status, attempt, max_attempts, created_at, updated_at
	`, operationKey, now)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Job{}, false, nil
	}
	if err != nil {
		return domain.Job{}, false, err
	}
	return job, true, nil
}

func (repository *Repository) PersistMetadata(ctx context.Context, job domain.Job, metadata ports.VideoMetadata, sizeBytes int64, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH updated_video AS (
			UPDATE videos
			SET status = 'PROCESSING', source_size_bytes = $3,
			    source_container = $4, source_codec = $5,
			    duration_ms = $6, width = $7, height = $8,
			    frame_rate = $9, failure_code = NULL, failure_message = NULL,
			    updated_at = $10
			WHERE id = $2 AND status IN ('UPLOADED', 'QUEUED', 'PROCESSING')
			  AND processing_version = $11
			  AND EXISTS (SELECT 1 FROM processing_jobs WHERE id = $1 AND run_id = $12
			    AND status = 'RUNNING' AND lease_until > $10)
			RETURNING id
		)
		UPDATE processing_jobs
		SET stage = 'THUMBNAIL', error_code = NULL, error_message = NULL, updated_at = $10
		WHERE id = $1 AND run_id = $12 AND status = 'RUNNING'
		  AND EXISTS (SELECT 1 FROM updated_video)
	`, job.ID, job.VideoID, sizeBytes, metadata.Container, metadata.Codec,
		metadata.DurationMS, metadata.Width, metadata.Height, metadata.FrameRate, now,
		job.ProcessingVersion, job.RunID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("processing job completion affected no rows")
	}
	return nil
}

func (repository *Repository) PrepareTranscoding(ctx context.Context, job domain.Job, asset ports.GeneratedAsset, renditions []domain.Rendition, now time.Time) error {
	plannedRenditions, err := json.Marshal(newPlannedRenditions(renditions))
	if err != nil {
		return err
	}
	result, err := repository.db.ExecContext(ctx, `
		WITH eligible_job AS (
			SELECT j.id FROM processing_jobs j JOIN videos v ON v.id = j.video_id
			WHERE j.id = $1 AND j.run_id = $11 AND j.status = 'RUNNING'
			  AND j.lease_until > $9 AND v.processing_version = j.processing_version
		), persisted_asset AS (
			INSERT INTO assets (
				id, video_id, asset_type, variant, object_key,
				content_type, size_bytes, created_at
			) SELECT $2, $3, $4, $5, $6, $7, $8, $9
			  WHERE EXISTS (SELECT 1 FROM eligible_job)
			ON CONFLICT (video_id, asset_type, variant) DO UPDATE
			SET object_key = EXCLUDED.object_key,
			    content_type = EXCLUDED.content_type,
			    size_bytes = EXCLUDED.size_bytes
			RETURNING video_id
		), persisted_renditions AS (
			INSERT INTO renditions (
				id, video_id, name, width, height, codec, status, created_at, updated_at
			)
			SELECT planned.id::uuid, $3, planned.name, planned.width, planned.height,
				planned.codec, 'PLANNED', $9, $9
			FROM jsonb_to_recordset($10::jsonb)
			     AS planned(id text, name text, width integer, height integer, codec text)
			WHERE EXISTS (SELECT 1 FROM eligible_job)
			ON CONFLICT (video_id, name) DO UPDATE
			SET width = EXCLUDED.width,
			    height = EXCLUDED.height,
			    codec = EXCLUDED.codec,
				status = 'PLANNED',
				object_prefix = NULL,
				updated_at = EXCLUDED.updated_at
			RETURNING video_id
		)
		UPDATE processing_jobs
		SET stage = 'TRANSCODING', error_code = NULL, error_message = NULL, updated_at = $9
		WHERE id = $1 AND run_id = $11 AND status = 'RUNNING'
		  AND EXISTS (SELECT 1 FROM persisted_asset)
		  AND EXISTS (SELECT 1 FROM persisted_renditions)
	`, job.ID, asset.ID, asset.VideoID, asset.AssetType, asset.Variant,
		asset.ObjectKey, asset.ContentType, asset.SizeBytes, now, string(plannedRenditions), job.RunID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("processing job completion affected no rows")
	}
	return nil
}

func (repository *Repository) BeginRendition(ctx context.Context, job domain.Job, rendition domain.Rendition, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE renditions
		SET status = 'PROCESSING', updated_at = $4
		WHERE video_id = $1 AND name = $2 AND status = 'PLANNED'
		  AND EXISTS (SELECT 1 FROM processing_jobs j JOIN videos v ON v.id = j.video_id
		    WHERE j.id = $3 AND j.run_id = $5 AND j.status = 'RUNNING'
		      AND j.stage = 'TRANSCODING' AND j.lease_until > $4
		      AND v.processing_version = j.processing_version)
	`, job.VideoID, rendition.Name, job.ID, now, job.RunID)
	if err != nil {
		return err
	}
	return expectOneRow(result)
}

func (repository *Repository) CompleteRendition(ctx context.Context, job domain.Job, asset ports.GeneratedAsset, rendition domain.Rendition, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH eligible_job AS (
			SELECT j.id FROM processing_jobs j JOIN videos v ON v.id = j.video_id
			WHERE j.id = $9 AND j.run_id = $10 AND j.status = 'RUNNING'
			  AND j.stage = 'TRANSCODING' AND j.lease_until > $7
			  AND v.processing_version = j.processing_version
		), persisted_asset AS (
			INSERT INTO assets (id, video_id, asset_type, variant, object_key, content_type, size_bytes, created_at)
			SELECT $1, $2, 'RENDITION', $3, $4, $5, $6, $7
			WHERE EXISTS (SELECT 1 FROM eligible_job)
			ON CONFLICT (video_id, asset_type, variant) DO UPDATE
			SET object_key = EXCLUDED.object_key, content_type = EXCLUDED.content_type, size_bytes = EXCLUDED.size_bytes
			RETURNING video_id
		)
		UPDATE renditions
		SET status = 'READY', object_prefix = $8, updated_at = $7
		WHERE video_id = $2 AND name = $3 AND status = 'PROCESSING'
		  AND EXISTS (SELECT 1 FROM persisted_asset)
		  AND EXISTS (SELECT 1 FROM eligible_job)
	`, asset.ID, asset.VideoID, rendition.Name, asset.ObjectKey, asset.ContentType, asset.SizeBytes, now,
		renditionObjectPrefix(job, rendition), job.ID, job.RunID)
	if err != nil {
		return err
	}
	return expectOneRow(result)
}

func (repository *Repository) BeginPackaging(ctx context.Context, job domain.Job, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE processing_jobs
		SET stage = 'PACKAGING', updated_at = $3
		WHERE id = $1 AND status = 'RUNNING' AND stage = 'TRANSCODING'
		  AND run_id = $4 AND lease_until > $3
		  AND EXISTS (SELECT 1 FROM videos v WHERE v.id = $2
		    AND v.processing_version = processing_jobs.processing_version)
		  AND EXISTS (SELECT 1 FROM renditions WHERE video_id = $2)
		  AND NOT EXISTS (SELECT 1 FROM renditions WHERE video_id = $2 AND status <> 'READY')
	`, job.ID, job.VideoID, now, job.RunID)
	if err != nil {
		return err
	}
	return expectOneRow(result)
}

func (repository *Repository) CompleteHLS(ctx context.Context, job domain.Job, master ports.GeneratedAsset, variants []ports.GeneratedAsset, now time.Time) error {
	if len(variants) == 0 {
		return errors.New("HLS variants are required")
	}
	type hlsVariantRow struct {
		ID          string `json:"id"`
		Variant     string `json:"variant"`
		ObjectKey   string `json:"object_key"`
		ContentType string `json:"content_type"`
		SizeBytes   int64  `json:"size_bytes"`
	}
	variantRows := make([]hlsVariantRow, 0, len(variants))
	for _, asset := range variants {
		variantRows = append(variantRows, hlsVariantRow{ID: asset.ID, Variant: asset.Variant, ObjectKey: asset.ObjectKey, ContentType: asset.ContentType, SizeBytes: asset.SizeBytes})
	}
	encoded, err := json.Marshal(variantRows)
	if err != nil {
		return err
	}
	result, err := repository.db.ExecContext(ctx, `
		WITH eligible_job AS (
			SELECT j.id FROM processing_jobs j JOIN videos v ON v.id = j.video_id
			WHERE j.id = $1 AND j.run_id = $10 AND j.status = 'RUNNING'
			  AND j.stage = 'PACKAGING' AND j.lease_until > $7
			  AND v.processing_version = j.processing_version
		), persisted_master AS (
			INSERT INTO assets (id, video_id, asset_type, variant, object_key, content_type, size_bytes, created_at)
			SELECT $2, $3, 'HLS_MASTER', 'default', $4, $5, $6, $7
			WHERE EXISTS (SELECT 1 FROM eligible_job)
			ON CONFLICT (video_id, asset_type, variant) DO UPDATE
			SET object_key = EXCLUDED.object_key, content_type = EXCLUDED.content_type, size_bytes = EXCLUDED.size_bytes
			RETURNING video_id
		), persisted_variants AS (
			INSERT INTO assets (id, video_id, asset_type, variant, object_key, content_type, size_bytes, created_at)
			SELECT item.id::uuid, $3, 'HLS_VARIANT', item.variant, item.object_key, item.content_type, item.size_bytes, $7
			FROM jsonb_to_recordset($8::jsonb) AS item(id text, variant text, object_key text, content_type text, size_bytes bigint)
			WHERE EXISTS (SELECT 1 FROM eligible_job)
			ON CONFLICT (video_id, asset_type, variant) DO UPDATE
			SET object_key = EXCLUDED.object_key, content_type = EXCLUDED.content_type, size_bytes = EXCLUDED.size_bytes
			RETURNING video_id
		)
		UPDATE processing_jobs
		SET updated_at = $7
		WHERE id = $1 AND run_id = $10 AND status = 'RUNNING' AND stage = 'PACKAGING'
		  AND EXISTS (SELECT 1 FROM persisted_master)
		  AND (SELECT count(*) FROM persisted_variants) = $9
	`, job.ID, master.ID, job.VideoID, master.ObjectKey, master.ContentType, master.SizeBytes, now, string(encoded), len(variants), job.RunID)
	if err != nil {
		return err
	}
	return expectOneRow(result)
}

func (repository *Repository) MarkSucceeded(ctx context.Context, job domain.Job, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH ready_renditions AS (
			SELECT count(*) AS total, count(*) FILTER (WHERE status = 'READY') AS ready
			FROM renditions WHERE video_id = $2
		), updated_video AS (
			UPDATE videos
			SET status = 'READY', failure_code = NULL, failure_message = NULL, updated_at = $3
			WHERE id = $2 AND status = 'PROCESSING' AND processing_version = $5
			  AND EXISTS (SELECT 1 FROM processing_jobs WHERE id = $1 AND run_id = $4
			    AND status = 'RUNNING' AND stage = 'PACKAGING' AND lease_until > $3)
			  AND (SELECT total > 0 AND total = ready FROM ready_renditions)
			  AND EXISTS (SELECT 1 FROM assets WHERE video_id = $2 AND asset_type = 'THUMBNAIL'
			    AND object_key LIKE $6 || '%')
			  AND (SELECT count(*) FROM assets WHERE video_id = $2 AND asset_type = 'RENDITION'
			    AND object_key LIKE $6 || '%') = (SELECT ready FROM ready_renditions)
			  AND EXISTS (SELECT 1 FROM assets WHERE video_id = $2 AND asset_type = 'HLS_MASTER'
			    AND variant = 'default' AND object_key LIKE $6 || '%')
			  AND (SELECT count(*) FROM assets WHERE video_id = $2 AND asset_type = 'HLS_VARIANT'
			    AND object_key LIKE $6 || '%') = (SELECT ready FROM ready_renditions)
			RETURNING id
		), completed_job AS (
			UPDATE processing_jobs SET status = 'SUCCEEDED', finished_at = $3,
				lease_until = NULL, error_code = NULL, error_message = NULL, updated_at = $3
			WHERE id = $1 AND run_id = $4 AND status = 'RUNNING' AND stage = 'PACKAGING'
			  AND EXISTS (SELECT 1 FROM updated_video)
			RETURNING run_id
		)
		UPDATE processing_runs SET status = 'SUCCEEDED', finished_at = $3
		WHERE id = $4 AND status = 'RUNNING' AND EXISTS (SELECT 1 FROM completed_job)
	`, job.ID, job.VideoID, now, job.RunID, job.ProcessingVersion, runObjectPrefix(job))
	if err != nil {
		return err
	}
	return expectOneRow(result)
}

func (repository *Repository) MarkRenditionFailed(ctx context.Context, job domain.Job, rendition domain.Rendition, code, message string, now time.Time) error {
	return repository.finishTerminal(ctx, job, rendition.Name, code, message, now)
}

type plannedRendition struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Codec  string `json:"codec"`
}

func newPlannedRenditions(renditions []domain.Rendition) []plannedRendition {
	planned := make([]plannedRendition, 0, len(renditions))
	for _, rendition := range renditions {
		planned = append(planned, plannedRendition{
			ID:     uuid.NewString(),
			Name:   rendition.Name,
			Width:  rendition.Width,
			Height: rendition.Height,
			Codec:  rendition.Codec,
		})
	}
	return planned
}

func renditionObjectPrefix(job domain.Job, rendition domain.Rendition) string {
	if job.RunID != "" {
		return "users/" + job.OwnerID + "/videos/" + job.VideoID + "/processing/v" +
			fmt.Sprint(job.ProcessingVersion) + "/runs/" + job.RunID + "/renditions/" + rendition.Name
	}
	return "users/" + job.OwnerID + "/videos/" + job.VideoID + "/renditions/" + rendition.Name
}

func expectOneRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("processing job completion affected no rows")
	}
	return nil
}

func (repository *Repository) MarkFailed(ctx context.Context, job domain.Job, code, message string, now time.Time) error {
	return repository.finishTerminal(ctx, job, "", code, message, now)
}

func (repository *Repository) finishTerminal(ctx context.Context, job domain.Job, renditionName, code, message string, now time.Time) error {
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM processing_jobs j JOIN videos v ON v.id = j.video_id
		WHERE j.id = $1 AND j.run_id = $2 AND j.status = 'RUNNING' AND j.lease_until > $3
		  AND v.processing_version = j.processing_version FOR UPDATE OF j, v`, job.ID, job.RunID, now).Scan(&eligible)
	if err != nil {
		return err
	}
	if renditionName != "" {
		result, err := tx.ExecContext(ctx, `UPDATE renditions SET status = 'FAILED', updated_at = $3
			WHERE video_id = $1 AND name = $2 AND status = 'PROCESSING'`, job.VideoID, renditionName, now)
		if err != nil {
			return err
		}
		if err := expectOneRow(result); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE videos SET status = 'FAILED', failure_code = $2,
		failure_message = $3, updated_at = $4 WHERE id = $1 AND processing_version = $5
		AND status IN ('UPLOADED', 'QUEUED', 'PROCESSING')`, job.VideoID, code, message, now, job.ProcessingVersion)
	if err != nil {
		return err
	}
	if err := expectOneRow(result); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `UPDATE processing_jobs SET status = 'FAILED', lease_until = NULL,
		error_code = $2, error_message = $3, finished_at = $4, updated_at = $4
		WHERE id = $1 AND run_id = $5 AND status = 'RUNNING'`, job.ID, code, message, now, job.RunID)
	if err != nil {
		return err
	}
	if err := expectOneRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE video_id = $1 AND object_key LIKE $2 || '%'`,
		job.VideoID, runObjectPrefix(job)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE processing_operations SET status = 'FAILED', updated_at = $2
		WHERE run_id = $1 AND status = 'SUCCEEDED'`, job.RunID, now); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `UPDATE processing_runs SET status = 'FAILED', finished_at = $2
		WHERE id = $1 AND status = 'RUNNING'`, job.RunID, now)
	if err != nil {
		return err
	}
	if err := expectOneRow(result); err != nil {
		return err
	}
	return tx.Commit()
}

func runObjectPrefix(job domain.Job) string {
	return fmt.Sprintf("users/%s/videos/%s/processing/v%d/runs/%s/",
		job.OwnerID, job.VideoID, job.ProcessingVersion, job.RunID)
}

func scanJob(scanner rowScanner) (domain.Job, error) {
	var job domain.Job
	var status string
	if err := scanner.Scan(
		&job.ID, &job.VideoID, &job.OwnerID, &job.SourceObjectKey,
		&job.ProcessingVersion, &job.OperationKey, &status,
		&job.Attempt, &job.MaxAttempts, &job.CreatedAt, &job.UpdatedAt,
	); err != nil {
		return domain.Job{}, err
	}
	job.Status = domain.Status(status)
	return job, nil
}

type rowScanner interface {
	Scan(...any) error
}

var _ ports.Repository = (*Repository)(nil)
var _ ports.ProcessorRepository = (*Repository)(nil)
