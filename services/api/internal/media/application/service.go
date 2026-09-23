package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"video/services/api/internal/media/domain"
	"video/services/api/internal/media/ports"
	"video/services/api/internal/platform/ids"
)

var (
	ErrNotFound      = errors.New("video not found")
	ErrInvalidCursor = errors.New("invalid cursor")
)

type Clock interface {
	Now() time.Time
}

type Service struct {
	repository ports.VideoRepository
	clock      Clock
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func NewService(repository ports.VideoRepository) *Service {
	return &Service{repository: repository, clock: realClock{}}
}

func NewServiceWithClock(repository ports.VideoRepository, clock Clock) *Service {
	return &Service{repository: repository, clock: clock}
}

func (service *Service) Create(ctx context.Context, ownerID, title string, originalFilename *string) (domain.Video, error) {
	id, err := ids.NewUUID()
	if err != nil {
		return domain.Video{}, fmt.Errorf("create video id: %w", err)
	}
	video, err := domain.NewVideo(id, ownerID, title, originalFilename, service.clock.Now())
	if err != nil {
		return domain.Video{}, err
	}
	if err := service.repository.Create(ctx, video); err != nil {
		return domain.Video{}, fmt.Errorf("persist video: %w", err)
	}
	return video, nil
}

func (service *Service) Get(ctx context.Context, ownerID, videoID string) (domain.Video, error) {
	video, err := service.repository.GetOwned(ctx, ownerID, videoID)
	if errors.Is(err, ports.ErrNotFound) {
		return domain.Video{}, ErrNotFound
	}
	if err != nil {
		return domain.Video{}, fmt.Errorf("get video: %w", err)
	}
	return video, nil
}

func (service *Service) List(ctx context.Context, ownerID string, limit int, encodedCursor string) (ports.VideoPage, string, error) {
	if limit < 1 || limit > 100 {
		return ports.VideoPage{}, "", errors.New("limit must be between 1 and 100")
	}
	cursor, err := decodeCursor(encodedCursor)
	if err != nil {
		return ports.VideoPage{}, "", err
	}
	page, err := service.repository.ListOwned(ctx, ownerID, limit, cursor)
	if err != nil {
		return ports.VideoPage{}, "", fmt.Errorf("list videos: %w", err)
	}
	if page.NextCursor == nil {
		return page, "", nil
	}
	nextCursor, err := encodeCursor(*page.NextCursor)
	if err != nil {
		return ports.VideoPage{}, "", fmt.Errorf("encode cursor: %w", err)
	}
	return page, nextCursor, nil
}

func (service *Service) Delete(ctx context.Context, ownerID, videoID string) error {
	video, err := service.Get(ctx, ownerID, videoID)
	if err != nil {
		return err
	}
	if err := video.Delete(service.clock.Now()); err != nil {
		return err
	}
	if err := service.repository.DeleteOwned(ctx, video); err != nil {
		return fmt.Errorf("delete video: %w", err)
	}
	return nil
}

type cursorPayload struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeCursor(cursor ports.VideoCursor) (string, error) {
	payload, err := json.Marshal(cursorPayload{CreatedAt: cursor.CreatedAt, ID: cursor.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(encoded string) (*ports.VideoCursor, error) {
	if strings.TrimSpace(encoded) == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var decoded cursorPayload
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.ID == "" || decoded.CreatedAt.IsZero() {
		return nil, ErrInvalidCursor
	}
	return &ports.VideoCursor{CreatedAt: decoded.CreatedAt, ID: decoded.ID}, nil
}
