package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

func (repository *Repository) MarkSucceeded(ctx context.Context, job domain.Job, metadata ports.VideoMetadata, sizeBytes int64, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH updated_video AS (
			UPDATE videos
			SET status = 'READY', source_size_bytes = $3,
			    source_container = $4, source_codec = $5,
			    duration_ms = $6, width = $7, height = $8,
			    frame_rate = $9, failure_code = NULL, failure_message = NULL,
			    updated_at = $10
			WHERE id = $2 AND status IN ('UPLOADED', 'QUEUED', 'PROCESSING')
			RETURNING id
		)
		UPDATE processing_jobs
		SET status = 'SUCCEEDED', stage = 'METADATA', finished_at = $10,
		    error_code = NULL, error_message = NULL, updated_at = $10
		WHERE id = $1 AND EXISTS (SELECT 1 FROM updated_video)
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
