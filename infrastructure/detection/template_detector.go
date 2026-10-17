package detection

// The cross detector: frames out of the video with ffmpeg, matched in Go
// against every picture of the cross at every scale (match.go), searched
// for the moment the cross lights up (scan.go). No build tag and no OpenCV:
// the gate tests the whole path.

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"nac-service-media/domain/detection"
	"nac-service-media/infrastructure/config"
)

// TemplateDetector implements detection.StartDetector by template matching.
type TemplateDetector struct {
	config  config.DetectionConfig
	frames  FrameExtractor
	out     io.Writer
	matcher *matcher
	tempDir string

	samplesDir *string // where a miss leaves its sampled frames; nil: beside the video
}

// TemplateDetectorOption is a functional option for configuring TemplateDetector
type TemplateDetectorOption func(*TemplateDetector)

// WithFFmpegPath sets a custom ffmpeg path
func WithFFmpegPath(path string) TemplateDetectorOption {
	return func(d *TemplateDetector) {
		d.frames = FFmpegFrames{FFmpegPath: path}
	}
}

// WithFrameExtractor replaces ffmpeg as the source of frames.
func WithFrameExtractor(frames FrameExtractor) TemplateDetectorOption {
	return func(d *TemplateDetector) {
		d.frames = frames
	}
}

// WithOutput is where the search writes one line per frame it looks at.
func WithOutput(out io.Writer) TemplateDetectorOption {
	return func(d *TemplateDetector) {
		d.out = out
	}
}

// NewTemplateDetector creates a new template-based detector
func NewTemplateDetector(cfg config.DetectionConfig, opts ...TemplateDetectorOption) *TemplateDetector {
	d := &TemplateDetector{config: cfg, frames: FFmpegFrames{}, out: io.Discard}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// LoadTemplates loads every shot in the templates folder: each
// <shot>_lit.png with its <shot>_unlit.png.
func (d *TemplateDetector) LoadTemplates(templatesDir string) error {
	files, err := TemplateFiles(templatesDir)
	if err != nil {
		return fmt.Errorf("%w (%w)", err, ErrNothingAnalysed)
	}
	templates := map[string]*image.Gray{}
	for name, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to load template %s: %w (%w)", path, err, ErrNothingAnalysed)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			return fmt.Errorf("failed to load template %s: %w (%w)", path, err, ErrNothingAnalysed)
		}
		templates[name] = toGray(img)
	}
	d.matcher, err = newMatcher(templates, d.config.Thresholds.MatchScore)
	if err != nil {
		return fmt.Errorf("%w (%w)", err, ErrNothingAnalysed)
	}
	return nil
}

// Close removes the folder the frames were extracted into.
func (d *TemplateDetector) Close() {
	if d.tempDir != "" {
		os.RemoveAll(d.tempDir)
		d.tempDir = ""
	}
}

// DetectStart implements detection.StartDetector using a 3-phase algorithm
// (see crossScan). When the cross is not found, the frames of the coarse scan
// are left in a folder beside the video and named in the error, which also
// says the best each picture of the cross scored.
func (d *TemplateDetector) DetectStart(ctx context.Context, videoPath string) (detection.DetectionResult, error) {
	if d.matcher == nil {
		return detection.DetectionResult{}, fmt.Errorf("no templates loaded (%w)", ErrNothingAnalysed)
	}
	var err error
	d.tempDir, err = os.MkdirTemp("", "nac-detection-*")
	if err != nil {
		return detection.DetectionResult{}, fmt.Errorf("failed to create temp directory: %w (%w)", err, ErrNothingAnalysed)
	}
	return newCrossScan(d.config, d.analyzeFrame, d.samplesDirFor(videoPath), d.out).run(ctx, videoPath)
}

// analyzeFrame extracts and analyzes a single frame from the video. When keep
// is not nil it is given the extracted frame before the frame is removed.
func (d *TemplateDetector) analyzeFrame(ctx context.Context, videoPath string, seconds int, keep func(framePath string)) (frameReport, error) {
	at := secondsToTimestamp(seconds)
	framePath := filepath.Join(d.tempDir, fmt.Sprintf("frame_%d.png", seconds))
	if err := d.frames.ExtractFrame(ctx, videoPath, at, framePath); err != nil {
		return frameReport{}, fmt.Errorf("failed to extract frame at %s: %w", at, err)
	}
	defer os.Remove(framePath)

	file, err := os.Open(framePath)
	if err != nil {
		return frameReport{}, fmt.Errorf("failed to read extracted frame at %s: %w", at, err)
	}
	img, err := png.Decode(file)
	file.Close()
	if err != nil {
		return frameReport{}, fmt.Errorf("failed to read extracted frame at %s: %w", at, err)
	}
	if keep != nil {
		keep(framePath)
	}
	return d.matcher.analyze(img, seconds), nil
}

// Ensure TemplateDetector implements detection.StartDetector
var _ detection.StartDetector = (*TemplateDetector)(nil)
