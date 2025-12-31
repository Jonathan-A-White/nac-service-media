package detection

import (
	"testing"

	"nac-service-media/domain/video"
)

func TestDetectionResult(t *testing.T) {
	t.Run("stores timestamp correctly", func(t *testing.T) {
		ts, _ := video.ParseTimestamp("00:24:05")
		result := DetectionResult{
			Timestamp:      ts,
			Confidence:     0.95,
			CameraAngle:    "wide",
			FramesAnalyzed: 26,
		}

		if result.Timestamp.String() != "00:24:05" {
			t.Errorf("expected timestamp 00:24:05, got %s", result.Timestamp.String())
		}
		if result.Confidence != 0.95 {
			t.Errorf("expected confidence 0.95, got %f", result.Confidence)
		}
		if result.CameraAngle != "wide" {
			t.Errorf("expected camera angle 'wide', got %s", result.CameraAngle)
		}
		if result.FramesAnalyzed != 26 {
			t.Errorf("expected 26 frames analyzed, got %d", result.FramesAnalyzed)
		}
	})
}

func TestFrameState(t *testing.T) {
	t.Run("state constants are defined", func(t *testing.T) {
		if StateLit != "lit" {
			t.Errorf("expected StateLit to be 'lit', got %s", StateLit)
		}
		if StateUnlit != "unlit" {
			t.Errorf("expected StateUnlit to be 'unlit', got %s", StateUnlit)
		}
		if StateNotVisible != "not_visible" {
			t.Errorf("expected StateNotVisible to be 'not_visible', got %s", StateNotVisible)
		}
	})
}

func TestFrameAnalysis(t *testing.T) {
	t.Run("lit frame analysis", func(t *testing.T) {
		analysis := FrameAnalysis{
			State:            StateLit,
			CameraAngle:      "wide",
			Confidence:       0.92,
			TimestampSeconds: 1445,
		}

		if analysis.State != StateLit {
			t.Errorf("expected state lit, got %s", analysis.State)
		}
		if analysis.CameraAngle != "wide" {
			t.Errorf("expected camera angle 'wide', got %s", analysis.CameraAngle)
		}
	})

	t.Run("not visible frame analysis", func(t *testing.T) {
		analysis := FrameAnalysis{
			State:            StateNotVisible,
			CameraAngle:      "",
			Confidence:       0.45,
			TimestampSeconds: 300,
		}

		if analysis.State != StateNotVisible {
			t.Errorf("expected state not_visible, got %s", analysis.State)
		}
		if analysis.CameraAngle != "" {
			t.Errorf("expected empty camera angle, got %s", analysis.CameraAngle)
		}
	})
}
