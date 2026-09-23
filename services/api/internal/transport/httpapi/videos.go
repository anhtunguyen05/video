package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"video/services/api/internal/media/application"
	"video/services/api/internal/media/domain"
	"video/services/api/internal/media/ports"
	"video/services/api/internal/platform/ids"
)

type VideoService interface {
	Create(context.Context, string, string, *string) (domain.Video, error)
	Get(context.Context, string, string) (domain.Video, error)
	List(context.Context, string, int, string) (ports.VideoPage, string, error)
	Delete(context.Context, string, string) error
}

type createVideoRequest struct {
	Title            string  `json:"title"`
	OriginalFilename *string `json:"original_filename"`
}

type videoResponse struct {
	ID                string  `json:"id"`
	Title             string  `json:"title"`
	OriginalFilename  *string `json:"original_filename"`
	Status            string  `json:"status"`
	ProcessingVersion int     `json:"processing_version"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

func (s *Server) createVideo(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	var payload createVideoRequest
	if err := decodeJSON(request, &payload); err != nil {
		writeAPIError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "The request body is invalid.")
		return
	}
	video, err := s.videos.Create(request.Context(), principal.ID, payload.Title, payload.OriginalFilename)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidVideo) {
			writeAPIError(writer, request, http.StatusBadRequest, "INVALID_VIDEO", err.Error())
			return
		}
		writeAPIError(writer, request, http.StatusInternalServerError, "VIDEO_CREATE_FAILED", "The video could not be created.")
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"data": toVideoResponse(video)})
}

func (s *Server) listVideos(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	limit := 20
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > 100 {
			writeAPIError(writer, request, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100.")
			return
		}
	}
	page, nextCursor, err := s.videos.List(request.Context(), principal.ID, limit, request.URL.Query().Get("cursor"))
	if err != nil {
		if errors.Is(err, application.ErrInvalidCursor) || strings.Contains(err.Error(), "limit must") {
			writeAPIError(writer, request, http.StatusBadRequest, "INVALID_CURSOR", "The cursor is invalid.")
			return
		}
		writeAPIError(writer, request, http.StatusInternalServerError, "VIDEO_LIST_FAILED", "The videos could not be loaded.")
		return
	}
	items := make([]videoResponse, 0, len(page.Items))
	for _, video := range page.Items {
		items = append(items, toVideoResponse(video))
	}
	var cursor *string
	if nextCursor != "" {
		cursor = &nextCursor
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{"items": items, "next_cursor": cursor}})
}

func (s *Server) getVideo(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	videoID := request.PathValue("videoId")
	if !ids.IsUUID(videoID) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return
	}
	video, err := s.videos.Get(request.Context(), principal.ID, videoID)
	if errors.Is(err, application.ErrNotFound) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return
	}
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "VIDEO_GET_FAILED", "The video could not be loaded.")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": toVideoResponse(video)})
}

func (s *Server) deleteVideo(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	videoID := request.PathValue("videoId")
	if !ids.IsUUID(videoID) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return
	}
	err = s.videos.Delete(request.Context(), principal.ID, videoID)
	if errors.Is(err, application.ErrNotFound) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return
	}
	if errors.Is(err, domain.ErrInvalidState) {
		writeAPIError(writer, request, http.StatusConflict, "INVALID_VIDEO_STATE", "The video cannot be deleted in its current state.")
		return
	}
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "VIDEO_DELETE_FAILED", "The video could not be deleted.")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body contains multiple JSON values")
	}
	return nil
}

func toVideoResponse(video domain.Video) videoResponse {
	return videoResponse{
		ID:                video.ID,
		Title:             video.Title,
		OriginalFilename:  video.OriginalFilename,
		Status:            string(video.Status),
		ProcessingVersion: video.ProcessingVersion,
		CreatedAt:         video.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:         video.UpdatedAt.Format(time.RFC3339Nano),
	}
}
