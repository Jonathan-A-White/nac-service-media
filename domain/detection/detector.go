package detection

import (
	"context"

	"nac-service-media/domain/video"
)

// StartDetector defines the interface for detecting service start timestamps
type StartDetector interface {
	// DetectStart analyzes a video to find when the cross lights up
	DetectStart(ctx context.Context, videoPath string) (DetectionResult, error)

	// LoadTemplates loads the template images for matching
	LoadTemplates(templatesDir string) error

	// Close releases any resources
	Close()
}

// DetectionResult contains the outcome of start timestamp detection
type DetectionResult struct {
	// Timestamp is the detected start time in HH:MM:SS format
	Timestamp video.Timestamp

	// Confidence is the template match score (0.0-1.0)
	Confidence float64

	// CameraAngle indicates which template matched ("wide" or "closeup")
	CameraAngle string

	// FramesAnalyzed is the number of frames processed during detection
	FramesAnalyzed int
}

// FrameState represents the detected state of the cross in a video frame
type FrameState string

const (
	// StateLit indicates the cross is lit (lights on)
	StateLit FrameState = "lit"

	// StateUnlit indicates the cross is unlit (lights off)
	StateUnlit FrameState = "unlit"

	// StateNotVisible indicates the cross is not visible in the frame
	StateNotVisible FrameState = "not_visible"
)

// FrameAnalysis contains the result of analyzing a single frame
type FrameAnalysis struct {
	// State is the detected cross state
	State FrameState

	// CameraAngle indicates which template matched (empty if not visible)
	CameraAngle string

	// Confidence is the match score (0.0-1.0)
	Confidence float64

	// TimestampSeconds is the frame's position in the video
	TimestampSeconds int
}

// EndDetector defines the interface for detecting service end timestamps
type EndDetector interface {
	// DetectEnd analyzes a video's audio to find the three-fold amen
	// serviceStartSeconds is the detected/provided service start time in the video
	DetectEnd(ctx context.Context, videoPath string, serviceStartSeconds int) (EndDetectionResult, error)
}

// EndDetectionResult contains the outcome of end timestamp detection
type EndDetectionResult struct {
	// Timestamp is the detected end time (end of amen)
	Timestamp video.Timestamp

	// Confidence is the match score (0.0-1.0)
	Confidence float64

	// Detected indicates whether the amen was found
	Detected bool

	// Error message if detection failed
	Error string
}
