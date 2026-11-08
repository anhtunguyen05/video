package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"video/services/api/internal/media/application"
	"video/services/api/internal/media/domain"
	"video/services/api/internal/platform/ids"
	"video/services/api/internal/platform/postgres"
)

type subtitleProjection struct {
	Status    string
	ObjectKey sql.NullString
}

type subtitleLookup interface {
	Get(context.Context, string, string, int) (subtitleProjection, error)
}

type postgresSubtitleLookup struct{ db postgres.DB }

func (p postgresSubtitleLookup) Get(ctx context.Context, ownerID, videoID string, sourceVersion int) (subtitleProjection, error) {
	var result subtitleProjection
	err := p.db.QueryRowContext(ctx, `SELECT s.status, s.output_object_key
		FROM subtitle_requests s JOIN videos v ON v.id = s.video_id
		WHERE s.video_id = $1 AND s.owner_id = $2 AND v.owner_id = $2
		  AND v.deleted_at IS NULL AND s.source_version = $3 AND v.processing_version = $3 AND s.language = 'vi'`,
		videoID, ownerID, sourceVersion).Scan(&result.Status, &result.ObjectKey)
	return result, err
}

func (s *Server) ownedSubtitle(writer http.ResponseWriter, request *http.Request) (domain.Video, subtitleProjection, bool) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return domain.Video{}, subtitleProjection{}, false
	}
	videoID := request.PathValue("videoId")
	if !ids.IsUUID(videoID) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return domain.Video{}, subtitleProjection{}, false
	}
	video, err := s.videos.Get(request.Context(), principal.ID, videoID)
	if errors.Is(err, application.ErrNotFound) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return domain.Video{}, subtitleProjection{}, false
	}
	if err != nil || s.subtitles == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "SUBTITLES_UNAVAILABLE", "Subtitle status is unavailable.")
		return domain.Video{}, subtitleProjection{}, false
	}
	projection, err := s.subtitles.Get(request.Context(), principal.ID, videoID, video.ProcessingVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return video, subtitleProjection{Status: "NOT_REQUESTED"}, true
	}
	if err != nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "SUBTITLES_UNAVAILABLE", "Subtitle status is unavailable.")
		return domain.Video{}, subtitleProjection{}, false
	}
	return video, projection, true
}

func (s *Server) getSubtitles(writer http.ResponseWriter, request *http.Request) {
	video, projection, ok := s.ownedSubtitle(writer, request)
	if !ok {
		return
	}
	data := map[string]any{"status": projection.Status, "language": "vi"}
	if projection.Status == "READY" && projection.ObjectKey.Valid {
		data["track_url"] = fmt.Sprintf("/api/v1/videos/%s/subtitles/vi.vtt", video.ID)
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writeJSON(writer, http.StatusOK, map[string]any{"data": data})
}

func (s *Server) getSubtitleTrack(writer http.ResponseWriter, request *http.Request) {
	_, projection, ok := s.ownedSubtitle(writer, request)
	if !ok {
		return
	}
	if projection.Status != "READY" || !projection.ObjectKey.Valid || s.assetReader == nil {
		writeAPIError(writer, request, http.StatusNotFound, "SUBTITLE_NOT_FOUND", "Subtitle track was not found.")
		return
	}
	track, err := s.assetReader.ReadAsset(request.Context(), projection.ObjectKey.String)
	if err != nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "SUBTITLES_UNAVAILABLE", "Subtitle track is unavailable.")
		return
	}
	writer.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = writer.Write(track)
}
