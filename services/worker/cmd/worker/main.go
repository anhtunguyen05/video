package main

import (
	"context"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"video/services/worker/internal/config"
	"video/services/worker/internal/platform/logging"
	"video/services/worker/internal/platform/postgres"
	"video/services/worker/internal/platform/rabbitmq"
	processingapplication "video/services/worker/internal/processing/application"
	"video/services/worker/internal/processing/infrastructure/ffmpeg"
	"video/services/worker/internal/processing/infrastructure/ffprobe"
	processingPostgres "video/services/worker/internal/processing/infrastructure/postgres"
	processingS3 "video/services/worker/internal/processing/infrastructure/s3"
)

func main() {
	logger := logging.New()
	cfg := config.Load()
	if _, err := exec.LookPath(cfg.FFProbePath); err != nil {
		logger.Error("ffprobe executable is unavailable", "path", cfg.FFProbePath, "error", err)
		return
	}
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		logger.Error("ffmpeg executable is unavailable", "path", cfg.FFmpegPath, "error", err)
		return
	}
	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		return
	}
	defer db.Close()

	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("create RabbitMQ consumer", "error", err)
		return
	}
	defer consumer.Close()
	deadPublisher, err := rabbitmq.NewDeadPublisher(cfg.RabbitMQURL)
	if err != nil {
		logger.Error("create dead-letter publisher", "error", err)
		return
	}
	defer deadPublisher.Close()
	processingRepository := processingPostgres.NewRepository(db)
	storage, err := processingS3.New(
		cfg.S3Endpoint,
		cfg.S3Region,
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3Bucket,
		cfg.S3UsePathStyle,
	)
	if err != nil {
		logger.Error("create object storage client", "error", err)
		return
	}
	transcoder := ffmpeg.NewTranscoder(cfg.FFmpegPath, ffmpeg.TranscodeProfile{
		VideoCodec: cfg.TranscodeVideoCodec,
		AudioCodec: cfg.TranscodeAudioCodec,
		Preset:     cfg.TranscodePreset,
		CRF:        cfg.TranscodeCRF,
	})
	processingService := processingapplication.NewReliableProcessingService(
		processingRepository,
		processingRepository,
		processingRepository,
		storage,
		storage,
		ffprobe.NewRunner(cfg.FFProbePath),
		ffmpeg.NewRunner(cfg.FFmpegPath, cfg.ThumbnailMaxEdge),
		transcoder,
		transcoder,
		cfg.ProcessingTempDir,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		processingService.RunScheduler(ctx, logger)
	}()
	go processingService.RunCleanup(ctx, logger)
	go func() {
		if err := deadPublisher.Run(ctx, db); err != nil {
			logger.Error("dead-letter outbox stopped", "error", err)
			stop()
		}
	}()
	logger.Info("worker started", "env", cfg.AppEnv, "queue", rabbitmq.QueueName)
	if err := consumer.Run(ctx, processingService); err != nil {
		logger.Error("worker consumer stopped", "error", err)
	}
	stop()
	logger.Info("shutdown signal received")
	select {
	case <-schedulerDone:
	case <-time.After(cfg.ShutdownTimeout):
		logger.Error("processing job did not stop before shutdown deadline")
	}
	logger.Info("worker stopped")
}
