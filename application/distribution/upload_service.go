package distribution

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"nac-service-media/domain/distribution"
)

// UploadService handles file upload operations to Google Drive
type UploadService struct {
	driveClient distribution.DriveClient
	folderID    string
	output      io.Writer
}

// NewUploadService creates a new upload service
func NewUploadService(client distribution.DriveClient, folderID string, output io.Writer) *UploadService {
	if output == nil {
		output = io.Discard
	}
	return &UploadService{
		driveClient: client,
		folderID:    folderID,
		output:      output,
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

	fileName := filepath.Base(filePath)

	// Check for existing file with same name and delete if found
	existing, err := s.driveClient.FindFileByName(ctx, s.folderID, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing file: %w", err)
	}
	if existing != nil {
		fmt.Fprintf(s.output, "      Replacing existing %s (%.1f MB)\n", existing.Name, float64(existing.Size)/1024/1024)
		if err := s.driveClient.DeletePermanently(ctx, existing.ID); err != nil {
			return nil, fmt.Errorf("failed to delete existing file %s: %w", existing.Name, err)
		}
	}

	req := distribution.UploadRequest{
		LocalPath: filePath,
		FileName:  fileName,
		FolderID:  s.folderID,
		MimeType:  mimeType,
	}

	result, err := s.driveClient.UploadAndShare(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload and share %s: %w", fileName, err)
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
