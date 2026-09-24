package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"video/services/api/internal/config"
	mediaapplication "video/services/api/internal/media/application"
	mediaPostgres "video/services/api/internal/media/infrastructure/postgres"
	platformauth "video/services/api/internal/platform/auth"
	"video/services/api/internal/platform/logging"
	"video/services/api/internal/platform/postgres"
	"video/services/api/internal/transport/httpapi"
	uploadapplication "video/services/api/internal/upload/application"
	uploadPostgres "video/services/api/internal/upload/infrastructure/postgres"
	uploadS3 "video/services/api/internal/upload/infrastructure/s3"
)

func main() {
	logger := logging.New()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		return
	}
	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		return
	}
	defer db.Close()

	storage, err := uploadS3.New(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UsePathStyle)
	if err != nil {
		logger.Error("create object storage client", "error", err)
		return
	}
	storageCtx, storageCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := storage.EnsureBucket(storageCtx); err != nil {
		storageCancel()
		logger.Error("ensure object storage bucket", "bucket", cfg.S3Bucket, "error", err)
		return
	}
	storageCancel()

	videoService := mediaapplication.NewService(mediaPostgres.NewRepository(db))
	allowedTypes := make(map[string]struct{}, len(cfg.AllowedUploadMimeTypes))
	for _, contentType := range cfg.AllowedUploadMimeTypes {
		allowedTypes[contentType] = struct{}{}
	}
	uploadService := uploadapplication.NewService(
		uploadPostgres.NewRepository(db),
		videoService,
		storage,
		uploadapplication.Policy{
			MaxSizeBytes: cfg.MaxUploadSizeBytes,
			AllowedTypes: allowedTypes,
			URLExpiry:    cfg.UploadURLExpiry,
		},
	)
	server := httpapi.NewServerWithUploadDependencies(db, videoService, uploadService, platformauth.StaticPrincipal{ID: cfg.DevUserID})
	server.Addr = cfg.HTTPAddr
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("api started", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
		serverErr <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped unexpectedly", "error", err)
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("api graceful shutdown failed", "error", err)
		}
	}
	logger.Info("api stopped")
}
