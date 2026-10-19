package application

import (
	"context"
	"errors"
	"testing"

	"video/services/worker/internal/processing/ports"
)

func TestClassifyProcessingError(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		code  string
		retry bool
	}{
		{"storage timeout", context.DeadlineExceeded, "PROCESS_INTERRUPTED", true},
		{"worker shutdown", context.Canceled, "PROCESS_INTERRUPTED", true},
		{"credential failure", errors.Join(errors.New("upload"), ports.ErrInvalidConfiguration), "INVALID_PROCESSING_CONFIGURATION", false},
		{"temporary dependency", errors.New("storage unavailable"), "PROCESS_TEMPORARY_ERROR", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, retry := classifyProcessingError(tc.err)
			if code != tc.code || retry != tc.retry {
				t.Fatalf("classify = %s/%v, want %s/%v", code, retry, tc.code, tc.retry)
			}
		})
	}
}
