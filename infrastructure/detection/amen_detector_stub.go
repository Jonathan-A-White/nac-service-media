//go:build !detection

package detection

import (
	"context"
	"errors"

	"nac-service-media/domain/detection"
	"nac-service-media/infrastructure/config"
)

// AmenDetector is a stub when Python/librosa is not available
type AmenDetector struct {
	config config.DetectionConfig
}

// AmenDetectorOption is a functional option for configuring AmenDetector
type AmenDetectorOption func(*AmenDetector)

// WithScriptPath is a no-op in stub mode
func WithScriptPath(path string) AmenDetectorOption {
	return func(d *AmenDetector) {}
}

// NewAmenDetector creates a stub detector (requires building with -tags=detection)
func NewAmenDetector(cfg config.DetectionConfig, opts ...AmenDetectorOption) *AmenDetector {
	return &AmenDetector{config: cfg}
}

// DetectEnd returns an error indicating detection is not available
func (d *AmenDetector) DetectEnd(ctx context.Context, videoPath string, serviceStartSeconds int) (detection.EndDetectionResult, error) {
	return detection.EndDetectionResult{}, errors.New("end detection requires -tags=detection build and Python 3 with librosa")
}

// Ensure AmenDetector implements detection.EndDetector
var _ detection.EndDetector = (*AmenDetector)(nil)
