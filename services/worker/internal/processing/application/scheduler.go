package application

import (
	"context"
	"encoding/json"
	"errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"time"
	"video/services/worker/internal/platform/observability"

	"video/services/worker/internal/processing/domain"
	"video/services/worker/internal/processing/ports"
)

const (
	pollInterval      = 5 * time.Second
	heartbeatInterval = 15 * time.Second
	cleanupAge        = 24 * time.Hour
)

// Claiming stops on claimCtx cancellation, while an already claimed job keeps
// its workCtx (and heartbeat) until it finishes or the shutdown drain expires.
func (service *Service) RunScheduler(claimCtx, workCtx context.Context, logger *slog.Logger) {
	for claimCtx.Err() == nil {
		if err := service.runDue(claimCtx, workCtx, logger); err != nil && claimCtx.Err() == nil {
			logger.Error("processing scheduler iteration failed", "error", err)
		}
		select {
		case <-claimCtx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

func (service *Service) runDue(claimCtx, workCtx context.Context, logger *slog.Logger) error {
	now := service.clock.Now()
	if err := service.reliable.RecoverExpired(claimCtx, now); err != nil {
		return err
	}
	if claimCtx.Err() != nil {
		return nil
	}
	job, claimed, err := service.reliable.ClaimNext(claimCtx, now)
	if err != nil || !claimed {
		return err
	}
	var event eventEnvelope
	_ = json.Unmarshal(job.OriginalMessage, &event)
	logger = logger.With("event_id", event.MessageID, "correlation_id", event.CorrelationID, "job_id", job.ID, "video_id", job.VideoID, "run_id", job.RunID)
	workCtx = observability.Extract(workCtx, event.Traceparent)
	workCtx, span := otel.Tracer("video/worker").Start(workCtx, "processing.run")
	defer span.End()
	workCtx = context.WithValue(workCtx, operationLoggerKey{}, logger)
	started := time.Now()
	logger.Info("processing job claimed", "attempt", job.Attempt)
	err = service.runClaimed(workCtx, job)
	logger.Info("processing run ended", "duration_ms", time.Since(started).Milliseconds(), "error", err)
	return err
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

type operationLoggerKey struct{}

func logOperation(ctx context.Context, operation string, started time.Time, err error) {
	logger, _ := ctx.Value(operationLoggerKey{}).(*slog.Logger)
	if logger == nil {
		return
	}
	_, span := otel.Tracer("video/worker").Start(ctx, operation, trace.WithTimestamp(started))
	if err != nil {
		span.SetStatus(codes.Error, "operation failed")
	}
	span.End()
	result := "ok"
	if err != nil {
		result = "error"
	}
	logger.Info("processing operation", "operation", operation, "duration_ms", time.Since(started).Milliseconds(), "result", result)
}
