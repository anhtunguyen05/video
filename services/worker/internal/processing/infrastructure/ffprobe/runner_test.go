package ffprobe

import (
	"context"
	"errors"
	"testing"
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

func TestRunnerProbeExtractsVideoMetadata(t *testing.T) {
	command := &fakeCommandRunner{output: []byte(`{
        "streams": [{"codec_type":"audio","codec_name":"aac"},{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"30000/1001","r_frame_rate":"0/0"}],
        "format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"302.004","size":"123456"}
    }`)}
	runner := NewRunnerWithCommand("ffprobe-test", command)

	metadata, err := runner.Probe(context.Background(), "source.mp4")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if metadata.DurationMS != 302004 || metadata.Width != 1920 || metadata.Height != 1080 || metadata.Codec != "h264" || metadata.Container != "mov" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if metadata.FrameRate == nil || *metadata.FrameRate < 29.96 || *metadata.FrameRate > 29.98 {
		t.Fatalf("unexpected frame rate: %#v", metadata.FrameRate)
	}
	if command.path != "ffprobe-test" || command.args[len(command.args)-1] != "source.mp4" {
		t.Fatalf("unexpected command: %s %#v", command.path, command.args)
	}
}

func TestRunnerProbeRejectsMissingVideoStream(t *testing.T) {
	runner := NewRunnerWithCommand("ffprobe", &fakeCommandRunner{output: []byte(`{"streams":[{"codec_type":"audio"}],"format":{"format_name":"mp4","duration":"1"}}`)})

	_, err := runner.Probe(context.Background(), "source.mp4")
	if !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("Probe() error = %v, want ErrInvalidMetadata", err)
	}
}

func TestRunnerProbeReturnsCommandFailure(t *testing.T) {
	runner := NewRunnerWithCommand("ffprobe", &fakeCommandRunner{output: []byte("Invalid data found"), err: errors.New("exit status 1")})

	_, err := runner.Probe(context.Background(), "source.mp4")
	if !errors.Is(err, ErrInvalidMetadata) || err.Error() != "invalid media: ffprobe failed: Invalid data found" {
		t.Fatalf("Probe() error = %v", err)
	}
}
