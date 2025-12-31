//go:build detection

package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"

	"nac-service-media/domain/detection"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// AmenDetector implements detection.EndDetector using Python audio analysis
type AmenDetector struct {
	scriptPath            string
	templateDir           string
	startOffsetMinutes    int
	searchDurationMinutes int
	minConfidence         float64
}

// AmenDetectorOption is a functional option for configuring AmenDetector
type AmenDetectorOption func(*AmenDetector)

// WithScriptPath sets a custom path to the detect_amen.py script
func WithScriptPath(path string) AmenDetectorOption {
	return func(d *AmenDetector) {
		d.scriptPath = path
	}
}

// NewAmenDetector creates a new amen-based end detector
func NewAmenDetector(cfg config.DetectionConfig, opts ...AmenDetectorOption) *AmenDetector {
	// Default start offset (when to start looking for amen)
	startOffsetMinutes := cfg.SearchRange.AmenStartOffsetMinutes
	if startOffsetMinutes == 0 {
		startOffsetMinutes = 20 // Default: start looking 20 min into video
	}

	// Default search duration
	searchDurationMinutes := cfg.SearchRange.AmenSearchDurationMinutes
	if searchDurationMinutes == 0 {
		searchDurationMinutes = 90 // Default: search for 90 minutes
	}

	// Default confidence threshold
	minConfidence := cfg.Thresholds.AmenMatchScore
	if minConfidence == 0 {
		minConfidence = 0.50 // Default threshold
	}

	// Default template directory
	templateDir := cfg.AudioTemplatesDir
	if templateDir == "" {
		templateDir = "config/audio_templates"
	}

	d := &AmenDetector{
		scriptPath:            "scripts/detect_amen.py",
		templateDir:           templateDir,
		startOffsetMinutes:    startOffsetMinutes,
		searchDurationMinutes: searchDurationMinutes,
		minConfidence:         minConfidence,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// pythonResult represents the JSON output from detect_amen.py
type pythonResult struct {
	Detected       bool    `json:"detected"`
	AmenEnd        string  `json:"amen_end"`
	AmenEndSeconds int     `json:"amen_end_seconds"`
	Confidence     float64 `json:"confidence"`
	Error          string  `json:"error"`
}

// DetectEnd implements detection.EndDetector
func (d *AmenDetector) DetectEnd(ctx context.Context, videoPath string, serviceStartSeconds int) (detection.EndDetectionResult, error) {
	// Resolve script path relative to working directory if not absolute
	scriptPath := d.scriptPath
	if !filepath.IsAbs(scriptPath) {
		// Script path is relative to current working directory
		scriptPath = d.scriptPath
	}

	// Calculate actual start offset: service start + configured offset
	// e.g., if service starts at 24:05 (1445s) and offset is 20 min, search from 44:05
	actualStartOffsetMinutes := (serviceStartSeconds / 60) + d.startOffsetMinutes

	// Build command: detect_amen.py <video_path> <start_offset_minutes> <search_duration_minutes> <template_dir>
	cmd := exec.CommandContext(ctx, "python3", scriptPath,
		videoPath,
		fmt.Sprintf("%d", actualStartOffsetMinutes),
		fmt.Sprintf("%d", d.searchDurationMinutes),
		d.templateDir,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Try to parse error from stderr or stdout
			return detection.EndDetectionResult{}, fmt.Errorf("amen detection script failed: %s", string(exitErr.Stderr))
		}
		return detection.EndDetectionResult{}, fmt.Errorf("amen detection failed: %w", err)
	}

	// Parse JSON result
	var result pythonResult
	if err := json.Unmarshal(output, &result); err != nil {
		return detection.EndDetectionResult{}, fmt.Errorf("failed to parse detection result: %w\nOutput: %s", err, string(output))
	}

	if !result.Detected {
		return detection.EndDetectionResult{
			Detected: false,
			Error:    result.Error,
		}, nil
	}

	// Check confidence threshold
	if result.Confidence < d.minConfidence {
		return detection.EndDetectionResult{
			Detected: false,
			Error:    fmt.Sprintf("confidence %.2f below threshold %.2f", result.Confidence, d.minConfidence),
		}, nil
	}

	// Parse timestamp
	timestamp, err := video.ParseTimestamp(result.AmenEnd)
	if err != nil {
		return detection.EndDetectionResult{}, fmt.Errorf("invalid timestamp from detection: %w", err)
	}

	return detection.EndDetectionResult{
		Detected:   true,
		Timestamp:  timestamp,
		Confidence: result.Confidence,
	}, nil
}

// Ensure AmenDetector implements detection.EndDetector
var _ detection.EndDetector = (*AmenDetector)(nil)
