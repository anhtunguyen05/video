package postgres

import (
	"context"
	"database/sql"

	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

type DB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
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

var _ ports.Repository = (*Repository)(nil)
