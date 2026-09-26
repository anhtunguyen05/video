package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

var ErrInvalidMessage = errors.New("invalid video uploaded message")

const (
	FailureSourceNotFound   = "SOURCE_NOT_FOUND"
	FailureSourceUnreadable = "SOURCE_UNREADABLE"
	FailureInvalidMedia     = "INVALID_MEDIA_STREAM"
	FailureThumbnail        = "THUMBNAIL_GENERATION_FAILED"
)

const DefaultThumbnailMaxEdge = 640

type Clock interface {
	Now() time.Time
}

type Service struct {
	repository ports.Repository
	processor  ports.ProcessorRepository
	source     ports.SourceStorage
	assets     ports.AssetStorage
	probe      ports.MetadataProbe
	thumbnail  ports.ThumbnailGenerator
	tempDir    string
	clock      Clock
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type eventEnvelope struct {
	MessageType   string `json:"message_type"`
	SchemaVersion int    `json:"schema_version"`
	Payload       struct {
		VideoID           string `json:"video_id"`
		OwnerID           string `json:"owner_id"`
		SourceObjectKey   string `json:"source_object_key"`
		ProcessingVersion int    `json:"processing_version"`
	} `json:"payload"`
}

func NewService(repository ports.Repository) *Service {
	return &Service{repository: repository, clock: realClock{}}
}

func NewServiceWithClock(repository ports.Repository, clock Clock) *Service {
	return &Service{repository: repository, clock: clock}
}

func NewProcessingService(repository ports.Repository, processor ports.ProcessorRepository, source ports.SourceStorage, assets ports.AssetStorage, probe ports.MetadataProbe, thumbnail ports.ThumbnailGenerator, tempDir string) *Service {
	return NewProcessingServiceWithClock(repository, processor, source, assets, probe, thumbnail, tempDir, realClock{})
}

func NewProcessingServiceWithClock(repository ports.Repository, processor ports.ProcessorRepository, source ports.SourceStorage, assets ports.AssetStorage, probe ports.MetadataProbe, thumbnail ports.ThumbnailGenerator, tempDir string, clock Clock) *Service {
	return &Service{
		repository: repository,
		processor:  processor,
		source:     source,
		assets:     assets,
		probe:      probe,
		thumbnail:  thumbnail,
		tempDir:    tempDir,
		clock:      clock,
	}
}

func (service *Service) HandleVideoUploaded(ctx context.Context, body []byte) error {
	var event eventEnvelope
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("%w: decode JSON: %v", ErrInvalidMessage, err)
	}
	if event.MessageType != "video.uploaded.v1" || event.SchemaVersion != 1 {
		return fmt.Errorf("%w: unexpected message type", ErrInvalidMessage)
	}
	job, err := domain.NewQueuedJob(domain.VideoUploaded{
		VideoID:           event.Payload.VideoID,
		OwnerID:           event.Payload.OwnerID,
		SourceObjectKey:   event.Payload.SourceObjectKey,
		ProcessingVersion: event.Payload.ProcessingVersion,
	}, service.clock.Now())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	if err := service.repository.CreateQueued(ctx, job); err != nil {
		return fmt.Errorf("create queued processing job: %w", err)
	}
	if service.processor == nil {
		return nil
	}
	claimedJob, claimed, err := service.processor.ClaimQueued(ctx, job.OperationKey, service.clock.Now())
	if err != nil {
		return fmt.Errorf("claim queued processing job: %w", err)
	}
	if !claimed {
		return nil
	}
	return service.process(ctx, claimedJob)
}

func (service *Service) process(ctx context.Context, job domain.Job) error {
	if service.source == nil || service.assets == nil || service.probe == nil || service.thumbnail == nil {
		return errors.New("processing dependencies are not configured")
	}
	workDir, err := os.MkdirTemp(service.tempDir, "video-processing-")
	if err != nil {
		return fmt.Errorf("create processing temp directory: %w", err)
	}
	defer os.RemoveAll(workDir)

	sourcePath := filepath.Join(workDir, "source")
	sourceFile, err := os.Create(sourcePath)
	if err != nil {
		return fmt.Errorf("create processing source file: %w", err)
	}
	objectInfo, downloadErr := service.source.Download(ctx, job.SourceObjectKey, sourceFile)
	closeErr := sourceFile.Close()
	if downloadErr != nil {
		if errors.Is(downloadErr, ports.ErrObjectNotFound) {
			return service.markFailed(ctx, job, FailureSourceNotFound, "The uploaded source object could not be found.")
		}
		if errors.Is(downloadErr, ports.ErrObjectUnreadable) {
			return service.markFailed(ctx, job, FailureSourceUnreadable, "The uploaded source object could not be read.")
		}
		return fmt.Errorf("download source object: %w", downloadErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close processing source file: %w", closeErr)
	}

	metadata, err := service.probe.Probe(ctx, sourcePath)
	if err != nil {
		if errors.Is(err, ports.ErrInvalidMedia) {
			return service.markFailed(ctx, job, FailureInvalidMedia, "The uploaded media is invalid or missing a video stream.")
		}
		return fmt.Errorf("probe source object: %w", err)
	}
	if err := service.processor.PersistMetadata(ctx, job, metadata, objectInfo.Size, service.clock.Now()); err != nil {
		return fmt.Errorf("persist video metadata: %w", err)
	}

	thumbnailPath := filepath.Join(workDir, "thumbnail.jpg")
	if err := service.thumbnail.Generate(ctx, sourcePath, thumbnailPath, thumbnailTimestamp(metadata.DurationMS)); err != nil {
		return service.markFailed(ctx, job, FailureThumbnail, "The video thumbnail could not be generated.")
	}
	thumbnailInfo, err := os.Stat(thumbnailPath)
	if err != nil || thumbnailInfo.Size() <= 0 {
		return service.markFailed(ctx, job, FailureThumbnail, "The video thumbnail output was empty.")
	}
	thumbnailFile, err := os.Open(thumbnailPath)
	if err != nil {
		return service.markFailed(ctx, job, FailureThumbnail, "The video thumbnail output could not be opened.")
	}
	_, uploadErr := service.assets.Upload(ctx,
		thumbnailObjectKey(job), "image/jpeg", thumbnailFile, thumbnailInfo.Size())
	closeThumbnailErr := thumbnailFile.Close()
	if uploadErr != nil {
		return fmt.Errorf("upload thumbnail: %w", uploadErr)
	}
	if closeThumbnailErr != nil {
		return fmt.Errorf("close thumbnail file: %w", closeThumbnailErr)
	}
	asset := ports.GeneratedAsset{
		ID:          uuid.NewString(),
		VideoID:     job.VideoID,
		AssetType:   "THUMBNAIL",
		Variant:     "default",
		ObjectKey:   thumbnailObjectKey(job),
		ContentType: "image/jpeg",
		SizeBytes:   thumbnailInfo.Size(),
	}
	if err := service.processor.MarkSucceeded(ctx, job, asset, service.clock.Now()); err != nil {
		return fmt.Errorf("mark processing job succeeded: %w", err)
	}
	return nil
}

func thumbnailTimestamp(durationMS int64) time.Duration {
	if durationMS <= 0 {
		return 0
	}
	duration := time.Duration(durationMS) * time.Millisecond
	if duration <= time.Second {
		return 0
	}
	timestamp := duration / 10
	latestSafe := duration - 100*time.Millisecond
	if timestamp > latestSafe {
		return latestSafe
	}
	return timestamp
}

func thumbnailObjectKey(job domain.Job) string {
	return fmt.Sprintf("users/%s/videos/%s/thumbnails/default.jpg", job.OwnerID, job.VideoID)
}

func (service *Service) markFailed(ctx context.Context, job domain.Job, code, message string) error {
	if err := service.processor.MarkFailed(ctx, job, code, message, service.clock.Now()); err != nil {
		return fmt.Errorf("mark processing job failed: %w", err)
	}
	return nil
}
