package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestDrainSchedulerAllowsActiveJobToFinish(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	close(done)
	if !drainScheduler(done, cancelWork, time.Second, time.Second, logger) {
		t.Fatal("completed scheduler did not drain")
	}
	if workCtx.Err() != nil {
		t.Fatal("drain canceled an already completed job")
	}
}

func TestDrainSchedulerCancelsOnlyAfterDeadlineAndWaitsForRetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	go func() {
		<-workCtx.Done()
		// The scheduler persists the retry before it closes done.
		close(done)
	}()
	if !drainScheduler(done, cancelWork, 20*time.Millisecond, time.Second, logger) {
		t.Fatal("scheduler did not finish after cancellation")
	}
	if workCtx.Err() != context.Canceled {
		t.Fatal("active job was not canceled at the drain deadline")
	}
}

func TestDrainSchedulerReportsUnsettledJob(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	workCtx, cancelWork := context.WithCancel(context.Background())
	defer cancelWork()
	if drainScheduler(done, cancelWork, time.Millisecond, time.Millisecond, logger) {
		t.Fatal("unsettled scheduler reported a successful drain")
	}
	if workCtx.Err() != context.Canceled {
		t.Fatal("unsettled job was not canceled")
	}
}
