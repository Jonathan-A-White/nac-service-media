//go:build detection

package detection

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"

	"nac-service-media/domain/detection"
	"nac-service-media/infrastructure/config"

	"gocv.io/x/gocv"
)

// TemplateDetector implements detection.StartDetector using GoCV template matching
type TemplateDetector struct {
	templates     map[string]gocv.Mat
	templateMeans map[string]float64 // mean brightness of each template
	ffmpegPath    string
	config        config.DetectionConfig
	tempDir       string
}

// TemplateDetectorOption is a functional option for configuring TemplateDetector
type TemplateDetectorOption func(*TemplateDetector)

// WithFFmpegPath sets a custom ffmpeg path
func WithFFmpegPath(path string) TemplateDetectorOption {
	return func(d *TemplateDetector) {
		d.ffmpegPath = path
	}
}

// NewTemplateDetector creates a new template-based detector
func NewTemplateDetector(cfg config.DetectionConfig, opts ...TemplateDetectorOption) *TemplateDetector {
	d := &TemplateDetector{
		templates:     make(map[string]gocv.Mat),
		templateMeans: make(map[string]float64),
		ffmpegPath:    "ffmpeg",
		config:        cfg,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// LoadTemplates loads every shot in the templates folder: each
// <shot>_lit.png with its <shot>_unlit.png.
func (d *TemplateDetector) LoadTemplates(templatesDir string) error {
	templateFiles, err := TemplateFiles(templatesDir)
	if err != nil {
		return err
	}

	for name, path := range templateFiles {
		mat := gocv.IMRead(path, gocv.IMReadGrayScale)
		if mat.Empty() {
			return fmt.Errorf("failed to load template: %s", path)
		}
		d.templates[name] = mat
		d.templateMeans[name] = mat.Mean().Val1
	}

	return nil
}

// Close releases all loaded templates
func (d *TemplateDetector) Close() {
	for _, mat := range d.templates {
		mat.Close()
	}

	// Clean up temp directory if created
	if d.tempDir != "" {
		os.RemoveAll(d.tempDir)
	}
}

// DetectStart implements detection.StartDetector using a 3-phase algorithm
// (see crossScan). When the cross is not found, the frames of the coarse scan
// are left in a folder beside the video and named in the error.
func (d *TemplateDetector) DetectStart(ctx context.Context, videoPath string) (detection.DetectionResult, error) {
	// Create temp directory for extracted frames
	var err error
	d.tempDir, err = os.MkdirTemp("", "nac-detection-*")
	if err != nil {
		return detection.DetectionResult{}, fmt.Errorf("failed to create temp directory: %w (%w)", err, ErrNothingAnalysed)
	}

	return newCrossScan(d.config, d.analyzeFrame, sampleFramesDir(videoPath)).run(ctx, videoPath)
}

// analyzeFrame extracts and analyzes a single frame from the video. When keep
// is not nil it is given the extracted frame before the frame is removed.
func (d *TemplateDetector) analyzeFrame(ctx context.Context, videoPath string, timestampSeconds int, keep func(framePath string)) (detection.FrameAnalysis, error) {
	// Format timestamp for ffmpeg
	hours := timestampSeconds / 3600
	minutes := (timestampSeconds % 3600) / 60
	seconds := timestampSeconds % 60
	timestamp := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)

	// Extract frame using ffmpeg
	framePath := filepath.Join(d.tempDir, fmt.Sprintf("frame_%d.png", timestampSeconds))
	cmd := exec.CommandContext(ctx, d.ffmpegPath,
		"-ss", timestamp,
		"-i", videoPath,
		"-frames:v", "1",
		"-y",
		framePath,
	)
	if err := cmd.Run(); err != nil {
		return detection.FrameAnalysis{}, fmt.Errorf("failed to extract frame at %s: %w", timestamp, err)
	}
	defer os.Remove(framePath)

	// Load and analyze frame
	frame := gocv.IMRead(framePath, gocv.IMReadGrayScale)
	if frame.Empty() {
		return detection.FrameAnalysis{}, fmt.Errorf("failed to read extracted frame at %s", timestamp)
	}
	defer frame.Close()

	if keep != nil {
		keep(framePath)
	}
	return d.analyzeFrameMat(frame, timestampSeconds), nil
}

// analyzeFrameMat analyzes a frame image against all templates
func (d *TemplateDetector) analyzeFrameMat(frame gocv.Mat, timestampSeconds int) detection.FrameAnalysis {
	matches := make([]templateMatch, 0, len(d.templates))
	for name, template := range d.templates {
		if template.Cols() > frame.Cols() || template.Rows() > frame.Rows() {
			continue // A template larger than the frame cannot be in it
		}
		result := gocv.NewMat()
		mask := gocv.NewMat()
		gocv.MatchTemplate(frame, template, &result, gocv.TmCcoeffNormed, mask)
		_, maxVal, _, maxLoc := gocv.MinMaxLoc(result)
		result.Close()
		mask.Close()

		region := frame.Region(image.Rect(maxLoc.X, maxLoc.Y, maxLoc.X+template.Cols(), maxLoc.Y+template.Rows()))
		regionMean := region.Mean().Val1
		region.Close()

		matches = append(matches, templateMatch{
			Name:         name,
			Score:        float64(maxVal),
			RegionMean:   regionMean,
			TemplateMean: d.templateMeans[name],
		})
	}

	return classifyMatches(matches, d.config.Thresholds.MatchScore, timestampSeconds)
}

// Ensure TemplateDetector implements detection.StartDetector
var _ detection.StartDetector = (*TemplateDetector)(nil)
