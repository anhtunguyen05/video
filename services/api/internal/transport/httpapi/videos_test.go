package httpapi

import (
	"context"
	"fmt"
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

type fakeAssetSigner struct{ objects map[string][]byte }

func (fakeAssetSigner) PresignAsset(_ context.Context, objectKey string, expiry time.Duration) (string, time.Time, error) {
	return fmt.Sprintf("http://localhost:9000/video-platform/%s?signed=1&expires=%d", objectKey, int64(expiry.Seconds())), time.Now().UTC().Add(expiry), nil
}

func (signer fakeAssetSigner) ReadAsset(_ context.Context, objectKey string) ([]byte, error) {
	content, ok := signer.objects[objectKey]
	if !ok {
		return nil, fmt.Errorf("missing object")
	}
	return content, nil
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

func TestGetVideoReturnsSignedThumbnailForOwnedAsset(t *testing.T) {
	service := &fakeVideoService{video: domain.Video{
		ID:                "00000000-0000-4000-8000-000000000002",
		OwnerID:           "00000000-0000-4000-8000-000000000001",
		Title:             "Thumbnail",
		Status:            domain.StatusReady,
		ProcessingVersion: 1,
		Thumbnail:         &domain.ThumbnailAsset{ObjectKey: "users/owner/videos/video/thumbnails/default.jpg"},
	}}
	server := NewServerWithAssetDependencies(nil, service, nil, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"}, fakeAssetSigner{}, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/00000000-0000-4000-8000-000000000002", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"thumbnail":{"url":"http://localhost:9000/video-platform/users/owner/videos/video/thumbnails/default.jpg?signed=1`) || !strings.Contains(body, `expires=60`) {
		t.Fatalf("body = %s, want signed thumbnail URL", body)
	}
}

func TestGetVideoReturnsSignedRenditionForOwnedAsset(t *testing.T) {
	objectKey := "users/owner/videos/video/renditions/360p/video.mp4"
	service := &fakeVideoService{video: domain.Video{
		ID:                "00000000-0000-4000-8000-000000000002",
		OwnerID:           "00000000-0000-4000-8000-000000000001",
		Title:             "Rendition",
		Status:            domain.StatusReady,
		ProcessingVersion: 1,
		Renditions: []domain.RenditionAsset{{
			Name: "360p", Width: 640, Height: 360, Codec: "h264", Status: "READY", ObjectKey: &objectKey,
		}},
	}}
	server := NewServerWithAssetDependencies(nil, service, nil, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"}, fakeAssetSigner{}, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/00000000-0000-4000-8000-000000000002", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"renditions":[{"name":"360p","width":640,"height":360,"codec":"h264","status":"READY","url":"http://localhost:9000/video-platform/users/owner/videos/video/renditions/360p/video.mp4?signed=1`) || !strings.Contains(recorder.Body.String(), `expires=60`) {
		t.Fatalf("body = %s, want signed rendition URL", recorder.Body.String())
	}
}

func TestPlaybackReturnsAuthenticatedManifestURL(t *testing.T) {
	videoID := "00000000-0000-4000-8000-000000000002"
	service := &fakeVideoService{video: domain.Video{ID: videoID, OwnerID: "00000000-0000-4000-8000-000000000001", Status: domain.StatusReady, HLSMaster: &domain.HLSAsset{Variant: "default", ObjectKey: "users/owner/videos/video/hls/master.m3u8"}}}
	server := NewServerWithAssetDependencies(nil, service, nil, platformauth.StaticPrincipal{ID: service.video.OwnerID}, fakeAssetSigner{}, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/playback", nil)
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"type":"hls"`) || !strings.Contains(recorder.Body.String(), `/api/v1/videos/`+videoID+`/playback/hls/master.m3u8`) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHLSManifestRewritesVariantAndSignsSegments(t *testing.T) {
	videoID := "00000000-0000-4000-8000-000000000002"
	ownerID := "00000000-0000-4000-8000-000000000001"
	masterKey := "users/owner/videos/video/hls/master.m3u8"
	variantKey := "users/owner/videos/video/hls/360p/index.m3u8"
	signer := fakeAssetSigner{objects: map[string][]byte{
		masterKey:  []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=100000,RESOLUTION=640x360\n360p/index.m3u8\n"),
		variantKey: []byte("#EXTM3U\n#EXTINF:6,\nsegment_00000.ts\n#EXT-X-ENDLIST\n"),
	}}
	service := &fakeVideoService{video: domain.Video{ID: videoID, OwnerID: ownerID, Status: domain.StatusReady, HLSMaster: &domain.HLSAsset{Variant: "default", ObjectKey: masterKey}, HLSVariants: []domain.HLSAsset{{Variant: "360p", ObjectKey: variantKey}}}}
	server := NewServerWithAssetDependencies(nil, service, nil, platformauth.StaticPrincipal{ID: ownerID}, signer, time.Minute)
	for _, test := range []struct{ path, contains string }{
		{"master.m3u8", "360p/index.m3u8"},
		{"360p/index.m3u8", "http://localhost:9000/video-platform/users/owner/videos/video/hls/360p/segment_00000.ts?signed=1&expires=60"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/playback/hls/"+test.path, nil)
		recorder := httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.contains) || recorder.Header().Get("Content-Type") != "application/vnd.apple.mpegurl" {
			t.Fatalf("path=%s status=%d body=%s", test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestPlaybackHidesOtherOwners(t *testing.T) {
	videoID := "00000000-0000-4000-8000-000000000002"
	service := &fakeVideoService{video: domain.Video{ID: videoID, OwnerID: "00000000-0000-4000-8000-000000000099", Status: domain.StatusReady}}
	server := NewServerWithAssetDependencies(nil, service, nil, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"}, fakeAssetSigner{}, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/playback", nil)
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHLSManifestRejectsInvalidPathAndUnreadyVideo(t *testing.T) {
	videoID := "00000000-0000-4000-8000-000000000002"
	ownerID := "00000000-0000-4000-8000-000000000001"
	service := &fakeVideoService{video: domain.Video{ID: videoID, OwnerID: ownerID, Status: domain.StatusProcessing}}
	server := NewServerWithAssetDependencies(nil, service, nil, platformauth.StaticPrincipal{ID: ownerID}, fakeAssetSigner{}, time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/playback/hls/master.m3u8", nil)
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("unready status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	service.video.Status = domain.StatusReady
	service.video.HLSMaster = &domain.HLSAsset{Variant: "default", ObjectKey: "master.m3u8"}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/videos/"+videoID+"/playback/hls/unknown/index.m3u8", nil)
	recorder = httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("invalid path status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
