package distribution

import (
	"context"
	"time"
)

// DriveClient defines the interface for Google Drive operations
// This is a port that can be implemented by different infrastructure adapters
type DriveClient interface {
	// ListFiles lists files in a folder
	ListFiles(ctx context.Context, folderID string) ([]FileInfo, error)
}

// FileInfo represents metadata about a file in Google Drive
type FileInfo struct {
	ID          string
	Name        string
	MimeType    string
	Size        int64
	CreatedTime time.Time
}
