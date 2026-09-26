package ffmpeg

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
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
	run     CommandRunner
}

func NewRunner(path string, maxEdge int) *Runner {
	return newRunner(path, maxEdge, commandRunner{})
}

func NewRunnerWithCommand(path string, maxEdge int, run CommandRunner) *Runner {
	return newRunner(path, maxEdge, run)
}

func newRunner(path string, maxEdge int, run CommandRunner) *Runner {
	if strings.TrimSpace(path) == "" {
		path = "ffmpeg"
	}
	if maxEdge <= 0 {
		maxEdge = 640
	}
	return &Runner{path: path, maxEdge: maxEdge, run: run}
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
