package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"

	"video/services/api/internal/config"
	"video/services/api/internal/platform/logging"
	"video/services/api/internal/platform/postgres"
	"video/services/api/internal/transport/httpapi"
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

	server := httpapi.NewServer(db)
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
