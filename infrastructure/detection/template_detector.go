//go:build detection

package detection

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"nac-service-media/domain/detection"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"

	"gocv.io/x/gocv"
)

// TemplateDetector implements detection.StartDetector using GoCV template matching
type TemplateDetector struct {
	templates  map[string]gocv.Mat
	ffmpegPath string
	config     config.DetectionConfig
	tempDir    string
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
		templates:  make(map[string]gocv.Mat),
		ffmpegPath: "ffmpeg",
		config:     cfg,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// LoadTemplates loads template images for matching
func (d *TemplateDetector) LoadTemplates(templatesDir string) error {
	templateFiles := map[string]string{
		"wide_lit":      "wide_lit.png",
		"wide_unlit":    "wide_unlit.png",
		"closeup_lit":   "closeup_lit.png",
		"closeup_unlit": "closeup_unlit.png",
	}

	for name, filename := range templateFiles {
		path := filepath.Join(templatesDir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("template file not found: %s", path)
		}

		mat := gocv.IMRead(path, gocv.IMReadGrayScale)
		if mat.Empty() {
			return fmt.Errorf("failed to load template: %s", path)
		}
		d.templates[name] = mat
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
func (d *TemplateDetector) DetectStart(ctx context.Context, videoPath string) (detection.DetectionResult, error) {
	// Create temp directory for extracted frames
	var err error
	d.tempDir, err = os.MkdirTemp("", "nac-detection-*")
	if err != nil {
		return detection.DetectionResult{}, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Get search range
	startSeconds := d.config.SearchRange.StartMinutes * 60
	endSeconds := d.config.SearchRange.EndMinutes * 60
	coarseStep := d.config.Thresholds.CoarseStepSeconds
	if coarseStep == 0 {
		coarseStep = 30 // Default to 30 seconds
	}

	var framesAnalyzed int

	// Check if cross is already lit at the very beginning (recording started late)
	earlyCheck, err := d.analyzeFrame(ctx, videoPath, 5) // Check at 5 seconds
	framesAnalyzed++
	if err == nil && earlyCheck.State == detection.StateLit {
		// Cross is already lit - return 00:00:00 as start
		return detection.DetectionResult{
			Timestamp:      video.Timestamp{Hours: 0, Minutes: 0, Seconds: 0},
			Confidence:     earlyCheck.Confidence,
			CameraAngle:    earlyCheck.CameraAngle,
			FramesAnalyzed: framesAnalyzed,
		}, nil
	}

	// Phase 1: Coarse scan to find bounds
	var firstUnlitTime, firstLitTime int
	foundUnlit, foundLit := false, false

	// Start from 0 if the early check showed unlit or not visible
	scanStart := 0
	if earlyCheck.State == detection.StateUnlit {
		firstUnlitTime = 5
		foundUnlit = true
		scanStart = coarseStep // Skip ahead since we already checked the beginning
	}

	for t := scanStart; t <= endSeconds; t += coarseStep {
		select {
		case <-ctx.Done():
			return detection.DetectionResult{}, ctx.Err()
		default:
		}

		analysis, err := d.analyzeFrame(ctx, videoPath, t)
		framesAnalyzed++
		if err != nil {
			continue // Skip frames that fail to extract
		}

		if analysis.State == detection.StateUnlit && !foundUnlit {
			firstUnlitTime = t
			foundUnlit = true
		}
		if analysis.State == detection.StateLit && !foundLit {
			firstLitTime = t
			foundLit = true
			break // Found lit, we have our bounds
		}
	}

	if !foundLit {
		return detection.DetectionResult{FramesAnalyzed: framesAnalyzed}, fmt.Errorf("could not detect cross lighting up in search range")
	}

	// If we found lit but no unlit, search backwards
	if !foundUnlit {
		for t := firstLitTime - coarseStep; t >= startSeconds; t -= coarseStep {
			analysis, err := d.analyzeFrame(ctx, videoPath, t)
			framesAnalyzed++
			if err != nil {
				continue
			}
			if analysis.State == detection.StateUnlit || analysis.State == detection.StateNotVisible {
				firstUnlitTime = t
				foundUnlit = true
				break
			}
		}
		if !foundUnlit {
			firstUnlitTime = startSeconds
		}
	}

	// Phase 2: Binary search to narrow down
	low, high := firstUnlitTime, firstLitTime
	var lastLitAnalysis detection.FrameAnalysis

	for high-low > 1 {
		select {
		case <-ctx.Done():
			return detection.DetectionResult{}, ctx.Err()
		default:
		}

		mid := (low + high) / 2
		analysis, err := d.analyzeFrame(ctx, videoPath, mid)
		framesAnalyzed++
		if err != nil {
			// On error, bias towards the lit end
			low = mid
			continue
		}

		if analysis.State == detection.StateLit {
			high = mid
			lastLitAnalysis = analysis
		} else {
			low = mid
		}
	}

	// Phase 3: Refinement - scan second by second around the transition
	transitionTime := high
	for t := high - 2; t <= high+2; t++ {
		if t < low {
			continue
		}
		analysis, err := d.analyzeFrame(ctx, videoPath, t)
		framesAnalyzed++
		if err != nil {
			continue
		}
		if analysis.State == detection.StateLit {
			transitionTime = t
			lastLitAnalysis = analysis
			break
		}
	}

	// Convert to timestamp
	hours := transitionTime / 3600
	minutes := (transitionTime % 3600) / 60
	seconds := transitionTime % 60

	timestamp := video.Timestamp{
		Hours:   hours,
		Minutes: minutes,
		Seconds: seconds,
	}

	return detection.DetectionResult{
		Timestamp:      timestamp,
		Confidence:     lastLitAnalysis.Confidence,
		CameraAngle:    lastLitAnalysis.CameraAngle,
		FramesAnalyzed: framesAnalyzed,
	}, nil
}

// analyzeFrame extracts and analyzes a single frame from the video
func (d *TemplateDetector) analyzeFrame(ctx context.Context, videoPath string, timestampSeconds int) (detection.FrameAnalysis, error) {
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
		return detection.FrameAnalysis{}, fmt.Errorf("failed to read extracted frame")
	}
	defer frame.Close()

	return d.analyzeFrameMat(frame, timestampSeconds), nil
}

// analyzeFrameMat analyzes a frame image against all templates
func (d *TemplateDetector) analyzeFrameMat(frame gocv.Mat, timestampSeconds int) detection.FrameAnalysis {
	threshold := d.config.Thresholds.MatchScore
	if threshold == 0 {
		threshold = 0.85 // Default threshold
	}

	var bestMatch struct {
		name       string
		score      float64
		isLit      bool
		cameraType string
	}

	for name, template := range d.templates {
		result := gocv.NewMat()
		gocv.MatchTemplate(frame, template, &result, gocv.TmCcoeffNormed, gocv.NewMat())
		_, maxVal, _, _ := gocv.MinMaxLoc(result)
		result.Close()

		score := float64(maxVal)
		if score > bestMatch.score {
			bestMatch.name = name
			bestMatch.score = score
			bestMatch.isLit = strings.HasSuffix(name, "_lit")
			bestMatch.cameraType = strings.TrimSuffix(strings.TrimSuffix(name, "_lit"), "_unlit")
		}
	}

	if bestMatch.score >= threshold {
		state := detection.StateUnlit
		if bestMatch.isLit {
			state = detection.StateLit
		}
		return detection.FrameAnalysis{
			State:            state,
			CameraAngle:      bestMatch.cameraType,
			Confidence:       bestMatch.score,
			TimestampSeconds: timestampSeconds,
		}
	}

	return detection.FrameAnalysis{
		State:            detection.StateNotVisible,
		CameraAngle:      "",
		Confidence:       bestMatch.score,
		TimestampSeconds: timestampSeconds,
	}
}

// Ensure TemplateDetector implements detection.StartDetector
var _ detection.StartDetector = (*TemplateDetector)(nil)
