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

	// GetStorageQuota returns the current storage quota information
	GetStorageQuota(ctx context.Context) (*StorageInfo, error)

	// ListMP4Files lists MP4 files in a folder sorted by filename (oldest first)
	ListMP4Files(ctx context.Context, folderID string) ([]FileInfo, error)

	// DeletePermanently deletes a file permanently (bypasses trash)
	DeletePermanently(ctx context.Context, fileID string) error

	// EmptyTrash empties the trash permanently
	EmptyTrash(ctx context.Context) error
}

// FileInfo represents metadata about a file in Google Drive
type FileInfo struct {
	ID          string
	Name        string
	MimeType    string
	Size        int64
	CreatedTime time.Time
}
