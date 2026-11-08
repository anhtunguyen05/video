package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"video/services/api/internal/platform/observability"
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
			expected_size_bytes, expires_at, created_at, updated_at, generate_subtitles
		)
		SELECT $3, id, $4, $5, $7, $8, $9, $10, $6, $11
		FROM updated_video
	`, session.VideoID, ownerID, session.ID, session.ObjectKey, session.Status, session.UpdatedAt,
		session.ExpectedContentType, session.ExpectedSizeBytes, session.ExpiresAt, session.CreatedAt, session.GenerateSubtitles)
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
			RETURNING v.id, v.processing_version
		), completed_upload AS (
			UPDATE video_uploads u
			SET status = 'COMPLETED', completed_size_bytes = $3,
				completed_at = $4, updated_at = $4
			FROM updated_video v
			WHERE u.id = $1 AND u.video_id = v.id
			RETURNING u.id, u.video_id, u.object_key, u.status, v.processing_version,
			u.expected_content_type, u.expected_size_bytes,
			u.completed_size_bytes, u.expires_at, u.completed_at,
			u.created_at, u.updated_at, u.generate_subtitles
		), event_identity AS (
			SELECT gen_random_uuid() AS event_id, * FROM completed_upload
		), inserted_event AS (
			INSERT INTO outbox_events (id, event_key, exchange_name, routing_key, payload)
			SELECT e.event_id, e.video_id::text || ':UPLOAD:v' || e.processing_version,
				'video.events', 'video.uploaded.v1',
				jsonb_build_object(
					'message_id', e.event_id, 'message_type', 'video.uploaded.v1',
					'schema_version', 1, 'occurred_at', $4,
					'correlation_id', COALESCE(NULLIF($5, ''), e.video_id::text), 'causation_id', e.id::text,
                    'traceparent', $6::text,
					'payload', jsonb_build_object('video_id', e.video_id,
						'owner_id', $2::uuid, 'source_object_key', e.object_key,
						'processing_version', e.processing_version))
			FROM event_identity e
			ON CONFLICT (event_key) DO NOTHING
			RETURNING id
		), subtitle_request AS (
			INSERT INTO subtitle_requests (id, video_id, owner_id, source_version, language, status, created_at, updated_at)
			SELECT gen_random_uuid(), e.video_id, $2::uuid, e.processing_version, 'vi', 'PENDING', $4, $4
			FROM event_identity e WHERE e.generate_subtitles
			ON CONFLICT (video_id, source_version, language) DO NOTHING
			RETURNING id, video_id, source_version
		), subtitle_event AS (
			INSERT INTO outbox_events (id, event_key, exchange_name, routing_key, payload)
			SELECT s.id, s.video_id::text || ':SUBTITLE:vi:v' || s.source_version,
				'video.events', 'subtitle.requested.v1', jsonb_build_object(
					'message_id', s.id, 'message_type', 'subtitle.requested.v1',
					'schema_version', 1, 'occurred_at', $4,
					'correlation_id', COALESCE(NULLIF($5, ''), s.video_id::text),
					'causation_id', e.id::text, 'traceparent', $6::text,
					'payload', jsonb_build_object('job_id', s.id, 'video_id', s.video_id,
						'owner_id', $2::uuid, 'source_object_key', e.object_key,
						'source_version', s.source_version, 'language', 'vi'))
			FROM subtitle_request s JOIN event_identity e ON e.video_id = s.video_id
			RETURNING id
		)
		SELECT e.id, e.video_id, e.object_key, e.status,
			e.expected_content_type, e.expected_size_bytes,
			e.completed_size_bytes, e.expires_at, e.completed_at,
			e.created_at, e.updated_at, e.generate_subtitles
		FROM event_identity e
	`, uploadID, ownerID, sizeBytes, now, observability.RequestID(ctx), observability.Traceparent(ctx))
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
	       u.created_at, u.updated_at, u.generate_subtitles
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
		&session.GenerateSubtitles,
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
