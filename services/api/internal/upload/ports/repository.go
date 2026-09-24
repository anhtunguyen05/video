package ports

import (
	"context"
	"errors"
	"time"

	"video/services/api/internal/upload/domain"
)

var (
	ErrNotFound = errors.New("upload not found")
	ErrConflict = errors.New("upload conflict")
)

type Repository interface {
	Create(context.Context, string, domain.Session) error
	GetOwned(context.Context, string, string) (domain.Session, error)
	CompleteOwned(context.Context, string, string, int64, time.Time) (domain.Session, error)
	AbortOwned(context.Context, string, string, time.Time) error
	ExpireOwned(context.Context, string, string, time.Time) error
}
