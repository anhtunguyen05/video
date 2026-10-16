package application

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type fakeRepository struct {
	jobs []domain.Job
}

func (repository *fakeRepository) CreateQueued(_ context.Context, job domain.Job) error {
	repository.jobs = append(repository.jobs, job)
	return nil
}

type fakeProcessor struct {
	job          domain.Job
	claim        bool
	succeeded    bool
	metadataDone bool
	prepared     bool
	failed       bool
	metadata     ports.VideoMetadata
	sizeBytes    int64
	renditions   []domain.Rendition
	started      []string
	completed    []string
	failureCode  string
	failureError string
}

func (processor *fakeProcessor) ClaimQueued(_ context.Context, _ string, _ time.Time) (domain.Job, bool, error) {
	return processor.job, processor.claim, nil
}

func (processor *fakeProcessor) PersistMetadata(_ context.Context, _ domain.Job, metadata ports.VideoMetadata, sizeBytes int64, _ time.Time) error {
	processor.metadataDone = true
	processor.metadata = metadata
	processor.sizeBytes = sizeBytes
	return nil
}

func (processor *fakeProcessor) PrepareTranscoding(_ context.Context, _ domain.Job, _ ports.GeneratedAsset, renditions []domain.Rendition, _ time.Time) error {
	processor.prepared = true
	processor.renditions = renditions
	return nil
}

func (processor *fakeProcessor) BeginRendition(_ context.Context, _ domain.Job, rendition domain.Rendition, _ time.Time) error {
	processor.started = append(processor.started, rendition.Name)
	return nil
}

func (processor *fakeProcessor) CompleteRendition(_ context.Context, _ domain.Job, _ ports.GeneratedAsset, rendition domain.Rendition, _ time.Time) error {
	processor.completed = append(processor.completed, rendition.Name)
	return nil
}

func (processor *fakeProcessor) MarkSucceeded(_ context.Context, _ domain.Job, _ time.Time) error {
	processor.succeeded = true
	return nil
}

func (processor *fakeProcessor) MarkRenditionFailed(_ context.Context, _ domain.Job, _ domain.Rendition, code, message string, _ time.Time) error {
	processor.failed = true
	processor.failureCode = code
	processor.failureError = message
	return nil
}

func (processor *fakeProcessor) MarkFailed(_ context.Context, _ domain.Job, code, message string, _ time.Time) error {
	processor.failed = true
	processor.failureCode = code
	processor.failureError = message
	return nil
}

type fakeStorage struct {
	content []byte
	err     error
}

func (storage fakeStorage) Download(_ context.Context, _ string, destination io.Writer) (ports.ObjectInfo, error) {
	if storage.err != nil {
		return ports.ObjectInfo{}, storage.err
	}
	if _, err := destination.Write(storage.content); err != nil {
		return ports.ObjectInfo{}, err
	}
	return ports.ObjectInfo{Size: int64(len(storage.content))}, nil
}

type fakeAssetStorage struct {
	content map[string][]byte
	err     error
}

func (storage *fakeAssetStorage) Upload(_ context.Context, objectKey string, _ string, source io.Reader, size int64) (ports.ObjectInfo, error) {
	if storage.err != nil {
		return ports.ObjectInfo{}, storage.err
	}
	content, err := io.ReadAll(source)
	if err != nil {
		return ports.ObjectInfo{}, err
	}
	if storage.content == nil {
		storage.content = make(map[string][]byte)
	}
	storage.content[objectKey] = content
	return ports.ObjectInfo{Size: size, ContentType: "image/jpeg"}, nil
}

type fakeThumbnail struct {
	err       error
	timestamp time.Duration
}

type fakeTranscoder struct{ err error }

func (transcoder fakeTranscoder) Transcode(_ context.Context, request ports.RenditionTranscodeRequest) error {
	if transcoder.err != nil {
		return transcoder.err
	}
	return os.WriteFile(request.OutputPath, []byte("mp4"), 0o600)
}

func (thumbnail *fakeThumbnail) Generate(_ context.Context, _ string, outputPath string, timestamp time.Duration) error {
	thumbnail.timestamp = timestamp
	if thumbnail.err != nil {
		return thumbnail.err
	}
	return os.WriteFile(outputPath, []byte("jpeg"), 0o600)
}

func newThumbnailService(repository *fakeRepository, processor *fakeProcessor, storage fakeStorage, probe fakeProbe, tempDir string) *Service {
	return NewProcessingService(
		repository,
		processor,
		storage,
		&fakeAssetStorage{},
		probe,
		&fakeThumbnail{},
		fakeTranscoder{},
		tempDir,
	)
}

type fakeProbe struct {
	metadata ports.VideoMetadata
	err      error
}

func (probe fakeProbe) Probe(_ context.Context, sourcePath string) (ports.VideoMetadata, error) {
	if _, err := os.Stat(sourcePath); err != nil {
		return ports.VideoMetadata{}, err
	}
	return probe.metadata, probe.err
}

func TestHandleVideoUploadedCreatesQueuedJob(t *testing.T) {
	repository := &fakeRepository{}
	service := NewServiceWithClock(repository, fixedClock{now: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)})
	body := []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"users/owner-1/videos/video-1/source/source.mp4","processing_version":2}}`)

	if err := service.HandleVideoUploaded(context.Background(), body); err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if len(repository.jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(repository.jobs))
	}
	job := repository.jobs[0]
	if job.Status != domain.StatusQueued || job.VideoID != "video-1" || job.ProcessingVersion != 2 || job.OperationKey != "video-1:PROCESS:v2" {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestHandleVideoUploadedRejectsWrongMessageType(t *testing.T) {
	service := NewService(&fakeRepository{})
	if err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"processing.execute.v1","schema_version":1,"payload":{}}`)); err == nil {
		t.Fatal("HandleVideoUploaded() error = nil, want invalid message")
	}
}

func TestHandleVideoUploadedProcessesQueuedJob(t *testing.T) {
	repository := &fakeRepository{}
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "source.mp4", ProcessingVersion: 1},
	}
	metadata := ports.VideoMetadata{DurationMS: 1500, Width: 1920, Height: 1080, Codec: "h264", Container: "mov"}
	thumbnail := &fakeThumbnail{}
	assets := &fakeAssetStorage{}
	service := NewProcessingServiceWithClock(
		repository,
		processor,
		fakeStorage{content: []byte("video")},
		assets,
		fakeProbe{metadata: metadata},
		thumbnail,
		fakeTranscoder{},
		t.TempDir(),
		fixedClock{now: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)},
	)

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.metadataDone || !processor.prepared || !processor.succeeded || processor.sizeBytes != 5 || processor.metadata != metadata {
		t.Fatalf("unexpected successful processing: %#v", processor)
	}
	if want := []domain.Rendition{
		{Name: "360p", Width: 640, Height: 360, Codec: domain.PlannedRenditionCodec},
		{Name: "720p", Width: 1280, Height: 720, Codec: domain.PlannedRenditionCodec},
		{Name: "1080p", Width: 1920, Height: 1080, Codec: domain.PlannedRenditionCodec},
	}; !reflect.DeepEqual(processor.renditions, want) {
		t.Fatalf("unexpected rendition plan: %#v", processor.renditions)
	}
	if thumbnail.timestamp != 150*time.Millisecond || string(assets.content["users/owner-1/videos/video-1/thumbnails/default.jpg"]) != "jpeg" {
		t.Fatalf("unexpected thumbnail output: timestamp=%s content=%q", thumbnail.timestamp, assets.content)
	}
	if want := []string{"360p", "720p", "1080p"}; !reflect.DeepEqual(processor.started, want) || !reflect.DeepEqual(processor.completed, want) {
		t.Fatalf("rendition order started=%#v completed=%#v, want %#v", processor.started, processor.completed, want)
	}
	for _, name := range []string{"360p", "720p", "1080p"} {
		key := filepath.Join("users", "owner-1", "videos", "video-1", "renditions", name, "video.mp4")
		key = filepath.ToSlash(key)
		if string(assets.content[key]) != "mp4" {
			t.Fatalf("rendition object %q = %q, want mp4", key, assets.content[key])
		}
	}
}

func TestHandleVideoUploadedMarksInvalidMediaFailed(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "source.mp4", ProcessingVersion: 1},
	}
	service := newThumbnailService(&fakeRepository{}, processor, fakeStorage{content: []byte("invalid")}, fakeProbe{err: ports.ErrInvalidMedia}, t.TempDir())

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.failed || processor.failureCode != FailureInvalidMedia || processor.failureError == "" {
		t.Fatalf("unexpected failed processing: %#v", processor)
	}
}

func TestHandleVideoUploadedMarksMissingSourceFailed(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "source.mp4", ProcessingVersion: 1},
	}
	service := newThumbnailService(&fakeRepository{}, processor, fakeStorage{err: ports.ErrObjectNotFound}, fakeProbe{}, t.TempDir())

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.failed || processor.failureCode != FailureSourceNotFound {
		t.Fatalf("unexpected failed processing: %#v", processor)
	}
}

func TestHandleVideoUploadedRejectsSourceWithoutSupportedRendition(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "source.mp4", ProcessingVersion: 1},
	}
	service := NewProcessingService(
		&fakeRepository{}, processor, fakeStorage{content: []byte("video")}, &fakeAssetStorage{},
		fakeProbe{metadata: ports.VideoMetadata{Width: 320, Height: 568}}, &fakeThumbnail{}, fakeTranscoder{}, t.TempDir(),
	)

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.failed || processor.failureCode != FailureNoRendition {
		t.Fatalf("unexpected no-rendition failure: %#v", processor)
	}
}

func TestHandleVideoUploadedMarksTranscodeFailure(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "source.mp4", ProcessingVersion: 1},
	}
	service := NewProcessingService(
		&fakeRepository{}, processor, fakeStorage{content: []byte("video")}, &fakeAssetStorage{},
		fakeProbe{metadata: ports.VideoMetadata{Width: 640, Height: 360}}, &fakeThumbnail{}, fakeTranscoder{err: errors.New("ffmpeg failed")}, t.TempDir(),
	)

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.failed || processor.failureCode != FailureTranscode || !reflect.DeepEqual(processor.started, []string{"360p"}) {
		t.Fatalf("unexpected transcode failure: %#v", processor)
	}
}

func TestHandleVideoUploadedPropagatesTransientSourceError(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", OwnerID: "owner-1", SourceObjectKey: "source.mp4", ProcessingVersion: 1},
	}
	service := newThumbnailService(&fakeRepository{}, processor, fakeStorage{err: errors.New("minio unavailable")}, fakeProbe{}, t.TempDir())

	if err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`)); err == nil {
		t.Fatal("HandleVideoUploaded() error = nil, want transient source error")
	}
}

func TestThumbnailTimestampClampsShortVideo(t *testing.T) {
	if got := thumbnailTimestamp(500); got != 0 {
		t.Fatalf("thumbnailTimestamp(500) = %s, want 0", got)
	}
	if got := thumbnailTimestamp(1500); got != 150*time.Millisecond {
		t.Fatalf("thumbnailTimestamp(1500) = %s, want 150ms", got)
	}
}
