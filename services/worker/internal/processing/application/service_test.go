package application

import (
	"context"
	"testing"
	"time"

	"video/services/worker/internal/processing/domain"
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
