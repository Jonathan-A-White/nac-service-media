//go:build !detection

package detection

import (
	"context"
	"fmt"

	"nac-service-media/domain/detection"
	"nac-service-media/infrastructure/config"
)

// TemplateDetector is a stub when GoCV/OpenCV is not available
type TemplateDetector struct {
	config config.DetectionConfig
}

// NewTemplateDetector creates a stub detector (requires building with -tags=detection)
func NewTemplateDetector(cfg config.DetectionConfig, opts ...TemplateDetectorOption) *TemplateDetector {
	return &TemplateDetector{config: cfg}
}

// TemplateDetectorOption is a functional option for configuring TemplateDetector
type TemplateDetectorOption func(*TemplateDetector)

// WithFFmpegPath is a no-op in stub mode
func WithFFmpegPath(path string) TemplateDetectorOption {
	return func(d *TemplateDetector) {}
}

// LoadTemplates returns an error indicating detection is not available
func (d *TemplateDetector) LoadTemplates(templatesDir string) error {
	return fmt.Errorf("detection not available: build with '-tags=detection' and install OpenCV/GoCV")
}

// Close is a no-op in stub mode
func (d *TemplateDetector) Close() {}

// DetectStart returns an error indicating detection is not available
func (d *TemplateDetector) DetectStart(ctx context.Context, videoPath string) (detection.DetectionResult, error) {
	return detection.DetectionResult{}, fmt.Errorf("detection not available: build with '-tags=detection' and install OpenCV/GoCV")
}

// Ensure TemplateDetector implements detection.StartDetector
var _ detection.StartDetector = (*TemplateDetector)(nil)
