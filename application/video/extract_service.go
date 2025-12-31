package video

import (
	"context"
	"fmt"
	"time"

	"nac-service-media/domain/video"
)

// ExtractResult contains the result of an audio extraction operation
type ExtractResult struct {
	OutputPath  string
	ServiceDate string
}

// ExtractService coordinates audio extraction operations
type ExtractService struct {
	extractor   video.AudioExtractor
	fileChecker video.FileChecker
	outputDir   string
	bitrate     string
}

// NewExtractService creates a new ExtractService
func NewExtractService(extractor video.AudioExtractor, fileChecker video.FileChecker, outputDir string, bitrate string) *ExtractService {
	if bitrate == "" {
		bitrate = video.DefaultAudioBitrate
	}
	return &ExtractService{
		extractor:   extractor,
		fileChecker: fileChecker,
		outputDir:   outputDir,
		bitrate:     bitrate,
	}
}

// ExtractInput represents the input for an audio extraction operation
type ExtractInput struct {
	SourcePath  string
	ServiceDate time.Time
	Bitrate     string // Optional, uses service default if empty
}

// ExtractWithTimestampsInput represents input for extracting audio from a source with timestamps
type ExtractWithTimestampsInput struct {
	SourcePath  string
	ServiceDate time.Time
	Bitrate     string
	StartTime   string // HH:MM:SS format
	EndTime     string // HH:MM:SS format
}

// Extract extracts audio from a video according to the input parameters
func (s *ExtractService) Extract(ctx context.Context, input ExtractInput) (*ExtractResult, error) {
	// Verify source file exists
	if !s.fileChecker.Exists(input.SourcePath) {
		return nil, fmt.Errorf("source video does not exist: %s", input.SourcePath)
	}

	// Use service default bitrate if not specified
	bitrate := input.Bitrate
	if bitrate == "" {
		bitrate = s.bitrate
	}

	// Create extraction request
	req, err := video.NewAudioExtractionRequest(input.SourcePath, input.ServiceDate, bitrate)
	if err != nil {
		return nil, err
	}

	// Perform extraction
	outputPath := req.OutputPath(s.outputDir)
	if err := s.extractor.Extract(ctx, req, outputPath); err != nil {
		return nil, err
	}

	return &ExtractResult{
		OutputPath:  outputPath,
		ServiceDate: req.ServiceDate.Format("2006-01-02"),
	}, nil
}

// ExtractWithTimestamps extracts audio from a source video using start/end timestamps
// This is used in skip-video mode to extract audio directly from the source without trimming
func (s *ExtractService) ExtractWithTimestamps(ctx context.Context, input ExtractWithTimestampsInput) (*ExtractResult, error) {
	// Verify source file exists
	if !s.fileChecker.Exists(input.SourcePath) {
		return nil, fmt.Errorf("source video does not exist: %s", input.SourcePath)
	}

	// Use service default bitrate if not specified
	bitrate := input.Bitrate
	if bitrate == "" {
		bitrate = s.bitrate
	}

	// Create extraction request with timestamps
	req, err := video.NewAudioExtractionRequestWithTimestamps(input.SourcePath, input.ServiceDate, bitrate, input.StartTime, input.EndTime)
	if err != nil {
		return nil, err
	}

	// Perform extraction
	outputPath := req.OutputPath(s.outputDir)
	if err := s.extractor.Extract(ctx, req, outputPath); err != nil {
		return nil, err
	}

	return &ExtractResult{
		OutputPath:  outputPath,
		ServiceDate: req.ServiceDate.Format("2006-01-02"),
	}, nil
}
