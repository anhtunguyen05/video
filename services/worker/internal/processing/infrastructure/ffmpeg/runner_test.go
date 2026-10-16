package ffmpeg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"video/services/worker/internal/processing/ports"
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

func TestRunnerTranscodeBuildsPlayableMP4Command(t *testing.T) {
	command := &fakeCommandRunner{}
	runner := NewTranscoderWithCommand("ffmpeg-test", TranscodeProfile{
		VideoCodec: "libx264",
		AudioCodec: "aac",
		Preset:     "slow",
		CRF:        21,
	}, command)

	err := runner.Transcode(context.Background(), ports.RenditionTranscodeRequest{
		SourcePath: "source.mp4",
		OutputPath: "360p.mp4",
		Width:      640,
		Height:     360,
	})
	if err != nil {
		t.Fatalf("Transcode() error = %v", err)
	}
	if command.path != "ffmpeg-test" {
		t.Fatalf("path = %q, want ffmpeg-test", command.path)
	}
	joined := strings.Join(command.args, " ")
	for _, expected := range []string{
		"-i source.mp4", "-map 0:v:0", "-map 0:a:0?", "-vf scale=640:360",
		"-c:v libx264", "-preset slow", "-crf 21", "-pix_fmt yuv420p",
		"-c:a aac", "-movflags +faststart", "360p.mp4",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("args = %v, missing %q", command.args, expected)
		}
	}
}

func TestRunnerTranscodeRejectsInvalidDimensions(t *testing.T) {
	runner := NewTranscoderWithCommand("ffmpeg", TranscodeProfile{}, &fakeCommandRunner{})
	err := runner.Transcode(context.Background(), ports.RenditionTranscodeRequest{
		SourcePath: "source.mp4",
		OutputPath: "output.mp4",
		Width:      641,
		Height:     360,
	})
	if err == nil || !strings.Contains(err.Error(), "positive even") {
		t.Fatalf("Transcode() error = %v", err)
	}
}

func TestRunnerTranscodeReturnsCommandFailure(t *testing.T) {
	runner := NewTranscoderWithCommand("ffmpeg", TranscodeProfile{}, &fakeCommandRunner{output: []byte("encoder unavailable"), err: errors.New("exit status 1")})
	err := runner.Transcode(context.Background(), ports.RenditionTranscodeRequest{
		SourcePath: "source.mp4",
		OutputPath: "output.mp4",
		Width:      640,
		Height:     360,
	})
	if err == nil || !strings.Contains(err.Error(), "ffmpeg transcode failed: encoder unavailable") {
		t.Fatalf("Transcode() error = %v", err)
	}
}
