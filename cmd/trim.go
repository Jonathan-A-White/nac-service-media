package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	appvideo "nac-service-media/application/video"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/ffmpeg"
	"nac-service-media/infrastructure/filesystem"

	"github.com/spf13/cobra"
)

var (
	trimSourcePath string
	trimStartTime  string
	trimEndTime    string
)

var trimCmd = &cobra.Command{
	Use:   "trim",
	Short: "Trim a video to specified timestamps",
	Long: `Trim a video file to the specified start and end timestamps.

The source filename must be in OBS format: YYYY-MM-DD HH-MM-SS.mp4
The output file will be named YYYY-MM-DD.mp4 in the configured trimmed directory.

Example:
  nac-service-media trim --source "/path/to/2025-12-28 10-06-16.mp4" --start "00:05:30" --end "01:45:00"`,
	RunE: runTrim,
}

func init() {
	rootCmd.AddCommand(trimCmd)
	trimCmd.Flags().StringVar(&trimSourcePath, "source", "", "Path to source video file (required)")
	trimCmd.Flags().StringVar(&trimStartTime, "start", "", "Start timestamp in HH:MM:SS format (required)")
	trimCmd.Flags().StringVar(&trimEndTime, "end", "", "End timestamp in HH:MM:SS format (required)")
	trimCmd.MarkFlagRequired("source")
	trimCmd.MarkFlagRequired("start")
	trimCmd.MarkFlagRequired("end")
}

func runTrim(cmd *cobra.Command, args []string) error {
	// Ensure config is loaded
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}

	// Create dependencies using production implementations
	trimmer := ffmpeg.NewTrimmer()
	fileChecker := filesystem.NewChecker()

	return RunTrimWithDependencies(
		cmd.Context(),
		trimmer,
		fileChecker,
		cfg.Paths.TrimmedDirectory,
		trimSourcePath,
		trimStartTime,
		trimEndTime,
		os.Stdout,
	)
}

// OutputWriter allows capturing output in tests
type OutputWriter interface {
	Write(p []byte) (n int, err error)
}

// RunTrimWithDependencies runs the trim command with injected dependencies (for testing)
func RunTrimWithDependencies(
	ctx context.Context,
	trimmer video.Trimmer,
	fileChecker video.FileChecker,
	outputDir string,
	sourcePath string,
	startTime string,
	endTime string,
	output OutputWriter,
) error {
	// Verify ffmpeg is available if trimmer supports it
	if verifiable, ok := trimmer.(interface{ VerifyInstalled(context.Context) error }); ok {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifiable.VerifyInstalled(verifyCtx); err != nil {
			return fmt.Errorf("ffmpeg verification failed: %w", err)
		}
	}

	// Create service with injected dependencies
	service := appvideo.NewTrimService(trimmer, fileChecker, outputDir)

	// Perform trim
	input := appvideo.TrimInput{
		SourcePath: sourcePath,
		StartTime:  startTime,
		EndTime:    endTime,
	}

	fmt.Fprintf(output, "Trimming video from %s to %s...\n", startTime, endTime)

	result, err := service.Trim(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(output, "Successfully created: %s\n", result.OutputPath)
	return nil
}
