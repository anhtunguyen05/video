package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusUploading Status = "UPLOADING"
	StatusCompleted Status = "COMPLETED"
	StatusAborted   Status = "ABORTED"
	StatusExpired   Status = "EXPIRED"
)

var (
	ErrInvalidState  = errors.New("invalid upload state")
	ErrInvalidUpload = errors.New("invalid upload")
)

type Session struct {
	ID                  string
	VideoID             string
	ObjectKey           string
	Status              Status
	ExpectedContentType string
	ExpectedSizeBytes   int64
	CompletedSizeBytes  *int64
	ExpiresAt           time.Time
	CompletedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func NewSession(id, videoID, objectKey, contentType string, sizeBytes int64, expiresAt, now time.Time) (Session, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(videoID) == "" || strings.TrimSpace(objectKey) == "" {
		return Session{}, fmt.Errorf("%w: identifiers and object key are required", ErrInvalidUpload)
	}
	if strings.TrimSpace(contentType) == "" || sizeBytes <= 0 || expiresAt.IsZero() {
		return Session{}, fmt.Errorf("%w: content type, size, and expiry are required", ErrInvalidUpload)
	}
	return Session{
		ID:                  id,
		VideoID:             videoID,
		ObjectKey:           objectKey,
		Status:              StatusUploading,
		ExpectedContentType: contentType,
		ExpectedSizeBytes:   sizeBytes,
		ExpiresAt:           expiresAt,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

func (session Session) IsActive() bool {
	return session.Status == StatusPending || session.Status == StatusUploading
}

func (session *Session) Complete(now time.Time, sizeBytes int64) error {
	if session.Status == StatusCompleted {
		return nil
	}
	if !session.IsActive() || sizeBytes <= 0 {
		return ErrInvalidState
	}
	session.Status = StatusCompleted
	session.CompletedSizeBytes = &sizeBytes
	session.CompletedAt = &now
	session.UpdatedAt = now
	return nil
}

func (session *Session) Abort(now time.Time) error {
	if session.Status == StatusAborted || session.Status == StatusExpired {
		return nil
	}
	if !session.IsActive() {
		return ErrInvalidState
	}
	session.Status = StatusAborted
	session.UpdatedAt = now
	return nil
}

func (session *Session) Expire(now time.Time) error {
	if session.Status == StatusExpired {
		return nil
	}
	if !session.IsActive() {
		return ErrInvalidState
	}
	session.Status = StatusExpired
	session.UpdatedAt = now
	return nil
}
