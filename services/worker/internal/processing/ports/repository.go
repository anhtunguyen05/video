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

// ReliableRepository owns scheduling and fencing. The event consumer only
// enqueues; the runner claims due jobs from PostgreSQL.
type ReliableRepository interface {
	ClaimNext(context.Context, time.Time) (domain.Job, bool, error)
	RenewLease(context.Context, domain.Job, time.Time) (bool, error)
	RecoverExpired(context.Context, time.Time) error
	RetryOrFail(context.Context, domain.Job, string, string, time.Time) error
	RecordOperation(context.Context, domain.Job, string, string, string, time.Time) error
	Quarantine(context.Context, []byte, string, string, time.Time) error
	CleanupCandidates(context.Context, time.Time) ([]RunCleanup, error)
	MarkRunClean(context.Context, string, time.Time) error
}

type RunCleanup struct {
	ID     string
	Prefix string
}
