package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"video/services/api/internal/media/domain"
	platformauth "video/services/api/internal/platform/auth"
	uploadapplication "video/services/api/internal/upload/application"
	uploaddomain "video/services/api/internal/upload/domain"
	"video/services/api/internal/upload/ports"
)

type fakeUploadService struct {
	createdResult  uploadapplication.CreateResult
	createdErr     error
	completeResult uploadapplication.CompleteResult
	completeErr    error
	abortErr       error
	ownerID        string
	videoID        string
	uploadID       string
}

func (service *fakeUploadService) Create(_ context.Context, ownerID, videoID, _ string, _ int64) (uploadapplication.CreateResult, error) {
	service.ownerID = ownerID
	service.videoID = videoID
	return service.createdResult, service.createdErr
}

func (service *fakeUploadService) Complete(_ context.Context, ownerID, uploadID string) (uploadapplication.CompleteResult, error) {
	service.ownerID = ownerID
	service.uploadID = uploadID
	return service.completeResult, service.completeErr
}

func (service *fakeUploadService) Abort(_ context.Context, ownerID, uploadID string) error {
	service.ownerID = ownerID
	service.uploadID = uploadID
	return service.abortErr
}

func newUploadServer(service UploadService) *http.Server {
	return NewServerWithUploadDependencies(nil, &fakeVideoService{}, service, platformauth.StaticPrincipal{ID: "00000000-0000-4000-8000-000000000001"})
}

func TestCreateUploadReturnsPresignedContract(t *testing.T) {
	service := &fakeUploadService{createdResult: uploadapplication.CreateResult{
		Session: uploaddomain.Session{
			ID:        "00000000-0000-4000-8000-000000000010",
			ExpiresAt: time.Date(2026, 9, 24, 0, 15, 0, 0, time.UTC),
		},
		Presigned: ports.PresignedUpload{
			URL:     "http://localhost:9000/upload",
			Method:  "PUT",
			Headers: map[string]string{"Content-Type": "video/mp4"},
		},
	}}
	server := newUploadServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/videos/00000000-0000-4000-8000-000000000002/uploads", strings.NewReader(`{"content_type":"video/mp4","size_bytes":80}`))
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	if service.ownerID != "00000000-0000-4000-8000-000000000001" || service.videoID == "" {
		t.Fatal("expected authenticated owner and video id")
	}
	if !strings.Contains(recorder.Body.String(), `"method":"PUT"`) || !strings.Contains(recorder.Body.String(), `"upload_id":"00000000-0000-4000-8000-000000000010"`) {
		t.Fatalf("body = %s, want presigned upload contract", recorder.Body.String())
	}
}

func TestCreateUploadMapsValidationErrors(t *testing.T) {
	service := &fakeUploadService{createdErr: uploadapplication.ErrUploadTooLarge}
	server := newUploadServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/videos/00000000-0000-4000-8000-000000000002/uploads", strings.NewReader(`{"content_type":"video/mp4","size_bytes":101}`))
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), `"code":"FILE_TOO_LARGE"`) {
		t.Fatalf("status/body = %d/%s, want 413 FILE_TOO_LARGE", recorder.Code, recorder.Body.String())
	}
}

func TestCompleteUploadReturnsUploadedStatus(t *testing.T) {
	service := &fakeUploadService{completeResult: uploadapplication.CompleteResult{
		VideoID: "00000000-0000-4000-8000-000000000002",
		Status:  domain.StatusUploaded,
	}}
	server := newUploadServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/00000000-0000-4000-8000-000000000010/complete", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"UPLOADED"`) {
		t.Fatalf("status/body = %d/%s, want uploaded response", recorder.Code, recorder.Body.String())
	}
	if service.uploadID != "00000000-0000-4000-8000-000000000010" {
		t.Fatalf("upload id = %s", service.uploadID)
	}
}

func TestCompleteUploadHidesUnauthorizedUpload(t *testing.T) {
	service := &fakeUploadService{completeErr: uploadapplication.ErrUploadNotFound}
	server := newUploadServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/00000000-0000-4000-8000-000000000010/complete", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"UPLOAD_NOT_FOUND"`) {
		t.Fatalf("status/body = %d/%s, want hidden upload", recorder.Code, recorder.Body.String())
	}
}

func TestAbortUploadReturnsNoContent(t *testing.T) {
	service := &fakeUploadService{}
	server := newUploadServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/00000000-0000-4000-8000-000000000010/abort", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent || service.uploadID == "" {
		t.Fatalf("status/upload id = %d/%s, want 204", recorder.Code, service.uploadID)
	}
}

func TestAbortUploadMapsInvalidState(t *testing.T) {
	service := &fakeUploadService{abortErr: uploadapplication.ErrInvalidState}
	server := newUploadServer(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/00000000-0000-4000-8000-000000000010/abort", nil)
	recorder := httptest.NewRecorder()

	server.Handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"INVALID_UPLOAD_STATE"`) {
		t.Fatalf("status/body = %d/%s, want conflict", recorder.Code, recorder.Body.String())
	}
}

var _ UploadService = (*fakeUploadService)(nil)
