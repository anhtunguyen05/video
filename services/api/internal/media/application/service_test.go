package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"video/services/api/internal/media/domain"
	"video/services/api/internal/media/ports"
)

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }

type fakeVideoRepository struct {
	created domain.Video
	videos  map[string]domain.Video
}

func (repository *fakeVideoRepository) Create(_ context.Context, video domain.Video) error {
	if repository.videos == nil {
		repository.videos = make(map[string]domain.Video)
	}
	repository.created = video
	repository.videos[video.ID] = video
	return nil
}

func (repository *fakeVideoRepository) GetOwned(_ context.Context, ownerID, videoID string) (domain.Video, error) {
	video, ok := repository.videos[videoID]
	if !ok || video.OwnerID != ownerID || video.DeletedAt != nil {
		return domain.Video{}, ports.ErrNotFound
	}
	return video, nil
}

func (repository *fakeVideoRepository) ListOwned(_ context.Context, ownerID string, limit int, _ *ports.VideoCursor) (ports.VideoPage, error) {
	items := make([]domain.Video, 0, limit)
	for _, video := range repository.videos {
		if video.OwnerID == ownerID && video.DeletedAt == nil {
			items = append(items, video)
		}
	}
	return ports.VideoPage{Items: items}, nil
}

func (repository *fakeVideoRepository) DeleteOwned(_ context.Context, video domain.Video) error {
	if _, ok := repository.videos[video.ID]; !ok {
		return ports.ErrNotFound
	}
	repository.videos[video.ID] = video
	return nil
}

func TestServiceCreateTrimsTitleAndSetsInitialState(t *testing.T) {
	repository := &fakeVideoRepository{}
	clock := fixedClock{now: time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)}
	service := NewServiceWithClock(repository, clock)

	video, err := service.Create(context.Background(), "owner-id", "  Summer Trip  ", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if video.Title != "Summer Trip" || video.Status != domain.StatusCreated || video.CreatedAt != clock.now {
		t.Fatalf("unexpected video: %#v", video)
	}
	if repository.created.ID == "" {
		t.Fatal("expected generated video ID")
	}
}

func TestServiceRejectsInvalidCursor(t *testing.T) {
	service := NewService(&fakeVideoRepository{})
	_, _, err := service.List(context.Background(), "owner-id", 20, "not-a-cursor")
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("List() error = %v, want ErrInvalidCursor", err)
	}
}
