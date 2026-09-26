package application

import (
	"context"
	"errors"
	"io"
	"os"
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
	failed       bool
	metadata     ports.VideoMetadata
	sizeBytes    int64
	failureCode  string
	failureError string
}

func (processor *fakeProcessor) ClaimQueued(_ context.Context, _ string, _ time.Time) (domain.Job, bool, error) {
	return processor.job, processor.claim, nil
}

func (processor *fakeProcessor) MarkSucceeded(_ context.Context, _ domain.Job, metadata ports.VideoMetadata, sizeBytes int64, _ time.Time) error {
	processor.succeeded = true
	processor.metadata = metadata
	processor.sizeBytes = sizeBytes
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
		job:   domain.Job{ID: "job-1", VideoID: "video-1", SourceObjectKey: "source.mp4"},
	}
	metadata := ports.VideoMetadata{DurationMS: 1500, Width: 1920, Height: 1080, Codec: "h264", Container: "mov"}
	service := NewProcessingServiceWithClock(
		repository,
		processor,
		fakeStorage{content: []byte("video")},
		fakeProbe{metadata: metadata},
		t.TempDir(),
		fixedClock{now: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)},
	)

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.succeeded || processor.sizeBytes != 5 || processor.metadata != metadata {
		t.Fatalf("unexpected successful processing: %#v", processor)
	}
}

func TestHandleVideoUploadedMarksInvalidMediaFailed(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", SourceObjectKey: "source.mp4"},
	}
	service := NewProcessingService(
		&fakeRepository{},
		processor,
		fakeStorage{content: []byte("invalid")},
		fakeProbe{err: ports.ErrInvalidMedia},
		t.TempDir(),
	)

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
		job:   domain.Job{ID: "job-1", VideoID: "video-1", SourceObjectKey: "source.mp4"},
	}
	service := NewProcessingService(
		&fakeRepository{},
		processor,
		fakeStorage{err: ports.ErrObjectNotFound},
		fakeProbe{},
		t.TempDir(),
	)

	err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`))
	if err != nil {
		t.Fatalf("HandleVideoUploaded() error = %v", err)
	}
	if !processor.failed || processor.failureCode != FailureSourceNotFound {
		t.Fatalf("unexpected failed processing: %#v", processor)
	}
}

func TestHandleVideoUploadedPropagatesTransientSourceError(t *testing.T) {
	processor := &fakeProcessor{
		claim: true,
		job:   domain.Job{ID: "job-1", VideoID: "video-1", SourceObjectKey: "source.mp4"},
	}
	service := NewProcessingService(
		&fakeRepository{},
		processor,
		fakeStorage{err: errors.New("minio unavailable")},
		fakeProbe{},
		t.TempDir(),
	)

	if err := service.HandleVideoUploaded(context.Background(), []byte(`{"message_type":"video.uploaded.v1","schema_version":1,"payload":{"video_id":"video-1","owner_id":"owner-1","source_object_key":"source.mp4","processing_version":1}}`)); err == nil {
		t.Fatal("HandleVideoUploaded() error = nil, want transient source error")
	}
}
