package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const StatusQueued Status = "QUEUED"

var ErrInvalidEvent = errors.New("invalid video uploaded event")

type VideoUploaded struct {
	VideoID           string
	OwnerID           string
	SourceObjectKey   string
	ProcessingVersion int
}

type Job struct {
	ID                string
	VideoID           string
	OwnerID           string
	SourceObjectKey   string
	ProcessingVersion int
	OperationKey      string
	Status            Status
	Attempt           int
	MaxAttempts       int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewQueuedJob(event VideoUploaded, now time.Time) (Job, error) {
	if strings.TrimSpace(event.VideoID) == "" || strings.TrimSpace(event.OwnerID) == "" || strings.TrimSpace(event.SourceObjectKey) == "" || event.ProcessingVersion <= 0 {
		return Job{}, ErrInvalidEvent
	}
	return Job{
		ID:                uuid.NewString(),
		VideoID:           event.VideoID,
		OwnerID:           event.OwnerID,
		SourceObjectKey:   event.SourceObjectKey,
		ProcessingVersion: event.ProcessingVersion,
		OperationKey:      fmt.Sprintf("%s:PROCESS:v%d", event.VideoID, event.ProcessingVersion),
		Status:            StatusQueued,
		MaxAttempts:       3,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}
