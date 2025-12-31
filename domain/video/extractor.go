package video

import "context"

// AudioExtractor defines the interface for audio extraction operations
// This is a port that can be implemented by different infrastructure adapters
type AudioExtractor interface {
	// Extract extracts audio from a video according to the request and saves to outputPath
	Extract(ctx context.Context, req *AudioExtractionRequest, outputPath string) error
}
