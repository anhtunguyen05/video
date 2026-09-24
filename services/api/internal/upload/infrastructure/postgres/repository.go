package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	platformpostgres "video/services/api/internal/platform/postgres"
	uploaddomain "video/services/api/internal/upload/domain"
	"video/services/api/internal/upload/ports"
)

type Repository struct {
	db platformpostgres.DB
}

func NewRepository(db platformpostgres.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) Create(ctx context.Context, ownerID string, session uploaddomain.Session) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH updated_video AS (
			UPDATE videos
			SET status = 'UPLOADING', updated_at = $6
			WHERE id = $1 AND owner_id = $2 AND status = 'CREATED' AND deleted_at IS NULL
			RETURNING id
		)
		INSERT INTO video_uploads (
			id, video_id, object_key, status, expected_content_type,
			expected_size_bytes, expires_at, created_at, updated_at
		)
		SELECT $3, id, $4, $5, $7, $8, $9, $10, $6
		FROM updated_video
	`, session.VideoID, ownerID, session.ID, session.ObjectKey, session.Status, session.UpdatedAt,
		session.ExpectedContentType, session.ExpectedSizeBytes, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ports.ErrConflict
		}
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ports.ErrNotFound
	}
	return nil
}

func (repository *Repository) GetOwned(ctx context.Context, ownerID, uploadID string) (uploaddomain.Session, error) {
	row := repository.db.QueryRowContext(ctx, uploadSelect+`
		WHERE u.id = $1 AND v.owner_id = $2 AND v.deleted_at IS NULL
	`, uploadID, ownerID)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return uploaddomain.Session{}, ports.ErrNotFound
	}
	if err != nil {
		return uploaddomain.Session{}, fmt.Errorf("scan upload: %w", err)
	}
	return session, nil
}

func (repository *Repository) CompleteOwned(ctx context.Context, ownerID, uploadID string, sizeBytes int64, now time.Time) (uploaddomain.Session, error) {
	row := repository.db.QueryRowContext(ctx, `
		WITH updated_video AS (
			UPDATE videos v
			SET status = 'UPLOADED',
				source_object_key = u.object_key,
				source_size_bytes = $3,
				updated_at = $4
			FROM video_uploads u
			WHERE u.id = $1
			  AND u.video_id = v.id
			  AND v.owner_id = $2
			  AND v.status = 'UPLOADING'
			  AND u.status IN ('PENDING', 'UPLOADING')
			RETURNING v.id
		)
		UPDATE video_uploads u
		SET status = 'COMPLETED', completed_size_bytes = $3,
			completed_at = $4, updated_at = $4
		FROM updated_video v
		WHERE u.id = $1 AND u.video_id = v.id
		RETURNING u.id, u.video_id, u.object_key, u.status,
			u.expected_content_type, u.expected_size_bytes,
			u.completed_size_bytes, u.expires_at, u.completed_at,
			u.created_at, u.updated_at
	`, uploadID, ownerID, sizeBytes, now)
	session, err := scanSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return uploaddomain.Session{}, ports.ErrNotFound
	}
	if err != nil {
		return uploaddomain.Session{}, fmt.Errorf("complete upload: %w", err)
	}
	return session, nil
}

func (repository *Repository) AbortOwned(ctx context.Context, ownerID, uploadID string, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH aborted AS (
			UPDATE video_uploads u
			SET status = 'ABORTED', updated_at = $3
			FROM videos v
			WHERE u.id = $1 AND u.video_id = v.id AND v.owner_id = $2
			  AND u.status IN ('PENDING', 'UPLOADING')
			RETURNING u.video_id
		)
		UPDATE videos v
		SET status = 'CREATED', updated_at = $3
		FROM aborted a
		WHERE v.id = a.video_id AND v.owner_id = $2 AND v.status = 'UPLOADING'
	`, uploadID, ownerID, now)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (repository *Repository) ExpireOwned(ctx context.Context, ownerID, uploadID string, now time.Time) error {
	result, err := repository.db.ExecContext(ctx, `
		WITH expired AS (
			UPDATE video_uploads u
			SET status = 'EXPIRED', updated_at = $3
			FROM videos v
			WHERE u.id = $1 AND u.video_id = v.id AND v.owner_id = $2
			  AND u.status IN ('PENDING', 'UPLOADING')
			RETURNING u.video_id
		)
		UPDATE videos v
		SET status = 'CREATED', updated_at = $3
		FROM expired e
		WHERE v.id = e.video_id AND v.owner_id = $2 AND v.status = 'UPLOADING'
	`, uploadID, ownerID, now)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ports.ErrNotFound
	}
	return nil
}

const uploadSelect = `
	SELECT u.id, u.video_id, u.object_key, u.status,
	       u.expected_content_type, u.expected_size_bytes,
	       u.completed_size_bytes, u.expires_at, u.completed_at,
	       u.created_at, u.updated_at
	FROM video_uploads u
	JOIN videos v ON v.id = u.video_id
`

type rowScanner interface {
	Scan(...any) error
}

func scanSession(scanner rowScanner) (uploaddomain.Session, error) {
	var session uploaddomain.Session
	var status string
	var completedSize sql.NullInt64
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&session.ID,
		&session.VideoID,
		&session.ObjectKey,
		&status,
		&session.ExpectedContentType,
		&session.ExpectedSizeBytes,
		&completedSize,
		&session.ExpiresAt,
		&completedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		return uploaddomain.Session{}, err
	}
	if completedSize.Valid {
		value := completedSize.Int64
		session.CompletedSizeBytes = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		session.CompletedAt = &value
	}
	session.Status = uploaddomain.Status(status)
	return session, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint"))
}

var _ ports.Repository = (*Repository)(nil)
