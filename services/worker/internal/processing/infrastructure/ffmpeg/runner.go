package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"video/services/worker/internal/processing/ports"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

type Runner struct {
	path    string
	maxEdge int
	profile TranscodeProfile
	run     CommandRunner
}

type TranscodeProfile struct {
	VideoCodec string
	AudioCodec string
	Preset     string
	CRF        int
}

func NewRunner(path string, maxEdge int) *Runner {
	return newRunner(path, maxEdge, defaultTranscodeProfile(), commandRunner{})
}

func NewRunnerWithCommand(path string, maxEdge int, run CommandRunner) *Runner {
	return newRunner(path, maxEdge, defaultTranscodeProfile(), run)
}

func NewTranscoder(path string, profile TranscodeProfile) *Runner {
	return newRunner(path, 640, profile, commandRunner{})
}

func NewTranscoderWithCommand(path string, profile TranscodeProfile, run CommandRunner) *Runner {
	return newRunner(path, 640, profile, run)
}

func newRunner(path string, maxEdge int, profile TranscodeProfile, run CommandRunner) *Runner {
	if strings.TrimSpace(path) == "" {
		path = "ffmpeg"
	}
	if maxEdge <= 0 {
		maxEdge = 640
	}
	return &Runner{path: path, maxEdge: maxEdge, profile: normalizeTranscodeProfile(profile), run: run}
}

func defaultTranscodeProfile() TranscodeProfile {
	return TranscodeProfile{VideoCodec: "libx264", AudioCodec: "aac", Preset: "medium", CRF: 23}
}

func normalizeTranscodeProfile(profile TranscodeProfile) TranscodeProfile {
	defaults := defaultTranscodeProfile()
	if strings.TrimSpace(profile.VideoCodec) == "" {
		profile.VideoCodec = defaults.VideoCodec
	}
	if strings.TrimSpace(profile.AudioCodec) == "" {
		profile.AudioCodec = defaults.AudioCodec
	}
	if strings.TrimSpace(profile.Preset) == "" {
		profile.Preset = defaults.Preset
	}
	if profile.CRF < 0 || profile.CRF > 51 {
		profile.CRF = defaults.CRF
	}
	return profile
}

func (runner *Runner) Generate(ctx context.Context, sourcePath, outputPath string, timestamp time.Duration) error {
	if strings.TrimSpace(sourcePath) == "" || strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("source and output paths are required")
	}
	seconds := strconv.FormatFloat(timestamp.Seconds(), 'f', 3, 64)
	filter := fmt.Sprintf("scale='if(gt(iw,ih),min(%d,iw),-2)':'if(gt(iw,ih),-2,min(%d,ih))'", runner.maxEdge, runner.maxEdge)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", sourcePath,
		"-ss", seconds,
		"-frames:v", "1",
		"-vf", filter,
		"-q:v", "2",
		outputPath,
	}
	output, err := runner.run.Run(ctx, runner.path, args...)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffmpeg thumbnail failed: %s", message)
	}
	return nil
}

func (runner *Runner) Transcode(ctx context.Context, request ports.RenditionTranscodeRequest) error {
	if strings.TrimSpace(request.SourcePath) == "" || strings.TrimSpace(request.OutputPath) == "" {
		return fmt.Errorf("source and output paths are required")
	}
	if request.Width <= 0 || request.Height <= 0 || request.Width%2 != 0 || request.Height%2 != 0 {
		return fmt.Errorf("rendition dimensions must be positive even values")
	}
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", request.SourcePath,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-vf", fmt.Sprintf("scale=%d:%d", request.Width, request.Height),
		"-c:v", runner.profile.VideoCodec,
		"-preset", runner.profile.Preset,
		"-crf", strconv.Itoa(runner.profile.CRF),
		"-pix_fmt", "yuv420p",
		"-c:a", runner.profile.AudioCodec,
		"-movflags", "+faststart",
		request.OutputPath,
	}
	output, err := runner.run.Run(ctx, runner.path, args...)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffmpeg transcode failed: %s", message)
	}
	return nil
}

func (runner *Runner) PackageVariant(ctx context.Context, request ports.HLSVariantRequest) error {
	if strings.TrimSpace(request.InputPath) == "" || strings.TrimSpace(request.OutputDir) == "" {
		return fmt.Errorf("input and output paths are required")
	}
	playlistPath := request.OutputDir + "/index.m3u8"
	segmentPattern := request.OutputDir + "/segment_%05d.ts"
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", request.InputPath,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c", "copy",
		"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_segment_type", "mpegts", "-hls_segment_filename", segmentPattern,
		playlistPath,
	}
	output, err := runner.run.Run(ctx, runner.path, args...)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("ffmpeg HLS packaging failed: %s", message)
	}
	return nil
}

var _ ports.ThumbnailGenerator = (*Runner)(nil)
var _ ports.RenditionTranscoder = (*Runner)(nil)
var _ ports.HLSVariantPackager = (*Runner)(nil)
