package ports

import (
	"context"
	"errors"
	"time"

	"video/services/api/internal/media/domain"
)

var ErrNotFound = errors.New("video not found")

type ThumbnailURLSigner interface {
	PresignThumbnail(context.Context, string, time.Duration) (string, time.Time, error)
}

type VideoCursor struct {
	CreatedAt time.Time
	ID        string
}

type VideoPage struct {
	Items      []domain.Video
	NextCursor *VideoCursor
}

type VideoRepository interface {
	Create(context.Context, domain.Video) error
	GetOwned(context.Context, string, string) (domain.Video, error)
	ListOwned(context.Context, string, int, *VideoCursor) (VideoPage, error)
	DeleteOwned(context.Context, domain.Video) error
}
