package distribution

import (
	"context"
	"fmt"

	"nac-service-media/domain/distribution"
)

// CleanupService handles storage cleanup operations
type CleanupService struct {
	driveClient distribution.DriveClient
	folderID    string
}

// NewCleanupService creates a new cleanup service
func NewCleanupService(client distribution.DriveClient, folderID string) *CleanupService {
	return &CleanupService{
		driveClient: client,
		folderID:    folderID,
	}
}

// EnsureSpaceAvailable deletes oldest mp4 files until sufficient space is available
// It returns the cleanup result with information about deleted files
func (s *CleanupService) EnsureSpaceAvailable(ctx context.Context, neededBytes int64) (*distribution.CleanupResult, error) {
	result := &distribution.CleanupResult{}

	for {
		storage, err := s.driveClient.GetStorageQuota(ctx)
		if err != nil {
			return result, fmt.Errorf("failed to check storage: %w", err)
		}

		if storage.HasSpaceFor(neededBytes) {
			return result, nil
		}

		files, err := s.driveClient.ListMP4Files(ctx, s.folderID)
		if err != nil {
			return result, fmt.Errorf("failed to list files: %w", err)
		}

		if len(files) == 0 {
			return result, fmt.Errorf("no mp4 files to delete, need %d bytes but only %d available",
				neededBytes, storage.AvailableBytes)
		}

		oldest := files[0] // Already sorted by name (oldest first)

		if err := s.driveClient.DeletePermanently(ctx, oldest.ID); err != nil {
			return result, fmt.Errorf("failed to delete %s: %w", oldest.Name, err)
		}

		result.DeletedFiles = append(result.DeletedFiles, distribution.DeletedFile{
			Name: oldest.Name,
			Size: oldest.Size,
		})
		result.FreedBytes += oldest.Size
	}
}

// ListMP4FilesSorted lists MP4 files sorted by filename (oldest first)
func (s *CleanupService) ListMP4FilesSorted(ctx context.Context) ([]distribution.FileInfo, error) {
	return s.driveClient.ListMP4Files(ctx, s.folderID)
}
