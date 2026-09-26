package ffmpeg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeCommandRunner struct {
	output []byte
	err    error
	path   string
	args   []string
}

func (runner *fakeCommandRunner) Run(_ context.Context, path string, args ...string) ([]byte, error) {
	runner.path = path
	runner.args = args
	return runner.output, runner.err
}

func TestRunnerGenerateBuildsThumbnailCommand(t *testing.T) {
	command := &fakeCommandRunner{}
	runner := NewRunnerWithCommand("ffmpeg-test", 640, command)

	if err := runner.Generate(context.Background(), "source.mp4", "thumbnail.jpg", 1500*time.Millisecond); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if command.path != "ffmpeg-test" {
		t.Fatalf("path = %q, want ffmpeg-test", command.path)
	}
	joined := strings.Join(command.args, " ")
	for _, expected := range []string{"-i source.mp4", "-ss 1.500", "-frames:v 1", "-q:v 2", "thumbnail.jpg"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("args = %v, missing %q", command.args, expected)
		}
	}
	if !strings.Contains(joined, "min(640,iw)") || !strings.Contains(joined, "min(640,ih)") {
		t.Fatalf("args = %v, expected bounded aspect-ratio filter", command.args)
	}
}

func TestRunnerGenerateReturnsCommandFailure(t *testing.T) {
	runner := NewRunnerWithCommand("ffmpeg", 640, &fakeCommandRunner{output: []byte("invalid media"), err: errors.New("exit status 1")})

	err := runner.Generate(context.Background(), "source.mp4", "thumbnail.jpg", 0)
	if err == nil || !strings.Contains(err.Error(), "ffmpeg thumbnail failed: invalid media") {
		t.Fatalf("Generate() error = %v", err)
	}
}
