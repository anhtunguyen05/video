package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"video/services/api/internal/media/application"
	"video/services/api/internal/media/domain"
	"video/services/api/internal/media/ports"
	platformauth "video/services/api/internal/platform/auth"
)

type fakeVideoService struct {
	video domain.Video
}

func (service *fakeVideoService) Create(_ context.Context, ownerID, title string, originalFilename *string) (domain.Video, error) {
	service.video = domain.Video{
		ID:                "00000000-0000-4000-8000-000000000002",
		OwnerID:           ownerID,
		Title:             title,
		OriginalFilename:  originalFilename,
		Status:            domain.StatusCreated,
		ProcessingVersion: 1,
		CreatedAt:         time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC),
	}
	return service.video, nil
}

func (service *fakeVideoService) Get(_ context.Context, ownerID, videoID string) (domain.Video, error) {
	if service.video.ID != videoID || service.video.OwnerID != ownerID {
		return domain.Video{}, application.ErrNotFound
	}
	return service.video, nil
}

func (service *fakeVideoService) List(_ context.Context, ownerID string, _ int, _ string) (ports.VideoPage, string, error) {
	if service.video.ID == "" || service.video.OwnerID != ownerID {
		return ports.VideoPage{Items: []domain.Video{}}, "", nil
	}
	return ports.VideoPage{Items: []domain.Video{service.video}}, "", nil
}

func (service *fakeVideoService) Delete(_ context.Context, ownerID, videoID string) error {
	if service.video.ID != videoID || service.video.OwnerID != ownerID {
		return application.ErrNotFound
	}
	return nil
}

func TestCreateVideoReturnsContractEnvelope(t *testing.T) {
	service := &fakeVideoService{}
	server := NewServerWithDependencies(nil, service, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/videos", strings.NewReader(`{"title":"Summer Trip"}`))
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header")
	}
	if !strings.Contains(recorder.Body.String(), `"status":"CREATED"`) {
		t.Fatalf("body = %s, want created status", recorder.Body.String())
	}
}

func TestGetVideoHidesOtherOwners(t *testing.T) {
	service := &fakeVideoService{video: domain.Video{
		ID:                "00000000-0000-4000-8000-000000000002",
		OwnerID:           "00000000-0000-4000-8000-000000000099",
		Title:             "Private",
		Status:            domain.StatusCreated,
		ProcessingVersion: 1,
	}}
	server := NewServerWithDependencies(nil, service, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/00000000-0000-4000-8000-000000000002", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestGetVideoReturnsProcessedMetadata(t *testing.T) {
	duration := int64(302000)
	width := 1920
	height := 1080
	codec := "h264"
	container := "mov"
	size := int64(123456)
	frameRate := 29.97
	service := &fakeVideoService{video: domain.Video{
		ID:                "00000000-0000-4000-8000-000000000002",
		OwnerID:           "00000000-0000-4000-8000-000000000001",
		Title:             "Processed",
		Status:            domain.StatusReady,
		SourceSizeBytes:   &size,
		SourceContainer:   &container,
		SourceCodec:       &codec,
		DurationMS:        &duration,
		Width:             &width,
		Height:            &height,
		FrameRate:         &frameRate,
		ProcessingVersion: 1,
	}}
	server := NewServerWithDependencies(nil, service, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/00000000-0000-4000-8000-000000000002", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"status":"READY"`, `"duration_ms":302000`, `"width":1920`, `"height":1080`, `"codec":"h264"`, `"container":"mov"`, `"source_size_bytes":123456`, `"frame_rate":29.97`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("body = %s, missing %s", body, expected)
		}
	}
}
