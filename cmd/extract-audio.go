package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appvideo "nac-service-media/application/video"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/ffmpeg"
	"nac-service-media/infrastructure/filesystem"

	"github.com/spf13/cobra"
)

var (
	extractSourcePath string
	extractBitrate    string
	extractDate       string
)

var extractAudioCmd = &cobra.Command{
	Use:   "extract-audio",
	Short: "Extract audio from a video file",
	Long: `Extract audio from a video file to MP3 format.

The source should be a trimmed video file. The output will be saved to the
configured audio directory with the service date as the filename.

If --source is just a filename, it will be resolved from the configured trimmed_directory.

Example:
  nac-service-media extract-audio --source "2025-12-28.mp4"
  nac-service-media extract-audio --source "/path/to/video.mp4" --date "2025-12-28" --bitrate "128k"`,
	RunE: runExtractAudio,
}

func init() {
	rootCmd.AddCommand(extractAudioCmd)
	extractAudioCmd.Flags().StringVar(&extractSourcePath, "source", "", "Path to source video file (required)")
	extractAudioCmd.Flags().StringVar(&extractBitrate, "bitrate", "", "Audio bitrate (default from config or 192k)")
	extractAudioCmd.Flags().StringVar(&extractDate, "date", "", "Service date in YYYY-MM-DD format (defaults to parsing from filename)")
	extractAudioCmd.MarkFlagRequired("source")
}

func runExtractAudio(cmd *cobra.Command, args []string) error {
	// Ensure config is loaded
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}

	// Resolve source path - if not absolute, use trimmed_directory from config
	sourcePath := extractSourcePath
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(cfg.Paths.TrimmedDirectory, sourcePath)
	}

	// Determine bitrate
	bitrate := extractBitrate
	if bitrate == "" {
		bitrate = cfg.Audio.Bitrate
	}
	if bitrate == "" {
		bitrate = video.DefaultAudioBitrate
	}

	// Parse service date
	var serviceDate time.Time
	var err error
	if extractDate != "" {
		serviceDate, err = time.Parse("2006-01-02", extractDate)
		if err != nil {
			return fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
		}
	} else {
		// Try to parse from filename (YYYY-MM-DD.mp4)
		serviceDate, err = parseDateFromFilename(filepath.Base(sourcePath))
		if err != nil {
			return fmt.Errorf("could not determine service date from filename; use --date flag: %w", err)
		}
	}

	// Create dependencies using production implementations
	extractor := ffmpeg.NewExtractor()
	fileChecker := filesystem.NewChecker()

	return RunExtractAudioWithDependencies(
		cmd.Context(),
		extractor,
		fileChecker,
		cfg.Paths.AudioDirectory,
		bitrate,
		sourcePath,
		serviceDate,
		os.Stdout,
	)
}

// parseDateFromFilename extracts the date from a filename in YYYY-MM-DD.ext format
func parseDateFromFilename(filename string) (time.Time, error) {
	// Remove extension
	name := filename[:len(filename)-len(filepath.Ext(filename))]

	// Try to parse as date
	return time.Parse("2006-01-02", name)
}

// RunExtractAudioWithDependencies runs the extract-audio command with injected dependencies (for testing)
func RunExtractAudioWithDependencies(
	ctx context.Context,
	extractor video.AudioExtractor,
	fileChecker video.FileChecker,
	outputDir string,
	bitrate string,
	sourcePath string,
	serviceDate time.Time,
	output OutputWriter,
) error {
	// Verify ffmpeg is available if extractor supports it
	if verifiable, ok := extractor.(interface{ VerifyInstalled(context.Context) error }); ok {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifiable.VerifyInstalled(verifyCtx); err != nil {
			return fmt.Errorf("ffmpeg verification failed: %w", err)
		}
	}

	// Create service with injected dependencies
	service := appvideo.NewExtractService(extractor, fileChecker, outputDir, bitrate)

	// Perform extraction
	input := appvideo.ExtractInput{
		SourcePath:  sourcePath,
		ServiceDate: serviceDate,
		Bitrate:     bitrate,
	}

	fmt.Fprintf(output, "Extracting audio from %s with bitrate %s...\n", sourcePath, bitrate)

	result, err := service.Extract(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(output, "Successfully created: %s\n", result.OutputPath)
	return nil
}
