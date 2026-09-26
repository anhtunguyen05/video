package main

import (
	"context"
	"os/exec"
	"os/signal"
	"syscall"

	"video/services/worker/internal/config"
	"video/services/worker/internal/platform/logging"
	"video/services/worker/internal/platform/postgres"
	"video/services/worker/internal/platform/rabbitmq"
	processingapplication "video/services/worker/internal/processing/application"
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
	processingService := processingapplication.NewProcessingService(
		processingRepository,
		processingRepository,
		storage,
		ffprobe.NewRunner(cfg.FFProbePath),
		cfg.ProcessingTempDir,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker started", "env", cfg.AppEnv, "queue", rabbitmq.QueueName)
	if err := consumer.Run(ctx, processingService); err != nil {
		logger.Error("worker consumer stopped", "error", err)
	}
	logger.Info("shutdown signal received")
	logger.Info("worker stopped")
}
