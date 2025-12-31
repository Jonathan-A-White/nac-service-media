package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	appdist "nac-service-media/application/distribution"
	"nac-service-media/domain/distribution"
	"nac-service-media/infrastructure/drive"

	"github.com/spf13/cobra"
)

var (
	uploadVideoPath string
	uploadAudioPath string
	uploadVideoOnly bool
	uploadAudioOnly bool
)

var uploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload files to Google Drive with public sharing",
	Long: `Upload video and/or audio files to Google Drive and set public sharing.

By default, uploads both video and audio files based on the most recent service.
Use --video and --audio flags to specify specific files.
Use --video-only or --audio-only to upload only one type.

The files will be uploaded to the configured Google Drive Services folder
and made publicly accessible with "anyone with the link" permission.

Example:
  nac-service-media upload
  nac-service-media upload --video /path/to/2025-12-28.mp4 --audio /path/to/2025-12-28.mp3
  nac-service-media upload --video-only --video /path/to/2025-12-28.mp4`,
	RunE: runUpload,
}

func init() {
	rootCmd.AddCommand(uploadCmd)
	uploadCmd.Flags().StringVar(&uploadVideoPath, "video", "", "Path to video file (defaults to latest in trimmed directory)")
	uploadCmd.Flags().StringVar(&uploadAudioPath, "audio", "", "Path to audio file (defaults to latest in audio directory)")
	uploadCmd.Flags().BoolVar(&uploadVideoOnly, "video-only", false, "Upload only the video file")
	uploadCmd.Flags().BoolVar(&uploadAudioOnly, "audio-only", false, "Upload only the audio file")
}

func runUpload(cmd *cobra.Command, args []string) error {
	// Ensure config is loaded
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}

	// Resolve video path
	videoPath := uploadVideoPath
	if videoPath == "" && !uploadAudioOnly {
		// Find latest video in trimmed directory
		var err error
		videoPath, err = findLatestFile(cfg.Paths.TrimmedDirectory, ".mp4")
		if err != nil {
			return fmt.Errorf("no video file specified and could not find latest: %w", err)
		}
	}

	// Resolve audio path
	audioPath := uploadAudioPath
	if audioPath == "" && !uploadVideoOnly {
		// Find latest audio in audio directory
		var err error
		audioPath, err = findLatestFile(cfg.Paths.AudioDirectory, ".mp3")
		if err != nil {
			return fmt.Errorf("no audio file specified and could not find latest: %w", err)
		}
	}

	// Create drive client with OAuth
	ctx := cmd.Context()
	client, err := drive.NewClientWithOAuth(ctx, cfg.Google.CredentialsFile, cfg.Google.TokenFile)
	if err != nil {
		return fmt.Errorf("failed to create Google Drive client: %w", err)
	}

	return RunUploadWithDependencies(
		ctx,
		client,
		cfg.Google.ServicesFolderID,
		videoPath,
		audioPath,
		uploadVideoOnly,
		uploadAudioOnly,
		os.Stdout,
	)
}

// findLatestFile finds the most recently modified file with given extension in directory
func findLatestFile(dir, ext string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var latestPath string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ext {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestPath = filepath.Join(dir, entry.Name())
		}
	}

	if latestPath == "" {
		return "", fmt.Errorf("no %s files found in %s", ext, dir)
	}

	return latestPath, nil
}

// RunUploadWithDependencies runs the upload command with injected dependencies (for testing)
func RunUploadWithDependencies(
	ctx context.Context,
	driveClient distribution.DriveClient,
	folderID string,
	videoPath string,
	audioPath string,
	videoOnly bool,
	audioOnly bool,
	output io.Writer,
) error {
	service := appdist.NewUploadService(driveClient, folderID)

	// Upload video if not audio-only
	if !audioOnly && videoPath != "" {
		fmt.Fprintf(output, "Uploading video: %s...\n", filepath.Base(videoPath))
		result, err := service.UploadVideo(ctx, videoPath)
		if err != nil {
			return fmt.Errorf("video upload failed: %w", err)
		}
		fmt.Fprintf(output, "Video uploaded successfully!\n")
		fmt.Fprintf(output, "  File ID: %s\n", result.FileID)
		fmt.Fprintf(output, "  Size: %.2f MB\n", float64(result.Size)/1024/1024)
		fmt.Fprintf(output, "  Shareable URL: %s\n", result.ShareableURL)
		fmt.Fprintln(output)
	}

	// Upload audio if not video-only
	if !videoOnly && audioPath != "" {
		fmt.Fprintf(output, "Uploading audio: %s...\n", filepath.Base(audioPath))
		result, err := service.UploadAudio(ctx, audioPath)
		if err != nil {
			return fmt.Errorf("audio upload failed: %w", err)
		}
		fmt.Fprintf(output, "Audio uploaded successfully!\n")
		fmt.Fprintf(output, "  File ID: %s\n", result.FileID)
		fmt.Fprintf(output, "  Size: %.2f MB\n", float64(result.Size)/1024/1024)
		fmt.Fprintf(output, "  Shareable URL: %s\n", result.ShareableURL)
		fmt.Fprintln(output)
	}

	fmt.Fprintf(output, "Upload complete!\n")
	return nil
}
