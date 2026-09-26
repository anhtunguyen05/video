package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

type DB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Repository struct {
	db DB
}

func NewRepository(db DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) CreateQueued(ctx context.Context, job domain.Job) error {
	_, err := repository.db.ExecContext(ctx, `
		INSERT INTO processing_jobs (
			id, video_id, owner_id, source_object_key, processing_version,
			operation_key, status, attempt, max_attempts, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (operation_key) DO NOTHING
	`, job.ID, job.VideoID, job.OwnerID, job.SourceObjectKey, job.ProcessingVersion,
		job.OperationKey, job.Status, job.Attempt, job.MaxAttempts, job.CreatedAt, job.UpdatedAt)
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
			RETURNING id
		)
		UPDATE processing_jobs
		SET stage = 'THUMBNAIL', error_code = NULL, error_message = NULL, updated_at = $10
		WHERE id = $1 AND status = 'RUNNING' AND EXISTS (SELECT 1 FROM updated_video)
	`, job.ID, job.VideoID, sizeBytes, metadata.Container, metadata.Codec,
		metadata.DurationMS, metadata.Width, metadata.Height, metadata.FrameRate, now)
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

func (repository *Repository) MarkSucceeded(ctx context.Context, job domain.Job, asset ports.GeneratedAsset, renditions []domain.Rendition, now time.Time) error {
	plannedRenditions, err := json.Marshal(newPlannedRenditions(renditions))
	if err != nil {
		return err
	}
	result, err := repository.db.ExecContext(ctx, `
		WITH persisted_asset AS (
			INSERT INTO assets (
				id, video_id, asset_type, variant, object_key,
				content_type, size_bytes, created_at
			) VALUES ($2, $3, $4, $5, $6, $7, $8, $9)
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
			ON CONFLICT (video_id, name) DO UPDATE
			SET width = EXCLUDED.width,
			    height = EXCLUDED.height,
			    codec = EXCLUDED.codec,
			    status = 'PLANNED',
			    object_prefix = NULL,
			    updated_at = EXCLUDED.updated_at
			RETURNING video_id
		), updated_video AS (
			UPDATE videos
			SET status = 'READY', failure_code = NULL, failure_message = NULL,
			    updated_at = $9
			WHERE id = $3 AND status = 'PROCESSING'
			RETURNING id
		)
		UPDATE processing_jobs
		SET status = 'SUCCEEDED', stage = 'RENDITION_PLANNING', finished_at = $9,
		    error_code = NULL, error_message = NULL, updated_at = $9
		WHERE id = $1 AND status = 'RUNNING'
		  AND EXISTS (SELECT 1 FROM persisted_asset)
		  AND EXISTS (SELECT 1 FROM updated_video)
	`, job.ID, asset.ID, asset.VideoID, asset.AssetType, asset.Variant,
		asset.ObjectKey, asset.ContentType, asset.SizeBytes, now, string(plannedRenditions))
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

func (repository *Repository) MarkFailed(ctx context.Context, job domain.Job, code, message string, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH failed_video AS (
			UPDATE videos
			SET status = 'FAILED', failure_code = $2, failure_message = $3, updated_at = $4
			WHERE id = $5 AND status IN ('UPLOADED', 'QUEUED', 'PROCESSING')
			RETURNING id
		)
		UPDATE processing_jobs
		SET status = 'FAILED', error_code = $2, error_message = $3,
		    finished_at = $4, updated_at = $4
		WHERE id = $1 AND EXISTS (SELECT 1 FROM failed_video)
	`, job.ID, code, message, now, job.VideoID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("processing job failure affected no rows")
	}
	return nil
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
