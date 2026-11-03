package config

import (
	"testing"
	"time"
)

func TestWorkerDrainNeverExceedsComposeGraceBudget(t *testing.T) {
	t.Setenv("WORKER_SHUTDOWN_TIMEOUT_SECONDS", "999")
	if got := Load().ShutdownTimeout; got != 120*time.Second {
		t.Fatalf("shutdown timeout = %s, want at most 120s", got)
	}
	t.Setenv("WORKER_SHUTDOWN_TIMEOUT_SECONDS", "5")
	if got := Load().ShutdownTimeout; got != 5*time.Second {
		t.Fatalf("short drain timeout = %s, want 5s", got)
	}
}
