package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	mediaapplication "video/services/api/internal/media/application"
	"video/services/api/internal/media/domain"
	"video/services/api/internal/platform/ids"
	uploaddomain "video/services/api/internal/upload/domain"
	"video/services/api/internal/upload/ports"
)

var (
	ErrVideoNotFound       = errors.New("video not found")
	ErrUploadNotFound      = errors.New("upload not found")
	ErrInvalidState        = errors.New("invalid upload state")
	ErrInvalidVideoState   = errors.New("invalid video state")
	ErrUploadAlreadyActive = errors.New("upload already active")
	ErrExpired             = errors.New("upload expired")
	ErrObjectMissing       = errors.New("uploaded object not found")
	ErrSizeMismatch        = errors.New("uploaded object size mismatch")
	ErrUnsupportedMedia    = errors.New("unsupported media type")
	ErrInvalidObject       = errors.New("uploaded object content is invalid")
	ErrUploadTooLarge      = errors.New("upload is too large")
	ErrInvalidUploadInput  = errors.New("invalid upload input")
)

type Clock interface {
	Now() time.Time
}

type VideoReader interface {
	Get(context.Context, string, string) (domain.Video, error)
}

type Policy struct {
	MaxSizeBytes int64
	AllowedTypes map[string]struct{}
	URLExpiry    time.Duration
}

type Service struct {
	repository ports.Repository
	videos     VideoReader
	storage    ports.ObjectStorage
	clock      Clock
	policy     Policy
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type CreateResult struct {
	Session   uploaddomain.Session
	Presigned ports.PresignedUpload
}

type CompleteResult struct {
	VideoID string
	Status  domain.Status
}

func NewService(repository ports.Repository, videos VideoReader, storage ports.ObjectStorage, policy Policy) *Service {
	return &Service{repository: repository, videos: videos, storage: storage, clock: realClock{}, policy: policy}
}

func NewServiceWithClock(repository ports.Repository, videos VideoReader, storage ports.ObjectStorage, policy Policy, clock Clock) *Service {
	return &Service{repository: repository, videos: videos, storage: storage, clock: clock, policy: policy}
}

func (service *Service) Create(ctx context.Context, ownerID, videoID, contentType string, sizeBytes int64) (CreateResult, error) {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if _, ok := service.policy.AllowedTypes[contentType]; !ok {
		return CreateResult{}, ErrUnsupportedMedia
	}
	if sizeBytes <= 0 {
		return CreateResult{}, ErrInvalidUploadInput
	}
	if service.policy.MaxSizeBytes > 0 && sizeBytes > service.policy.MaxSizeBytes {
		return CreateResult{}, ErrUploadTooLarge
	}

	video, err := service.videos.Get(ctx, ownerID, videoID)
	if errors.Is(err, mediaapplication.ErrNotFound) {
		return CreateResult{}, ErrVideoNotFound
	}
	if err != nil {
		return CreateResult{}, fmt.Errorf("get upload video: %w", err)
	}
	if video.Status != domain.StatusCreated {
		return CreateResult{}, ErrInvalidVideoState
	}

	uploadID, err := ids.NewUUID()
	if err != nil {
		return CreateResult{}, fmt.Errorf("create upload id: %w", err)
	}
	now := service.clock.Now()
	expiresAt := now.Add(service.policy.URLExpiry)
	objectKey := fmt.Sprintf("users/%s/videos/%s/source/source.%s", ownerID, videoID, extensionFor(contentType))
	presigned, err := service.storage.PresignPut(ctx, objectKey, contentType, sizeBytes, service.policy.URLExpiry)
	if err != nil {
		return CreateResult{}, fmt.Errorf("presign upload: %w", err)
	}
	if !presigned.ExpiresAt.IsZero() {
		expiresAt = presigned.ExpiresAt
	}
	session, err := uploaddomain.NewSession(uploadID, videoID, objectKey, contentType, sizeBytes, expiresAt, now)
	if err != nil {
		return CreateResult{}, err
	}
	if err := service.repository.Create(ctx, ownerID, session); err != nil {
		if errors.Is(err, ports.ErrConflict) {
			return CreateResult{}, ErrUploadAlreadyActive
		}
		if errors.Is(err, ports.ErrNotFound) {
			return CreateResult{}, ErrVideoNotFound
		}
		return CreateResult{}, fmt.Errorf("persist upload: %w", err)
	}
	return CreateResult{Session: session, Presigned: presigned}, nil
}

func (service *Service) Complete(ctx context.Context, ownerID, uploadID string) (CompleteResult, error) {
	session, err := service.repository.GetOwned(ctx, ownerID, uploadID)
	if errors.Is(err, ports.ErrNotFound) {
		return CompleteResult{}, ErrUploadNotFound
	}
	if err != nil {
		return CompleteResult{}, fmt.Errorf("get upload: %w", err)
	}
	if session.Status == uploaddomain.StatusCompleted {
		return CompleteResult{VideoID: session.VideoID, Status: domain.StatusUploaded}, nil
	}
	if !session.IsActive() {
		return CompleteResult{}, ErrInvalidState
	}
	now := service.clock.Now()
	if !now.Before(session.ExpiresAt) {
		_ = service.repository.ExpireOwned(ctx, ownerID, uploadID, now)
		return CompleteResult{}, ErrExpired
	}
	object, err := service.storage.StatObject(ctx, session.ObjectKey)
	if errors.Is(err, ports.ErrObjectNotFound) {
		return CompleteResult{}, ErrObjectMissing
	}
	if err != nil {
		return CompleteResult{}, fmt.Errorf("stat uploaded object: %w", err)
	}
	if object.Size != session.ExpectedSizeBytes {
		return CompleteResult{}, ErrSizeMismatch
	}
	prefix, err := service.storage.ReadObjectPrefix(ctx, session.ObjectKey, 4096)
	if err != nil {
		return CompleteResult{}, fmt.Errorf("read uploaded object prefix: %w", err)
	}
	if !matchesMagic(session.ExpectedContentType, prefix) {
		return CompleteResult{}, ErrInvalidObject
	}
	completed, err := service.repository.CompleteOwned(ctx, ownerID, uploadID, object.Size, now)
	if errors.Is(err, ports.ErrNotFound) {
		current, getErr := service.repository.GetOwned(ctx, ownerID, uploadID)
		if getErr == nil && current.Status == uploaddomain.StatusCompleted {
			return CompleteResult{VideoID: current.VideoID, Status: domain.StatusUploaded}, nil
		}
		return CompleteResult{}, ErrInvalidState
	}
	if err != nil {
		return CompleteResult{}, fmt.Errorf("complete upload: %w", err)
	}
	return CompleteResult{VideoID: completed.VideoID, Status: domain.StatusUploaded}, nil
}

func (service *Service) Abort(ctx context.Context, ownerID, uploadID string) error {
	session, err := service.repository.GetOwned(ctx, ownerID, uploadID)
	if errors.Is(err, ports.ErrNotFound) {
		return ErrUploadNotFound
	}
	if err != nil {
		return fmt.Errorf("get upload: %w", err)
	}
	if session.Status == uploaddomain.StatusAborted || session.Status == uploaddomain.StatusExpired {
		return nil
	}
	if session.Status == uploaddomain.StatusCompleted {
		return ErrInvalidState
	}
	if err := service.storage.DeleteObject(ctx, session.ObjectKey); err != nil && !errors.Is(err, ports.ErrObjectNotFound) {
		return fmt.Errorf("delete aborted object: %w", err)
	}
	if err := service.repository.AbortOwned(ctx, ownerID, uploadID, service.clock.Now()); errors.Is(err, ports.ErrNotFound) {
		return ErrInvalidState
	} else if err != nil {
		return fmt.Errorf("abort upload: %w", err)
	}
	return nil
}

func extensionFor(contentType string) string {
	switch contentType {
	case "video/webm":
		return "webm"
	default:
		return "mp4"
	}
}

func matchesMagic(contentType string, prefix []byte) bool {
	switch contentType {
	case "video/mp4":
		return len(prefix) >= 8 && string(prefix[4:8]) == "ftyp"
	case "video/webm":
		return len(prefix) >= 4 && prefix[0] == 0x1a && prefix[1] == 0x45 && prefix[2] == 0xdf && prefix[3] == 0xa3
	default:
		return false
	}
}
