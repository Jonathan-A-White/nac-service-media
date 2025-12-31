package detection

import (
	"context"
	"fmt"
	"io"

	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"
)

// Service orchestrates start timestamp detection
type Service struct {
	config config.DetectionConfig
	output io.Writer
}

// NewService creates a new detection service
func NewService(cfg config.DetectionConfig, output io.Writer) *Service {
	return &Service{
		config: cfg,
		output: output,
	}
}

// DetectInput contains input for start detection
type DetectInput struct {
	VideoPath string
}

// DetectResult contains the detection outcome
type DetectResult struct {
	Timestamp      string
	Confidence     float64
	CameraAngle    string
	FramesAnalyzed int
}

// DetectStart attempts to detect when the cross lights up in the video
func (s *Service) DetectStart(ctx context.Context, input DetectInput) (*DetectResult, error) {
	fmt.Fprintf(s.output, "Analyzing video for service start...\n")

	// Create detector
	detector := infradetection.NewTemplateDetector(s.config)

	// Load templates
	fmt.Fprintf(s.output, "  Loading templates...\n")
	if err := detector.LoadTemplates(s.config.TemplatesDir); err != nil {
		return nil, fmt.Errorf("failed to load detection templates: %w", err)
	}
	defer detector.Close()

	// Phase 1: Coarse scan
	fmt.Fprintf(s.output, "  Phase 1: Coarse scan...\n")

	// Run detection (phases 1-3 happen inside)
	result, err := detector.DetectStart(ctx, input.VideoPath)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(s.output, "  Phase 2: Binary search...\n")
	fmt.Fprintf(s.output, "  Phase 3: Refining...\n")
	fmt.Fprintf(s.output, "Detected start: %s (%s angle, confidence: %.0f%%)\n",
		result.Timestamp.String(), result.CameraAngle, result.Confidence*100)

	return &DetectResult{
		Timestamp:      result.Timestamp.String(),
		Confidence:     result.Confidence,
		CameraAngle:    result.CameraAngle,
		FramesAnalyzed: result.FramesAnalyzed,
	}, nil
}

// IsEnabled returns whether detection is enabled in config
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}
