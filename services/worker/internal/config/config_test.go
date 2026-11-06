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

func TestWorkerUploadConsumerConfiguration(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", true}, {"true", true}, {"false", false}, {"invalid", true}} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("WORKER_CONSUME_UPLOADS", tc.value)
			if got := Load().ConsumeUploads; got != tc.want {
				t.Fatalf("ConsumeUploads = %v, want %v", got, tc.want)
			}
		})
	}
}
