package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

var ErrInvalidMessage = errors.New("invalid video uploaded message")

type Clock interface {
	Now() time.Time
}

type Service struct {
	repository ports.Repository
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
	return nil
}
