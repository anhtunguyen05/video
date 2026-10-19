package application

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

const (
	pollInterval      = 5 * time.Second
	heartbeatInterval = 15 * time.Second
	cleanupAge        = 24 * time.Hour
)

func (service *Service) RunScheduler(ctx context.Context, logger *slog.Logger) {
	for ctx.Err() == nil {
		if err := service.runDue(ctx); err != nil && ctx.Err() == nil {
			logger.Error("processing scheduler iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

func (service *Service) runDue(ctx context.Context) error {
	now := service.clock.Now()
	if err := service.reliable.RecoverExpired(ctx, now); err != nil {
		return err
	}
	job, claimed, err := service.reliable.ClaimNext(ctx, now)
	if err != nil || !claimed {
		return err
	}
	return service.runClaimed(ctx, job)
}

func (service *Service) runClaimed(ctx context.Context, job domain.Job) error {
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-ticker.C:
				checkCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
				ok, err := service.reliable.RenewLease(checkCtx, job, service.clock.Now())
				stop()
				if err != nil || !ok {
					cancel()
					return
				}
			}
		}
	}()
	err := service.ProcessClaimed(workCtx, job)
	cancel()
	<-heartbeatDone
	if err == nil {
		return nil
	}
	code, retryable := classifyProcessingError(err)
	finishCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if !retryable {
		return service.processor.MarkFailed(finishCtx, job, code, "The processing configuration is invalid.", service.clock.Now())
	}
	return service.reliable.RetryOrFail(finishCtx, job, code, err.Error(), service.clock.Now())
}

func classifyProcessingError(err error) (string, bool) {
	if errors.Is(err, ports.ErrInvalidConfiguration) {
		return "INVALID_PROCESSING_CONFIGURATION", false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "PROCESS_INTERRUPTED", true
	}
	return "PROCESS_TEMPORARY_ERROR", true
}

func (service *Service) RunCleanup(ctx context.Context, logger *slog.Logger) {
	cleaner, ok := service.assets.(ports.RunCleaner)
	if !ok {
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		service.cleanupRuns(ctx, logger, cleaner)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (service *Service) cleanupRuns(ctx context.Context, logger *slog.Logger, cleaner ports.RunCleaner) {
	runs, err := service.reliable.CleanupCandidates(ctx, service.clock.Now().Add(-cleanupAge))
	if err != nil {
		if ctx.Err() == nil {
			logger.Error("list failed processing runs", "error", err)
		}
		return
	}
	for _, run := range runs {
		if err := cleaner.DeletePrefix(ctx, run.Prefix); err != nil {
			if ctx.Err() == nil {
				logger.Error("delete failed processing run", "run_id", run.ID, "error", err)
			}
			continue
		}
		if err := service.reliable.MarkRunClean(ctx, run.ID, service.clock.Now()); err != nil {
			logger.Error("mark failed processing run cleaned", "run_id", run.ID, "error", err)
		}
	}
}
