package video

import (
	"context"
	"fmt"

	"nac-service-media/domain/video"
)

// TrimResult contains the result of a trim operation
type TrimResult struct {
	OutputPath  string
	ServiceDate string
}

// TrimService coordinates video trimming operations
type TrimService struct {
	trimmer     video.Trimmer
	fileChecker video.FileChecker
	outputDir   string
}

// NewTrimService creates a new TrimService
func NewTrimService(trimmer video.Trimmer, fileChecker video.FileChecker, outputDir string) *TrimService {
	return &TrimService{
		trimmer:     trimmer,
		fileChecker: fileChecker,
		outputDir:   outputDir,
	}
}

// TrimInput represents the input for a trim operation
type TrimInput struct {
	SourcePath string
	StartTime  string
	EndTime    string
}

// Trim trims a video according to the input parameters
func (s *TrimService) Trim(ctx context.Context, input TrimInput) (*TrimResult, error) {
	// Verify source file exists
	if !s.fileChecker.Exists(input.SourcePath) {
		return nil, fmt.Errorf("source file does not exist: %s", input.SourcePath)
	}

	// Parse timestamps
	start, err := video.ParseTimestamp(input.StartTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start time: %w", err)
	}

	end, err := video.ParseTimestamp(input.EndTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end time: %w", err)
	}

	// Create trim request
	req, err := video.NewTrimRequest(input.SourcePath, start, end)
	if err != nil {
		return nil, err
	}

	// Perform trim
	outputPath := req.OutputPath(s.outputDir)
	if err := s.trimmer.Trim(ctx, req, outputPath); err != nil {
		return nil, err
	}

	return &TrimResult{
		OutputPath:  outputPath,
		ServiceDate: req.ServiceDate.Format("2006-01-02"),
	}, nil
}
