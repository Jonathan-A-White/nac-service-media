package video

import "context"

// Trimmer defines the interface for video trimming operations
// This is a port that can be implemented by different infrastructure adapters
type Trimmer interface {
	// Trim trims the video according to the request and saves to outputPath
	Trim(ctx context.Context, req *TrimRequest, outputPath string) error
}

// FileChecker defines the interface for checking file existence
// This is used to validate that source files exist before trimming
type FileChecker interface {
	// Exists returns true if the file exists
	Exists(path string) bool
}
