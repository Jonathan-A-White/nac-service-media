package distribution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"nac-service-media/domain/distribution"
)

// UploadService handles file upload operations to Google Drive
type UploadService struct {
	driveClient distribution.DriveClient
	folderID    string
}

// NewUploadService creates a new upload service
func NewUploadService(client distribution.DriveClient, folderID string) *UploadService {
	return &UploadService{
		driveClient: client,
		folderID:    folderID,
	}
}

// DistributionResult contains URLs for uploaded video and audio files
type DistributionResult struct {
	VideoURL   string
	VideoID    string
	VideoSize  int64
	AudioURL   string
	AudioID    string
	AudioSize  int64
}

// UploadVideo uploads a video file to Google Drive and sets public sharing
func (s *UploadService) UploadVideo(ctx context.Context, videoPath string) (*distribution.UploadResult, error) {
	return s.uploadAndShare(ctx, videoPath, distribution.MimeTypeMP4)
}

// UploadAudio uploads an audio file to Google Drive and sets public sharing
func (s *UploadService) UploadAudio(ctx context.Context, audioPath string) (*distribution.UploadResult, error) {
	return s.uploadAndShare(ctx, audioPath, distribution.MimeTypeMP3)
}

// uploadAndShare uploads a file and sets public sharing permissions
func (s *UploadService) uploadAndShare(ctx context.Context, filePath, mimeType string) (*distribution.UploadResult, error) {
	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	req := distribution.UploadRequest{
		LocalPath: filePath,
		FileName:  filepath.Base(filePath),
		FolderID:  s.folderID,
		MimeType:  mimeType,
	}

	result, err := s.driveClient.UploadAndShare(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload and share %s: %w", filepath.Base(filePath), err)
	}

	return result, nil
}

// Distribute uploads both video and audio files to Google Drive with sharing
func (s *UploadService) Distribute(ctx context.Context, videoPath, audioPath string) (*DistributionResult, error) {
	videoResult, err := s.UploadVideo(ctx, videoPath)
	if err != nil {
		return nil, fmt.Errorf("video upload failed: %w", err)
	}

	audioResult, err := s.UploadAudio(ctx, audioPath)
	if err != nil {
		return nil, fmt.Errorf("audio upload failed: %w", err)
	}

	return &DistributionResult{
		VideoURL:  videoResult.ShareableURL,
		VideoID:   videoResult.FileID,
		VideoSize: videoResult.Size,
		AudioURL:  audioResult.ShareableURL,
		AudioID:   audioResult.FileID,
		AudioSize: audioResult.Size,
	}, nil
}
