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
	StartTime       *Timestamp // Optional: start timestamp for extraction
	EndTime         *Timestamp // Optional: end timestamp for extraction
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

// NewAudioExtractionRequestWithTimestamps creates a request with start/end timestamps
// This is used to extract audio from a specific time range of the source video
func NewAudioExtractionRequestWithTimestamps(sourcePath string, serviceDate time.Time, bitrate, startTime, endTime string) (*AudioExtractionRequest, error) {
	if sourcePath == "" {
		return nil, fmt.Errorf("source video path is required")
	}

	if bitrate == "" {
		bitrate = DefaultAudioBitrate
	}

	// Parse timestamps
	start, err := ParseTimestamp(startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start time: %w", err)
	}

	end, err := ParseTimestamp(endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end time: %w", err)
	}

	return &AudioExtractionRequest{
		SourceVideoPath: sourcePath,
		ServiceDate:     serviceDate,
		Bitrate:         bitrate,
		StartTime:       &start,
		EndTime:         &end,
	}, nil
}

// HasTimestamps returns true if the request has start/end timestamps for extraction
func (r *AudioExtractionRequest) HasTimestamps() bool {
	return r.StartTime != nil && r.EndTime != nil
}

// OutputFilename returns the output filename in YYYY-MM-DD.mp3 format
func (r *AudioExtractionRequest) OutputFilename() string {
	return r.ServiceDate.Format("2006-01-02") + ".mp3"
}

// OutputPath returns the full output path including the directory
func (r *AudioExtractionRequest) OutputPath(outputDir string) string {
	return filepath.Join(outputDir, r.OutputFilename())
}
