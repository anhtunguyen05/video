package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

type blockingSource struct {
	started chan struct{}
	release chan struct{}
}

func (source blockingSource) Download(ctx context.Context, _ string, destination io.Writer) (ports.ObjectInfo, error) {
	close(source.started)
	select {
	case <-source.release:
	case <-ctx.Done():
		return ports.ObjectInfo{}, ctx.Err()
	}
	_, err := destination.Write([]byte("source"))
	return ports.ObjectInfo{Size: 6}, err
}

type schedulerReliable struct {
	ports.ReliableRepository
	job    domain.Job
	claims int
}

func (repository *schedulerReliable) RecoverExpired(context.Context, time.Time) error { return nil }
func (repository *schedulerReliable) ClaimNext(context.Context, time.Time) (domain.Job, bool, error) {
	repository.claims++
	if repository.claims == 1 {
		return repository.job, true, nil
	}
	return domain.Job{}, false, nil
}
func (repository *schedulerReliable) RecordOperation(context.Context, domain.Job, string, string, string, time.Time) error {
	return nil
}
func (repository *schedulerReliable) RenewLease(context.Context, domain.Job, time.Time) (bool, error) {
	return true, nil
}

func TestSchedulerDrainsClaimedJobAfterClaimsStop(t *testing.T) {
	job, err := domain.NewQueuedJob(domain.VideoUploaded{
		VideoID: uuid.NewString(), OwnerID: uuid.NewString(), SourceObjectKey: "source.mp4", ProcessingVersion: 1,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	job.RunID = uuid.NewString()
	job.Attempt = 1
	source := blockingSource{started: make(chan struct{}), release: make(chan struct{})}
	processor := &fakeProcessor{}
	reliable := &schedulerReliable{job: job}
	service := NewReliableProcessingService(&fakeRepository{}, processor, reliable, source,
		&fakeAssetStorage{}, fakeProbe{metadata: ports.VideoMetadata{
			DurationMS: 6000, Width: 1280, Height: 720, Codec: "h264", Container: "mp4",
		}}, &fakeThumbnail{}, fakeTranscoder{}, fakePackager{}, t.TempDir())
	claimCtx, stopClaims := context.WithCancel(context.Background())
	defer stopClaims()
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.RunScheduler(claimCtx, workCtx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	select {
	case <-source.started:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not start the claimed job")
	}
	stopClaims()
	if workCtx.Err() != nil {
		t.Fatal("stopping claims canceled the active processing context")
	}
	select {
	case <-done:
		t.Fatal("scheduler exited before the claimed job finished")
	default:
	}
	close(source.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not exit after draining the job")
	}
	if !processor.succeeded || reliable.claims != 1 {
		t.Fatalf("drained job did not complete once: succeeded=%v claims=%d", processor.succeeded, reliable.claims)
	}
}

func TestClassifyProcessingError(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		code  string
		retry bool
	}{
		{"storage timeout", context.DeadlineExceeded, "PROCESS_INTERRUPTED", true},
		{"worker shutdown", context.Canceled, "PROCESS_INTERRUPTED", true},
		{"credential failure", errors.Join(errors.New("upload"), ports.ErrInvalidConfiguration), "INVALID_PROCESSING_CONFIGURATION", false},
		{"temporary dependency", errors.New("storage unavailable"), "PROCESS_TEMPORARY_ERROR", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, retry := classifyProcessingError(tc.err)
			if code != tc.code || retry != tc.retry {
				t.Fatalf("classify = %s/%v, want %s/%v", code, retry, tc.code, tc.retry)
			}
		})
	}
}
