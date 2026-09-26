package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"video/services/api/internal/media/domain"
	"video/services/api/internal/media/ports"
	platformpostgres "video/services/api/internal/platform/postgres"
)

type Repository struct {
	db platformpostgres.DB
}

func NewRepository(db platformpostgres.DB) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) Create(ctx context.Context, video domain.Video) error {
	_, err := repository.db.ExecContext(ctx, `
		INSERT INTO videos (
			id, owner_id, title, original_filename, status, processing_version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, video.ID, video.OwnerID, video.Title, video.OriginalFilename, video.Status, video.ProcessingVersion, video.CreatedAt, video.UpdatedAt)
	return err
}

func (repository *Repository) GetOwned(ctx context.Context, ownerID, videoID string) (domain.Video, error) {
	row := repository.db.QueryRowContext(ctx, videoSelect+`
		WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
	`, videoID, ownerID)
	video, err := scanVideo(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Video{}, ports.ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("scan video: %w", err)
	}
	return video, nil
}

func (repository *Repository) ListOwned(ctx context.Context, ownerID string, limit int, cursor *ports.VideoCursor) (ports.VideoPage, error) {
	query := videoSelect + `
		WHERE owner_id = $1 AND deleted_at IS NULL
	`
	args := []any{ownerID}
	if cursor != nil {
		query += ` AND (created_at, id) < ($2, $3)`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit+1)

	rows, err := repository.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ports.VideoPage{}, err
	}
	defer rows.Close()

	items := make([]domain.Video, 0, limit)
	for rows.Next() {
		video, err := scanVideo(rows)
		if err != nil {
			return ports.VideoPage{}, fmt.Errorf("scan video list: %w", err)
		}
		items = append(items, video)
	}
	if err := rows.Err(); err != nil {
		return ports.VideoPage{}, err
	}

	var nextCursor *ports.VideoCursor
	if len(items) > limit {
		last := items[limit-1]
		items = items[:limit]
		nextCursor = &ports.VideoCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return ports.VideoPage{Items: items, NextCursor: nextCursor}, nil
}

func (repository *Repository) DeleteOwned(ctx context.Context, video domain.Video) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE videos
		SET status = $1, deleted_at = $2, updated_at = $2
		WHERE id = $3 AND owner_id = $4 AND status = $5 AND deleted_at IS NULL
	`, video.Status, video.DeletedAt, video.ID, video.OwnerID, domain.StatusCreated)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrInvalidState
	}
	return nil
}

const videoSelect = `
	SELECT id, owner_id, title, original_filename, status,
	       source_size_bytes, source_container, source_codec, duration_ms,
	       width, height, frame_rate, failure_code, failure_message,
	       processing_version, created_at, updated_at, deleted_at
	FROM videos
`

type rowScanner interface {
	Scan(...any) error
}

func scanVideo(scanner rowScanner) (domain.Video, error) {
	var video domain.Video
	var originalFilename sql.NullString
	var status string
	var sourceSizeBytes sql.NullInt64
	var sourceContainer sql.NullString
	var sourceCodec sql.NullString
	var durationMS sql.NullInt64
	var width sql.NullInt64
	var height sql.NullInt64
	var frameRate sql.NullFloat64
	var failureCode sql.NullString
	var failureMessage sql.NullString
	var deletedAt sql.NullTime
	if err := scanner.Scan(
		&video.ID,
		&video.OwnerID,
		&video.Title,
		&originalFilename,
		&status,
		&sourceSizeBytes,
		&sourceContainer,
		&sourceCodec,
		&durationMS,
		&width,
		&height,
		&frameRate,
		&failureCode,
		&failureMessage,
		&video.ProcessingVersion,
		&video.CreatedAt,
		&video.UpdatedAt,
		&deletedAt,
	); err != nil {
		return domain.Video{}, err
	}
	if originalFilename.Valid {
		video.OriginalFilename = &originalFilename.String
	}
	if deletedAt.Valid {
		value := deletedAt.Time
		video.DeletedAt = &value
	}
	video.Status = domain.Status(status)
	if sourceSizeBytes.Valid {
		value := sourceSizeBytes.Int64
		video.SourceSizeBytes = &value
	}
	if sourceContainer.Valid {
		value := sourceContainer.String
		video.SourceContainer = &value
	}
	if sourceCodec.Valid {
		value := sourceCodec.String
		video.SourceCodec = &value
	}
	if durationMS.Valid {
		value := durationMS.Int64
		video.DurationMS = &value
	}
	if width.Valid {
		value := int(width.Int64)
		video.Width = &value
	}
	if height.Valid {
		value := int(height.Int64)
		video.Height = &value
	}
	if frameRate.Valid {
		value := frameRate.Float64
		video.FrameRate = &value
	}
	if failureCode.Valid {
		value := failureCode.String
		video.FailureCode = &value
	}
	if failureMessage.Valid {
		value := failureMessage.String
		video.FailureMessage = &value
	}
	return video, nil
}

var _ ports.VideoRepository = (*Repository)(nil)
