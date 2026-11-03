package main

import (
	"context"
	"log/slog"
	"time"
)

// drainScheduler lets the active job finish while no new claims are made.
// After the drain deadline, cancellation lets runClaimed persist a bounded
// retry with its independent completion context.
func drainScheduler(done <-chan struct{}, cancelWork context.CancelFunc, drainTimeout, settleTimeout time.Duration, logger *slog.Logger) bool {
	timer := time.NewTimer(drainTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		logger.Warn("processing drain deadline reached; canceling active job")
		cancelWork()
	}
	settle := time.NewTimer(settleTimeout)
	defer settle.Stop()
	select {
	case <-done:
		return true
	case <-settle.C:
		logger.Error("processing job did not stop before shutdown deadline; lease recovery will retry it")
		return false
	}
}
