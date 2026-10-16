package httpapi

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"video/services/api/internal/media/application"
	"video/services/api/internal/media/domain"
	"video/services/api/internal/platform/ids"
)

type playbackResponse struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

func (s *Server) getPlayback(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	video, ok := s.ownedReadyVideo(writer, request, principal.ID)
	if !ok {
		return
	}
	if video.HLSMaster == nil || s.assetReader == nil || s.assetSigner == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "PLAYBACK_UNAVAILABLE", "HLS playback is not available.")
		return
	}
	url := fmt.Sprintf("/api/v1/videos/%s/playback/hls/master.m3u8", video.ID)
	writeJSON(writer, http.StatusOK, map[string]any{"data": playbackResponse{
		Type: "hls", URL: url, ExpiresAt: time.Now().UTC().Add(s.assetURLExpiry).Format(time.RFC3339Nano),
	}})
}

func (s *Server) getHLSManifest(writer http.ResponseWriter, request *http.Request) {
	principal, err := s.currentUser.CurrentUser(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required.")
		return
	}
	video, ok := s.ownedReadyVideo(writer, request, principal.ID)
	if !ok {
		return
	}
	if s.assetReader == nil || s.assetSigner == nil {
		writeAPIError(writer, request, http.StatusServiceUnavailable, "PLAYBACK_UNAVAILABLE", "HLS playback is not available.")
		return
	}
	manifestPath := filepath.ToSlash(request.PathValue("manifestPath"))
	var objectKey string
	master := manifestPath == "master.m3u8"
	if master {
		if video.HLSMaster == nil {
			writeAPIError(writer, request, http.StatusNotFound, "PLAYBACK_NOT_FOUND", "Playback was not found.")
			return
		}
		objectKey = video.HLSMaster.ObjectKey
	} else {
		if !safeManifestPath(manifestPath) {
			writeAPIError(writer, request, http.StatusNotFound, "PLAYBACK_NOT_FOUND", "Playback was not found.")
			return
		}
		variant := strings.Split(manifestPath, "/")[0]
		if len(strings.Split(manifestPath, "/")) != 2 || strings.Split(manifestPath, "/")[1] != "index.m3u8" {
			writeAPIError(writer, request, http.StatusNotFound, "PLAYBACK_NOT_FOUND", "Playback was not found.")
			return
		}
		for _, asset := range video.HLSVariants {
			if asset.Variant == variant {
				objectKey = asset.ObjectKey
				break
			}
		}
		if objectKey == "" {
			writeAPIError(writer, request, http.StatusNotFound, "PLAYBACK_NOT_FOUND", "Playback was not found.")
			return
		}
	}
	contents, err := s.assetReader.ReadAsset(request.Context(), objectKey)
	if err != nil {
		writeAPIError(writer, request, http.StatusNotFound, "PLAYBACK_NOT_FOUND", "Playback was not found.")
		return
	}
	rewritten, err := s.rewriteHLSManifest(request.Context(), video, contents, objectKey, master)
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "PLAYBACK_MANIFEST_FAILED", "The playback manifest could not be prepared.")
		return
	}
	writer.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(rewritten)
}

func (s *Server) ownedReadyVideo(writer http.ResponseWriter, request *http.Request, ownerID string) (domain.Video, bool) {
	videoID := request.PathValue("videoId")
	if !ids.IsUUID(videoID) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return domain.Video{}, false
	}
	video, err := s.videos.Get(request.Context(), ownerID, videoID)
	if errors.Is(err, application.ErrNotFound) {
		writeAPIError(writer, request, http.StatusNotFound, "VIDEO_NOT_FOUND", "Video was not found.")
		return domain.Video{}, false
	}
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "VIDEO_GET_FAILED", "The video could not be loaded.")
		return domain.Video{}, false
	}
	if video.Status != domain.StatusReady {
		writeAPIError(writer, request, http.StatusConflict, "VIDEO_NOT_READY", "The video is not ready for playback.")
		return domain.Video{}, false
	}
	return video, true
}

func safeManifestPath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.Contains(value, "..") || path.IsAbs(value) {
		return false
	}
	return true
}

func (s *Server) rewriteHLSManifest(ctx context.Context, video domain.Video, contents []byte, objectKey string, master bool) ([]byte, error) {
	var output strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			output.WriteString(line)
			output.WriteByte('\n')
			continue
		}
		if master {
			if !safeManifestPath(trimmed) || path.Base(trimmed) != "index.m3u8" {
				return nil, errors.New("master playlist contains an invalid variant path")
			}
			variant := path.Base(path.Dir(trimmed))
			found := false
			for _, asset := range video.HLSVariants {
				if asset.Variant == variant {
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("master playlist references an unknown variant")
			}
			fmt.Fprintf(&output, "%s\n", variant+"/index.m3u8")
			continue
		}
		if !safeManifestPath(trimmed) || path.Base(trimmed) != trimmed {
			return nil, errors.New("variant playlist contains an invalid segment path")
		}
		segmentKey := path.Join(path.Dir(objectKey), trimmed)
		url, _, err := s.assetSigner.PresignAsset(ctx, segmentKey, s.assetURLExpiry)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "%s\n", url)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}
