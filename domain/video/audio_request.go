package video

import (
	"fmt"
	"path/filepath"
	"time"
)

// DefaultAudioBitrate is the default bitrate for audio extraction
const DefaultAudioBitrate = "192k"

// AudioExtractionRequest represents a request to extract audio from a video
type AudioExtractionRequest struct {
	SourceVideoPath string
	ServiceDate     time.Time
	Bitrate         string
}

// NewAudioExtractionRequest creates a new AudioExtractionRequest with validation
func NewAudioExtractionRequest(sourcePath string, serviceDate time.Time, bitrate string) (*AudioExtractionRequest, error) {
	if sourcePath == "" {
		return nil, fmt.Errorf("source video path is required")
	}

	if bitrate == "" {
		bitrate = DefaultAudioBitrate
	}

	return &AudioExtractionRequest{
		SourceVideoPath: sourcePath,
		ServiceDate:     serviceDate,
		Bitrate:         bitrate,
	}, nil
}

// OutputFilename returns the output filename in YYYY-MM-DD.mp3 format
func (r *AudioExtractionRequest) OutputFilename() string {
	return r.ServiceDate.Format("2006-01-02") + ".mp3"
}

// OutputPath returns the full output path including the directory
func (r *AudioExtractionRequest) OutputPath(outputDir string) string {
	return filepath.Join(outputDir, r.OutputFilename())
}
