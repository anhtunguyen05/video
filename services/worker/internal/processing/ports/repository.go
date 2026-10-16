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
	PrepareTranscoding(context.Context, domain.Job, GeneratedAsset, []domain.Rendition, time.Time) error
	BeginRendition(context.Context, domain.Job, domain.Rendition, time.Time) error
	CompleteRendition(context.Context, domain.Job, GeneratedAsset, domain.Rendition, time.Time) error
	BeginPackaging(context.Context, domain.Job, time.Time) error
	CompleteHLS(context.Context, domain.Job, GeneratedAsset, []GeneratedAsset, time.Time) error
	MarkSucceeded(context.Context, domain.Job, time.Time) error
	MarkRenditionFailed(context.Context, domain.Job, domain.Rendition, string, string, time.Time) error
	MarkFailed(context.Context, domain.Job, string, string, time.Time) error
}
