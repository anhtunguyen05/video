package ports

import (
	"context"
	"errors"
	"time"
)

var ErrObjectNotFound = errors.New("object not found")

type PresignedUpload struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

type ObjectInfo struct {
	Size        int64
	ContentType string
}

type ObjectStorage interface {
	PresignPut(context.Context, string, string, int64, time.Duration) (PresignedUpload, error)
	StatObject(context.Context, string) (ObjectInfo, error)
	ReadObjectPrefix(context.Context, string, int64) ([]byte, error)
	DeleteObject(context.Context, string) error
}
