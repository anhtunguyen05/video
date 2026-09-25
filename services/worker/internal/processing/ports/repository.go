package ports

import (
	"context"

	"video/services/worker/internal/processing/domain"
)

type Repository interface {
	CreateQueued(context.Context, domain.Job) error
}
