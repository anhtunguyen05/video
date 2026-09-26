package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusCreated    Status = "CREATED"
	StatusUploading  Status = "UPLOADING"
	StatusUploaded   Status = "UPLOADED"
	StatusQueued     Status = "QUEUED"
	StatusProcessing Status = "PROCESSING"
	StatusReady      Status = "READY"
	StatusFailed     Status = "FAILED"
	StatusDeleted    Status = "DELETED"
)

var (
	ErrInvalidState = errors.New("invalid video state")
	ErrInvalidVideo = errors.New("invalid video")
)

type Video struct {
	ID                string
	OwnerID           string
	Title             string
	OriginalFilename  *string
	Status            Status
	SourceSizeBytes   *int64
	SourceContainer   *string
	SourceCodec       *string
	DurationMS        *int64
	Width             *int
	Height            *int
	FrameRate         *float64
	FailureCode       *string
	FailureMessage    *string
	ProcessingVersion int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

func NewVideo(id, ownerID, title string, originalFilename *string, now time.Time) (Video, error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 255 {
		return Video{}, fmt.Errorf("%w: title must contain between 1 and 255 characters", ErrInvalidVideo)
	}
	if originalFilename != nil && len([]rune(strings.TrimSpace(*originalFilename))) > 255 {
		return Video{}, fmt.Errorf("%w: original filename must contain at most 255 characters", ErrInvalidVideo)
	}
	return Video{
		ID:                id,
		OwnerID:           ownerID,
		Title:             title,
		OriginalFilename:  originalFilename,
		Status:            StatusCreated,
		ProcessingVersion: 1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (video Video) CanDelete() bool {
	return video.Status == StatusCreated && video.DeletedAt == nil
}

func (video *Video) Delete(now time.Time) error {
	if !video.CanDelete() {
		return ErrInvalidState
	}
	video.Status = StatusDeleted
	video.DeletedAt = &now
	video.UpdatedAt = now
	return nil
}
