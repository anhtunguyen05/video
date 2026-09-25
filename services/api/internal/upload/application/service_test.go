package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"video/services/api/internal/media/domain"
	uploaddomain "video/services/api/internal/upload/domain"
	"video/services/api/internal/upload/ports"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type fakeVideoReader struct {
	video domain.Video
	err   error
}

func (reader fakeVideoReader) Get(context.Context, string, string) (domain.Video, error) {
	if reader.err != nil {
		return domain.Video{}, reader.err
	}
	return reader.video, nil
}

type fakeUploadRepository struct {
	session       uploaddomain.Session
	createdOwner  string
	completed     bool
	aborted       bool
	expired       bool
	completeError error
}

func (repository *fakeUploadRepository) Create(_ context.Context, ownerID string, session uploaddomain.Session) error {
	repository.createdOwner = ownerID
	repository.session = session
	return nil
}

func (repository *fakeUploadRepository) GetOwned(_ context.Context, _, _ string) (uploaddomain.Session, error) {
	if repository.session.ID == "" {
		return uploaddomain.Session{}, ports.ErrNotFound
	}
	return repository.session, nil
}

func (repository *fakeUploadRepository) CompleteOwned(_ context.Context, _, _ string, sizeBytes int64, now time.Time) (uploaddomain.Session, error) {
	if repository.completeError != nil {
		return uploaddomain.Session{}, repository.completeError
	}
	if err := repository.session.Complete(now, sizeBytes); err != nil {
		return uploaddomain.Session{}, err
	}
	repository.completed = true
	return repository.session, nil
}

func (repository *fakeUploadRepository) AbortOwned(_ context.Context, _, _ string, now time.Time) error {
	repository.aborted = true
	return repository.session.Abort(now)
}

func (repository *fakeUploadRepository) ExpireOwned(_ context.Context, _, _ string, now time.Time) error {
	repository.expired = true
	return repository.session.Expire(now)
}

type fakeObjectStorage struct {
	presigned   ports.PresignedUpload
	object      ports.ObjectInfo
	prefix      []byte
	statError   error
	prefixError error
	deleted     bool
	presignCall bool
}

type fakePublisher struct {
	events []ports.VideoUploaded
	err    error
}

func (publisher *fakePublisher) PublishVideoUploaded(_ context.Context, event ports.VideoUploaded) error {
	if publisher.err != nil {
		return publisher.err
	}
	publisher.events = append(publisher.events, event)
	return nil
}

func (storage *fakeObjectStorage) PresignPut(_ context.Context, _, _ string, _ int64, _ time.Duration) (ports.PresignedUpload, error) {
	storage.presignCall = true
	return storage.presigned, nil
}

func (storage *fakeObjectStorage) StatObject(_ context.Context, _ string) (ports.ObjectInfo, error) {
	if storage.statError != nil {
		return ports.ObjectInfo{}, storage.statError
	}
	return storage.object, nil
}

func (storage *fakeObjectStorage) ReadObjectPrefix(_ context.Context, _ string, _ int64) ([]byte, error) {
	if storage.prefixError != nil {
		return nil, storage.prefixError
	}
	return storage.prefix, nil
}

func (storage *fakeObjectStorage) DeleteObject(_ context.Context, _ string) error {
	storage.deleted = true
	return nil
}

func newTestService(repository *fakeUploadRepository, storage *fakeObjectStorage) *Service {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	return NewServiceWithClock(
		repository,
		fakeVideoReader{video: domain.Video{ID: "video-1", OwnerID: "owner-1", Status: domain.StatusCreated}},
		storage,
		Policy{
			MaxSizeBytes: 100,
			AllowedTypes: map[string]struct{}{"video/mp4": {}, "video/webm": {}},
			URLExpiry:    15 * time.Minute,
		},
		fixedClock{now: now},
	)
}

func TestCreatePresignsAndPersistsUploadSession(t *testing.T) {
	repository := &fakeUploadRepository{}
	storage := &fakeObjectStorage{presigned: ports.PresignedUpload{
		URL:       "http://minio/upload",
		Method:    "PUT",
		Headers:   map[string]string{"Content-Type": "video/mp4"},
		ExpiresAt: time.Date(2026, 9, 24, 0, 15, 0, 0, time.UTC),
	}}
	service := newTestService(repository, storage)

	result, err := service.Create(context.Background(), "owner-1", "video-1", "VIDEO/MP4", 80)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !storage.presignCall || repository.createdOwner != "owner-1" {
		t.Fatal("expected presign and persistence")
	}
	if result.Session.Status != uploaddomain.StatusUploading || result.Session.ObjectKey != "users/owner-1/videos/video-1/source/source.mp4" {
		t.Fatalf("unexpected session: %#v", result.Session)
	}
}

func TestCreateRejectsUnsupportedOrOversizedFile(t *testing.T) {
	storage := &fakeObjectStorage{}
	service := newTestService(&fakeUploadRepository{}, storage)

	if _, err := service.Create(context.Background(), "owner-1", "video-1", "video/avi", 10); !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("unsupported type error = %v", err)
	}
	if _, err := service.Create(context.Background(), "owner-1", "video-1", "video/mp4", 101); !errors.Is(err, ErrUploadTooLarge) {
		t.Fatalf("oversized error = %v", err)
	}
	if storage.presignCall {
		t.Fatal("storage should not be called for invalid input")
	}
}

func TestCompleteVerifiesObjectAndUpdatesVideo(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	repository := &fakeUploadRepository{session: uploaddomain.Session{
		ID:                  "upload-1",
		VideoID:             "video-1",
		ObjectKey:           "users/owner-1/videos/video-1/source/source.mp4",
		Status:              uploaddomain.StatusUploading,
		ExpectedContentType: "video/mp4",
		ExpectedSizeBytes:   8,
		ExpiresAt:           now.Add(time.Hour),
	}}
	storage := &fakeObjectStorage{
		object: ports.ObjectInfo{Size: 8, ContentType: "application/octet-stream"},
		prefix: []byte{0, 0, 0, 24, 'f', 't', 'y', 'p'},
	}
	service := newTestService(repository, storage)

	result, err := service.Complete(context.Background(), "owner-1", "upload-1")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.VideoID != "video-1" || result.Status != domain.StatusUploaded || !repository.completed {
		t.Fatalf("unexpected completion: %#v", result)
	}
}

func TestCompletePublishesVideoUploadedEvent(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	repository := &fakeUploadRepository{session: uploaddomain.Session{
		ID:                  "upload-1",
		VideoID:             "video-1",
		ObjectKey:           "users/owner-1/videos/video-1/source/source.mp4",
		Status:              uploaddomain.StatusUploading,
		ExpectedContentType: "video/mp4",
		ExpectedSizeBytes:   8,
		ExpiresAt:           now.Add(time.Hour),
	}}
	storage := &fakeObjectStorage{
		object: ports.ObjectInfo{Size: 8},
		prefix: []byte{0, 0, 0, 24, 'f', 't', 'y', 'p'},
	}
	publisher := &fakePublisher{}
	service := NewServiceWithPublisherAndClock(
		repository,
		fakeVideoReader{video: domain.Video{ID: "video-1", OwnerID: "owner-1", ProcessingVersion: 2}},
		storage,
		Policy{MaxSizeBytes: 100, AllowedTypes: map[string]struct{}{"video/mp4": {}}, URLExpiry: time.Minute},
		publisher,
		fixedClock{now: now},
	)

	if _, err := service.Complete(context.Background(), "owner-1", "upload-1"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
	event := publisher.events[0]
	if event.VideoID != "video-1" || event.OwnerID != "owner-1" || event.SourceObjectKey != repository.session.ObjectKey || event.ProcessingVersion != 2 {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestCompleteRepublishesForCompletedSession(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	repository := &fakeUploadRepository{session: uploaddomain.Session{
		ID:        "upload-1",
		VideoID:   "video-1",
		ObjectKey: "users/owner-1/videos/video-1/source/source.mp4",
		Status:    uploaddomain.StatusCompleted,
		ExpiresAt: now.Add(time.Hour),
	}}
	publisher := &fakePublisher{}
	service := NewServiceWithPublisherAndClock(
		repository,
		fakeVideoReader{video: domain.Video{ID: "video-1", ProcessingVersion: 1}},
		&fakeObjectStorage{},
		Policy{},
		publisher,
		fixedClock{now: now},
	)

	if _, err := service.Complete(context.Background(), "owner-1", "upload-1"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(publisher.events))
	}
}

func TestCompleteIsIdempotentForCompletedSession(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	repository := &fakeUploadRepository{session: uploaddomain.Session{
		ID:        "upload-1",
		VideoID:   "video-1",
		Status:    uploaddomain.StatusCompleted,
		ExpiresAt: now.Add(time.Hour),
	}}
	storage := &fakeObjectStorage{statError: errors.New("must not stat completed upload")}
	service := newTestService(repository, storage)

	result, err := service.Complete(context.Background(), "owner-1", "upload-1")
	if err != nil || result.Status != domain.StatusUploaded {
		t.Fatalf("Complete() = %#v, %v", result, err)
	}
}

func TestCompleteRejectsMissingObject(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	repository := &fakeUploadRepository{session: uploaddomain.Session{
		ID:                  "upload-1",
		VideoID:             "video-1",
		Status:              uploaddomain.StatusUploading,
		ExpectedContentType: "video/mp4",
		ExpectedSizeBytes:   8,
		ExpiresAt:           now.Add(time.Hour),
	}}
	storage := &fakeObjectStorage{statError: ports.ErrObjectNotFound}
	service := newTestService(repository, storage)

	if _, err := service.Complete(context.Background(), "owner-1", "upload-1"); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("Complete() error = %v, want missing object", err)
	}
}

func TestAbortDeletesObjectAndMarksSession(t *testing.T) {
	repository := &fakeUploadRepository{session: uploaddomain.Session{
		ID:      "upload-1",
		VideoID: "video-1",
		Status:  uploaddomain.StatusUploading,
	}}
	storage := &fakeObjectStorage{}
	service := newTestService(repository, storage)

	if err := service.Abort(context.Background(), "owner-1", "upload-1"); err != nil {
		t.Fatalf("Abort() error = %v", err)
	}
	if !storage.deleted || !repository.aborted {
		t.Fatal("expected object deletion and session abort")
	}
}
