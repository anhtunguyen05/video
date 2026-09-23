package main

import (
	"context"
	"os/signal"
	"syscall"

	"video/services/worker/internal/config"
	"video/services/worker/internal/platform/logging"
	"video/services/worker/internal/platform/postgres"
)

func main() {
	logger := logging.New()
	cfg := config.Load()
	db, err := postgres.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		return
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker started", "env", cfg.AppEnv, "status", "idle")
	<-ctx.Done()
	logger.Info("shutdown signal received")
	logger.Info("worker stopped")
}
