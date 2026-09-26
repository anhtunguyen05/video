package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"video/services/api/internal/media/application"
	mediaPostgres "video/services/api/internal/media/infrastructure/postgres"
	mediaPorts "video/services/api/internal/media/ports"
	platformauth "video/services/api/internal/platform/auth"
	"video/services/api/internal/platform/ids"
	"video/services/api/internal/platform/postgres"
)

type Server struct {
	db                 postgres.DB
	videos             VideoService
	uploads            UploadService
	currentUser        platformauth.CurrentUserProvider
	thumbnailSigner    mediaPorts.ThumbnailURLSigner
	thumbnailURLExpiry time.Duration
}

func NewServer(db postgres.DB) *http.Server {
	return NewServerWithDependencies(
		db,
		application.NewService(mediaPostgres.NewRepository(db)),
		platformauth.StaticPrincipal{ID: getenv("DEV_USER_ID", "00000000-0000-4000-8000-000000000001")},
	)
}

func NewServerWithDependencies(db postgres.DB, videos VideoService, currentUser platformauth.CurrentUserProvider) *http.Server {
	return NewServerWithUploadDependencies(db, videos, nil, currentUser)
}

func NewServerWithUploadDependencies(db postgres.DB, videos VideoService, uploads UploadService, currentUser platformauth.CurrentUserProvider) *http.Server {
	return newServer(db, videos, uploads, currentUser, nil, 15*time.Minute)
}

func NewServerWithThumbnailDependencies(db postgres.DB, videos VideoService, uploads UploadService, currentUser platformauth.CurrentUserProvider, thumbnailSigner mediaPorts.ThumbnailURLSigner, thumbnailURLExpiry time.Duration) *http.Server {
	return newServer(db, videos, uploads, currentUser, thumbnailSigner, thumbnailURLExpiry)
}

func newServer(db postgres.DB, videos VideoService, uploads UploadService, currentUser platformauth.CurrentUserProvider, thumbnailSigner mediaPorts.ThumbnailURLSigner, thumbnailURLExpiry time.Duration) *http.Server {
	if videos == nil {
		videos = application.NewService(mediaPostgres.NewRepository(db))
	}
	if thumbnailURLExpiry <= 0 {
		thumbnailURLExpiry = 15 * time.Minute
	}
	api := &Server{db: db, videos: videos, uploads: uploads, currentUser: currentUser, thumbnailSigner: thumbnailSigner, thumbnailURLExpiry: thumbnailURLExpiry}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", api.live)
	mux.HandleFunc("GET /health/ready", api.ready)
	mux.HandleFunc("POST /api/v1/videos", api.createVideo)
	mux.HandleFunc("GET /api/v1/videos", api.listVideos)
	mux.HandleFunc("GET /api/v1/videos/{videoId}", api.getVideo)
	mux.HandleFunc("DELETE /api/v1/videos/{videoId}", api.deleteVideo)
	mux.HandleFunc("POST /api/v1/videos/{videoId}/uploads", api.createUpload)
	mux.HandleFunc("POST /api/v1/uploads/{uploadId}/complete", api.completeUpload)
	mux.HandleFunc("POST /api/v1/uploads/{uploadId}/abort", api.abortUpload)
	return &http.Server{
		Handler:           withRequestID(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-ID")
		if !ids.IsUUID(requestID) {
			requestID, _ = ids.NewUUID()
		}
		writer.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) live(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(writer http.ResponseWriter, request *http.Request) {
	if s.db == nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeAPIError(writer http.ResponseWriter, _ *http.Request, status int, code, message string) {
	writeJSON(writer, status, map[string]any{
		"error": map[string]string{
			"code":       code,
			"message":    message,
			"request_id": writer.Header().Get("X-Request-ID"),
		},
	})
}
