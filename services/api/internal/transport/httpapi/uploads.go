package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"video/services/api/internal/platform/ids"
	uploadapplication "video/services/api/internal/upload/application"
)

type UploadService interface {
	Create(context.Context, string, string, string, int64) (uploadapplication.CreateResult, error)
	Complete(context.Context, string, string) (uploadapplication.CompleteResult, error)
	Abort(context.Context, string, string) error
}

type createUploadRequest struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type uploadResponse struct {
	UploadID  string            `json:"upload_id"`
	UploadURL string            `json:"upload_url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt string            `json:"expires_at"`
}

func (s *Server) createUpload(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	if s.uploads == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "Uploads are temporarily unavailable.")
		return
	}
	videoID := request.PathValue("videoId")
	if !ids.IsUUID(videoID) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return
	}
	var payload createUploadRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "The request body is invalid.")
		return
	}
	result, err := s.uploads.Create(request.Context(), principal.ID, videoID, payload.ContentType, payload.SizeBytes)
	if err != nil {
		writeCreateUploadError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"data": toUploadResponse(result)})
}

func (s *Server) completeUpload(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	if s.uploads == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "Uploads are temporarily unavailable.")
		return
	}
	uploadID := request.PathValue("uploadId")
	if !ids.IsUUID(uploadID) {
		writeAPIError(writer, request, http.StatusNotFound, "UPLOAD_NOT_FOUND", "Upload was not found.")
		return
	}
	result, err := s.uploads.Complete(request.Context(), principal.ID, uploadID)
	if err != nil {
		writeCompleteUploadError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": map[string]string{
		"video_id": result.VideoID,
		"status":   string(result.Status),
	}})
}

func (s *Server) abortUpload(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	if s.uploads == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "Uploads are temporarily unavailable.")
		return
	}
	uploadID := request.PathValue("uploadId")
	if !ids.IsUUID(uploadID) {
		writeAPIError(writer, request, http.StatusNotFound, "UPLOAD_NOT_FOUND", "Upload was not found.")
		return
	}
	if err := s.uploads.Abort(request.Context(), principal.ID, uploadID); err != nil {
		if errors.Is(err, uploadapplication.ErrUploadNotFound) {
			writeAPIError(writer, request, http.StatusNotFound, "UPLOAD_NOT_FOUND", "Upload was not found.")
			return
		}
		if errors.Is(err, uploadapplication.ErrInvalidState) {
			writeAPIError(writer, request, http.StatusConflict, "INVALID_UPLOAD_STATE", "The upload cannot be aborted in its current state.")
			return
		}
		writeAPIError(writer, request, http.StatusInternalServerError, "UPLOAD_ABORT_FAILED", "The upload could not be aborted.")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func writeCreateUploadError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, uploadapplication.ErrVideoNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
	case errors.Is(err, uploadapplication.ErrUploadTooLarge):
		writeAPIError(writer, request, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "The upload exceeds the maximum allowed size.")
	case errors.Is(err, uploadapplication.ErrUnsupportedMedia):
		writeAPIError(writer, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "The media type is not supported.")
	case errors.Is(err, uploadapplication.ErrInvalidVideoState):
		writeAPIError(writer, request, http.StatusConflict, "INVALID_VIDEO_STATE", "The video cannot start an upload in its current state.")
	case errors.Is(err, uploadapplication.ErrUploadAlreadyActive):
		writeAPIError(writer, request, http.StatusConflict, "UPLOAD_ALREADY_ACTIVE", "The video already has an active upload.")
	case errors.Is(err, uploadapplication.ErrInvalidUploadInput):
		writeAPIError(writer, request, http.StatusBadRequest, "INVALID_UPLOAD", "The upload request is invalid.")
	default:
		writeAPIError(writer, request, http.StatusInternalServerError, "UPLOAD_CREATE_FAILED", "The upload could not be created.")
	}
}

func writeCompleteUploadError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, uploadapplication.ErrUploadNotFound):
		writeAPIError(writer, request, http.StatusNotFound, "UPLOAD_NOT_FOUND", "Upload was not found.")
	case errors.Is(err, uploadapplication.ErrUnsupportedMedia), errors.Is(err, uploadapplication.ErrInvalidObject):
		writeAPIError(writer, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "The uploaded object is not a supported video.")
	case errors.Is(err, uploadapplication.ErrObjectMissing):
		writeAPIError(writer, request, http.StatusConflict, "UPLOAD_OBJECT_MISSING", "The uploaded object was not found.")
	case errors.Is(err, uploadapplication.ErrSizeMismatch):
		writeAPIError(writer, request, http.StatusConflict, "UPLOAD_SIZE_MISMATCH", "The uploaded object size does not match the upload request.")
	case errors.Is(err, uploadapplication.ErrExpired):
		writeAPIError(writer, request, http.StatusConflict, "UPLOAD_EXPIRED", "The upload session has expired.")
	case errors.Is(err, uploadapplication.ErrInvalidState):
		writeAPIError(writer, request, http.StatusConflict, "INVALID_UPLOAD_STATE", "The upload cannot be completed in its current state.")
	default:
		writeAPIError(writer, request, http.StatusInternalServerError, "UPLOAD_COMPLETE_FAILED", "The upload could not be completed.")
	}
}

func toUploadResponse(result uploadapplication.CreateResult) uploadResponse {
	return uploadResponse{
		UploadID:  result.Session.ID,
		UploadURL: result.Presigned.URL,
		Method:    result.Presigned.Method,
		Headers:   result.Presigned.Headers,
		ExpiresAt: result.Session.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
}

var _ UploadService = (*uploadapplication.Service)(nil)
