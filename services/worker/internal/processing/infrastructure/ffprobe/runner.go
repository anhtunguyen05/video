package ffprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"

	"video/services/worker/internal/processing/ports"
)

var ErrInvalidMetadata = ports.ErrInvalidMedia

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

type Runner struct {
	path string
	run  CommandRunner
}

func NewRunner(path string) *Runner {
	if strings.TrimSpace(path) == "" {
		path = "ffprobe"
	}
	return &Runner{path: path, run: commandRunner{}}
}

func NewRunnerWithCommand(path string, run CommandRunner) *Runner {
	if strings.TrimSpace(path) == "" {
		path = "ffprobe"
	}
	return &Runner{path: path, run: run}
}

type probeOutput struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeStream struct {
	CodecType  string `json:"codec_type"`
	CodecName  string `json:"codec_name"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	RFrameRate string `json:"r_frame_rate"`
	AvgRate    string `json:"avg_frame_rate"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
}

func (runner *Runner) Probe(ctx context.Context, path string) (ports.VideoMetadata, error) {
	output, err := runner.run.Run(ctx, runner.path, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return ports.VideoMetadata{}, fmt.Errorf("%w: ffprobe failed: %s", ports.ErrInvalidMedia, message)
	}

	var parsed probeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return ports.VideoMetadata{}, fmt.Errorf("%w: decode ffprobe output: %v", ErrInvalidMetadata, err)
	}

	videoStream, ok := firstVideoStream(parsed.Streams)
	if !ok {
		return ports.VideoMetadata{}, fmt.Errorf("%w: video stream is missing", ErrInvalidMetadata)
	}
	if videoStream.Width <= 0 || videoStream.Height <= 0 {
		return ports.VideoMetadata{}, fmt.Errorf("%w: video dimensions are missing", ErrInvalidMetadata)
	}
	if strings.TrimSpace(videoStream.CodecName) == "" {
		return ports.VideoMetadata{}, fmt.Errorf("%w: video codec is missing", ErrInvalidMetadata)
	}

	durationMS, err := parseDurationMS(parsed.Format.Duration)
	if err != nil {
		return ports.VideoMetadata{}, fmt.Errorf("%w: %v", ErrInvalidMetadata, err)
	}
	container := normalizeContainer(parsed.Format.FormatName)
	if container == "" {
		return ports.VideoMetadata{}, fmt.Errorf("%w: container is missing", ErrInvalidMetadata)
	}

	return ports.VideoMetadata{
		DurationMS: durationMS,
		Width:      videoStream.Width,
		Height:     videoStream.Height,
		Codec:      videoStream.CodecName,
		Container:  container,
		FrameRate:  parseFrameRate(videoStream.AvgRate, videoStream.RFrameRate),
	}, nil
}

func firstVideoStream(streams []probeStream) (probeStream, bool) {
	for _, stream := range streams {
		if stream.CodecType == "video" {
			return stream, true
		}
	}
	return probeStream{}, false
}

func parseDurationMS(raw string) (int64, error) {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, fmt.Errorf("duration is invalid")
	}
	return int64(math.Round(seconds * 1000)), nil
}

func normalizeContainer(raw string) string {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func parseFrameRate(values ...string) *float64 {
	for _, raw := range values {
		parts := strings.Split(strings.TrimSpace(raw), "/")
		if len(parts) != 2 {
			continue
		}
		numerator, numeratorErr := strconv.ParseFloat(parts[0], 64)
		denominator, denominatorErr := strconv.ParseFloat(parts[1], 64)
		if numeratorErr != nil || denominatorErr != nil || denominator <= 0 || numerator <= 0 {
			continue
		}
		rate := numerator / denominator
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
			continue
		}
		return &rate
	}
	return nil
}
