package ffmpeg

import (
	"context"
	"fmt"

	"nac-service-media/domain/video"
)

// Extractor implements video.AudioExtractor using ffmpeg
type Extractor struct {
	ffmpegPath string
	runner     CommandRunner
}

// ExtractorOption is a functional option for configuring Extractor
type ExtractorOption func(*Extractor)

// WithExtractorFFmpegPath sets a custom ffmpeg executable path
func WithExtractorFFmpegPath(path string) ExtractorOption {
	return func(e *Extractor) {
		e.ffmpegPath = path
	}
}

// WithExtractorCommandRunner sets a custom command runner (for testing)
func WithExtractorCommandRunner(runner CommandRunner) ExtractorOption {
	return func(e *Extractor) {
		e.runner = runner
	}
}

// NewExtractor creates a new FFmpeg-based audio extractor
func NewExtractor(opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		ffmpegPath: "ffmpeg",
		runner:     &ExecCommandRunner{},
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Extract implements video.AudioExtractor
func (e *Extractor) Extract(ctx context.Context, req *video.AudioExtractionRequest, outputPath string) error {
	args := []string{
		"-i", req.SourceVideoPath,
		"-vn",                   // No video
		"-acodec", "libmp3lame", // MP3 codec
		"-ab", req.Bitrate,      // Audio bitrate
		"-y",                    // Overwrite output file if it exists
		outputPath,
	}

	if err := e.runner.Run(ctx, e.ffmpegPath, args...); err != nil {
		return fmt.Errorf("ffmpeg audio extraction failed: %w", err)
	}

	return nil
}

// VerifyInstalled checks that ffmpeg is available
func (e *Extractor) VerifyInstalled(ctx context.Context) error {
	_, err := e.runner.Output(ctx, e.ffmpegPath, "-version")
	if err != nil {
		return fmt.Errorf("ffmpeg not found or not executable: %w", err)
	}
	return nil
}

// Ensure Extractor implements video.AudioExtractor
var _ video.AudioExtractor = (*Extractor)(nil)
