package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	appdetection "nac-service-media/application/detection"
	appprocess "nac-service-media/application/process"
	"nac-service-media/domain/distribution"
	"nac-service-media/domain/notification"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
	"nac-service-media/infrastructure/drive"
	"nac-service-media/infrastructure/ffmpeg"
	"nac-service-media/infrastructure/filesystem"
	"nac-service-media/infrastructure/gmail"

	"github.com/spf13/cobra"
)

var (
	processInputPath     string
	processStartTime     string
	processEndTime       string
	processMinisterKey   string
	processRecipientKeys []string
	processCCKeys        []string
	processDateOverride  string
	processSenderKey     string
)

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Process a service recording through the complete workflow",
	Long: `Process a service recording through the complete automated workflow:
1. Detect or specify start timestamp (auto-detection when --start omitted)
2. Trim video to specified timestamps
3. Extract audio as MP3
4. Clean up old files from Google Drive if needed
5. Upload video and audio to Google Drive
6. Share files publicly
7. Send email notification with links

The source video can be specified with --input, or the newest file in the
source directory will be used by default.

The start timestamp can be auto-detected (cross lighting up) when --start is
omitted and detection.enabled is true in config. You will be prompted to
confirm or adjust the detected timestamp.

The service date is inferred from the filename (OBS format: YYYY-MM-DD HH-MM-SS.mp4),
or can be specified with --date.

Ministers, recipients, and CCs are looked up by their config keys.

Example:
  # Auto-detect start timestamp
  nac-service-media process --end 01:45:00 --minister smith --recipient jane

  # Specify start timestamp manually
  nac-service-media process --start 00:05:30 --end 01:45:00 --minister smith --recipient jane

  nac-service-media process \
    --input "2025-12-28 10-06-16.mp4" \
    --start 00:05:30 \
    --end 01:45:00 \
    --minister smith \
    --recipient jane --recipient john \
    --sender avteam`,
	RunE: runProcess,
}

func init() {
	rootCmd.AddCommand(processCmd)
	processCmd.Flags().StringVar(&processInputPath, "input", "", "Path to source video file (defaults to newest in source directory)")
	processCmd.Flags().StringVar(&processStartTime, "start", "", "Start timestamp in HH:MM:SS format (auto-detected if omitted)")
	processCmd.Flags().StringVar(&processEndTime, "end", "", "End timestamp in HH:MM:SS format (required)")
	processCmd.Flags().StringVar(&processMinisterKey, "minister", "", "Minister config key (required)")
	processCmd.Flags().StringArrayVar(&processRecipientKeys, "recipient", nil, "Recipient config key(s) (required, can be repeated)")
	processCmd.Flags().StringArrayVar(&processCCKeys, "cc", nil, "Additional CC config key(s) (optional)")
	processCmd.Flags().StringVar(&processDateOverride, "date", "", "Override service date (YYYY-MM-DD)")
	processCmd.Flags().StringVar(&processSenderKey, "sender", "", "Sender config key (defaults to config default_sender)")

	// --start is now optional (auto-detected when omitted)
	processCmd.MarkFlagRequired("end")
	processCmd.MarkFlagRequired("minister")
	processCmd.MarkFlagRequired("recipient")
}

func runProcess(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}

	ctx := cmd.Context()

	// Create production dependencies
	trimmer := ffmpeg.NewTrimmer()
	extractor := ffmpeg.NewExtractor()
	fileChecker := filesystem.NewChecker()
	fileFinder := &ProductionFileFinder{}

	// Resolve video path for detection
	startTime := processStartTime
	if startTime == "" {
		// Need to detect start timestamp
		videoPath := processInputPath
		if videoPath == "" {
			// Find newest file
			newest, err := fileFinder.FindNewestFile(cfg.Paths.SourceDirectory, ".mp4")
			if err != nil {
				return fmt.Errorf("failed to find video file: %w", err)
			}
			videoPath = newest
		} else if !filepath.IsAbs(videoPath) {
			videoPath = filepath.Join(cfg.Paths.SourceDirectory, videoPath)
		}

		// Check if detection is enabled
		if !cfg.Detection.Enabled {
			return fmt.Errorf("--start flag is required (auto-detection is disabled in config)")
		}

		// Run detection
		detectedTime, err := detectStartTimestamp(ctx, cfg, videoPath)
		if err != nil {
			return err
		}
		startTime = detectedTime
	}

	// Create Drive client
	driveClient, err := drive.NewClientWithOAuth(ctx, cfg.Google.CredentialsFile, cfg.Google.TokenFile)
	if err != nil {
		return fmt.Errorf("failed to create Google Drive client: %w", err)
	}

	// Create Gmail client
	from := notification.Recipient{
		Name:    cfg.Email.FromName,
		Address: cfg.Email.FromAddress,
	}
	gmailClient, err := gmail.NewClientWithOAuth(ctx, gmail.OAuthConfig{
		CredentialsFile: cfg.Google.CredentialsFile,
		TokenFile:       "gmail_token.json",
	}, from)
	if err != nil {
		return fmt.Errorf("failed to create Gmail client: %w", err)
	}

	input := ProcessInput{
		InputPath:     processInputPath,
		StartTime:     startTime,
		EndTime:       processEndTime,
		MinisterKey:   processMinisterKey,
		RecipientKeys: processRecipientKeys,
		CCKeys:        processCCKeys,
		DateOverride:  processDateOverride,
		SenderKey:     processSenderKey,
	}

	return runProcessWithClients(
		ctx,
		cfg,
		trimmer,
		extractor,
		fileChecker,
		driveClient,
		gmailClient,
		fileFinder,
		input,
		os.Stdout,
	)
}

// detectStartTimestamp runs the detection algorithm and returns the detected timestamp
func detectStartTimestamp(ctx context.Context, cfg *config.Config, videoPath string) (string, error) {
	// Create detection service
	detectionService := appdetection.NewService(cfg.Detection, os.Stdout)

	// Run detection
	result, err := detectionService.DetectStart(ctx, appdetection.DetectInput{
		VideoPath: videoPath,
	})
	if err != nil {
		return "", fmt.Errorf("auto-detection failed: %w\nUse --start to specify manually", err)
	}

	fmt.Fprintf(os.Stdout, "Using detected timestamp: %s\n\n", result.Timestamp)
	return result.Timestamp, nil
}

// ProcessInput contains the input parameters for process command
type ProcessInput struct {
	InputPath     string
	StartTime     string
	EndTime       string
	MinisterKey   string
	RecipientKeys []string
	CCKeys        []string
	DateOverride  string
	SenderKey     string
}

// FileFinder interface for finding files (allows testing)
type FileFinder interface {
	FindNewestFile(dir, ext string) (string, error)
	ListFiles(dir, ext string) ([]string, error)
}

// ProductionFileFinder implements FileFinder for production use
type ProductionFileFinder struct{}

func (f *ProductionFileFinder) FindNewestFile(dir, ext string) (string, error) {
	files, err := f.ListFiles(dir, ext)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no video files found in %s", dir)
	}

	// Sort by filename descending (newest by date in filename)
	sort.Slice(files, func(i, j int) bool {
		return files[i] > files[j]
	})

	return files[0], nil
}

func (f *ProductionFileFinder) ListFiles(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ext {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

// runProcessWithClients runs the process with the high-level clients (production path)
func runProcessWithClients(
	ctx context.Context,
	cfg *config.Config,
	trimmer video.Trimmer,
	extractor video.AudioExtractor,
	fileChecker video.FileChecker,
	driveClient distribution.DriveClient,
	gmailClient notification.EmailSender,
	fileFinder FileFinder,
	input ProcessInput,
	output io.Writer,
) error {
	// Verify ffmpeg is available
	if verifiable, ok := trimmer.(interface{ VerifyInstalled(context.Context) error }); ok {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifiable.VerifyInstalled(verifyCtx); err != nil {
			return fmt.Errorf("ffmpeg verification failed: %w", err)
		}
	}

	// Create file sizer
	fileSizer := &productionFileSizer{}

	// Create process service
	service := appprocess.NewService(
		trimmer,
		extractor,
		fileChecker,
		fileSizer,
		driveClient,
		gmailClient,
		&fileFinderAdapter{finder: fileFinder},
		cfg,
		output,
	)

	// Build input
	processInput := appprocess.Input{
		InputPath:     input.InputPath,
		StartTime:     input.StartTime,
		EndTime:       input.EndTime,
		MinisterKey:   input.MinisterKey,
		RecipientKeys: input.RecipientKeys,
		CCKeys:        input.CCKeys,
		DateOverride:  input.DateOverride,
		SenderKey:     input.SenderKey,
	}

	_, err := service.Process(ctx, processInput)
	return err
}

// RunProcessWithDependencies runs the process command with injected dependencies (for testing)
// This version accepts low-level service interfaces for mocking
func RunProcessWithDependencies(
	ctx context.Context,
	cfg *config.Config,
	trimmer video.Trimmer,
	extractor video.AudioExtractor,
	fileChecker video.FileChecker,
	driveService drive.DriveService,
	gmailService gmail.GmailService,
	fileFinder FileFinder,
	input ProcessInput,
	output io.Writer,
) error {
	// Verify ffmpeg is available
	if verifiable, ok := trimmer.(interface{ VerifyInstalled(context.Context) error }); ok {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifiable.VerifyInstalled(verifyCtx); err != nil {
			return fmt.Errorf("ffmpeg verification failed: %w", err)
		}
	}

	// Create Drive client wrapper
	driveClient, err := drive.NewClient(ctx, "", drive.WithDriveService(driveService))
	if err != nil {
		return fmt.Errorf("failed to create drive client: %w", err)
	}

	// Create Gmail client wrapper
	from := notification.Recipient{
		Name:    cfg.Email.FromName,
		Address: cfg.Email.FromAddress,
	}
	gmailClient := gmail.NewClient(from, gmail.WithGmailService(gmailService))

	// Create file sizer that uses the mock file checker
	fileSizer := &mockFileSizer{fileChecker: fileChecker}

	// Create process service
	service := appprocess.NewService(
		trimmer,
		extractor,
		fileChecker,
		fileSizer,
		driveClient,
		gmailClient,
		&fileFinderAdapter{finder: fileFinder},
		cfg,
		output,
	)

	// Build input
	processInput := appprocess.Input{
		InputPath:     input.InputPath,
		StartTime:     input.StartTime,
		EndTime:       input.EndTime,
		MinisterKey:   input.MinisterKey,
		RecipientKeys: input.RecipientKeys,
		CCKeys:        input.CCKeys,
		DateOverride:  input.DateOverride,
		SenderKey:     input.SenderKey,
	}

	_, err = service.Process(ctx, processInput)
	return err
}

// productionFileSizer provides file sizes using os.Stat
type productionFileSizer struct{}

func (s *productionFileSizer) Size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// mockFileSizer provides file sizes for testing using a SizeProvider interface
type mockFileSizer struct {
	fileChecker video.FileChecker
}

func (s *mockFileSizer) Size(path string) int64 {
	// Check if the file checker also implements a Size method
	if sizer, ok := s.fileChecker.(interface{ Size(string) int64 }); ok {
		return sizer.Size(path)
	}
	// Default to a reasonable size for testing
	return 1200000000 // ~1.2GB
}

// fileFinderAdapter adapts the FileFinder interface to appprocess.FileFinder
type fileFinderAdapter struct {
	finder FileFinder
}

func (a *fileFinderAdapter) FindNewestFile(dir, ext string) (string, error) {
	return a.finder.FindNewestFile(dir, ext)
}

func (a *fileFinderAdapter) ListFiles(dir, ext string) ([]string, error) {
	return a.finder.ListFiles(dir, ext)
}

// Ensure distribution.DriveClient is implemented
var _ distribution.DriveClient = (*drive.Client)(nil)
