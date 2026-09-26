package ports

import (
	"context"
	"time"

	"video/services/worker/internal/processing/domain"
)

type Repository interface {
	CreateQueued(context.Context, domain.Job) error
}

type ProcessorRepository interface {
	ClaimQueued(context.Context, string, time.Time) (domain.Job, bool, error)
	PersistMetadata(context.Context, domain.Job, VideoMetadata, int64, time.Time) error
	MarkSucceeded(context.Context, domain.Job, GeneratedAsset, []domain.Rendition, time.Time) error
	MarkFailed(context.Context, domain.Job, string, string, time.Time) error
}
