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
	Delete(context.Context, string) error
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

// RenditionTranscoder creates one playable MP4 rendition from a downloaded
// source video. The application service owns ordering and persistence; this
// port owns only the media-tool invocation.
type RenditionTranscoder interface {
	Transcode(context.Context, RenditionTranscodeRequest) error
}

type RenditionTranscodeRequest struct {
	SourcePath string
	OutputPath string
	Width      int
	Height     int
}

type HLSVariantRequest struct {
	InputPath string
	OutputDir string
}

type HLSVariantPackager interface {
	PackageVariant(context.Context, HLSVariantRequest) error
}

type HLSVariant struct {
	Name      string
	Width     int
	Height    int
	Bandwidth int64
}
