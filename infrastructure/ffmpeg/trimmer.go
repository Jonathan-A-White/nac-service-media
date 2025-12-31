package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"nac-service-media/domain/video"
)

// CommandRunner defines the interface for running external commands
// This allows mocking exec.Command in tests
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecCommandRunner is the production implementation using os/exec
type ExecCommandRunner struct{}

// Run executes a command and returns any error
func (r *ExecCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Output executes a command and returns its output
func (r *ExecCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// Trimmer implements video.Trimmer using ffmpeg
type Trimmer struct {
	ffmpegPath string
	runner     CommandRunner
}

// TrimmerOption is a functional option for configuring Trimmer
type TrimmerOption func(*Trimmer)

// WithFFmpegPath sets a custom ffmpeg executable path
func WithFFmpegPath(path string) TrimmerOption {
	return func(t *Trimmer) {
		t.ffmpegPath = path
	}
}

// WithCommandRunner sets a custom command runner (for testing)
func WithCommandRunner(runner CommandRunner) TrimmerOption {
	return func(t *Trimmer) {
		t.runner = runner
	}
}

// NewTrimmer creates a new FFmpeg-based trimmer
func NewTrimmer(opts ...TrimmerOption) *Trimmer {
	t := &Trimmer{
		ffmpegPath: "ffmpeg",
		runner:     &ExecCommandRunner{},
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// Trim implements video.Trimmer
func (t *Trimmer) Trim(ctx context.Context, req *video.TrimRequest, outputPath string) error {
	args := []string{
		"-i", req.SourcePath,
		"-ss", req.Start.String(),
		"-to", req.End.String(),
		"-c", "copy",
		"-y", // Overwrite output file if it exists
		outputPath,
	}

	if err := t.runner.Run(ctx, t.ffmpegPath, args...); err != nil {
		return fmt.Errorf("ffmpeg trim failed: %w", err)
	}

	return nil
}

// VerifyInstalled checks that ffmpeg is available
func (t *Trimmer) VerifyInstalled(ctx context.Context) error {
	_, err := t.runner.Output(ctx, t.ffmpegPath, "-version")
	if err != nil {
		return fmt.Errorf("ffmpeg not found or not executable: %w", err)
	}
	return nil
}

// Ensure Trimmer implements video.Trimmer
var _ video.Trimmer = (*Trimmer)(nil)
