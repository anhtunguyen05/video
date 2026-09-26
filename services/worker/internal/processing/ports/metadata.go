package ports

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrObjectNotFound = errors.New("object not found")
var ErrObjectUnreadable = errors.New("object unreadable")
var ErrInvalidMedia = errors.New("invalid media")

type ObjectInfo struct {
	Size        int64
	ContentType string
}

type SourceStorage interface {
	Download(context.Context, string, io.Writer) (ObjectInfo, error)
}

type AssetStorage interface {
	Upload(context.Context, string, string, io.Reader, int64) (ObjectInfo, error)
}

type GeneratedAsset struct {
	ID          string
	VideoID     string
	AssetType   string
	Variant     string
	ObjectKey   string
	ContentType string
	SizeBytes   int64
}

type VideoMetadata struct {
	DurationMS int64
	Width      int
	Height     int
	Codec      string
	Container  string
	FrameRate  *float64
}

type MetadataProbe interface {
	Probe(context.Context, string) (VideoMetadata, error)
}

type ThumbnailGenerator interface {
	Generate(context.Context, string, string, time.Duration) error
}
