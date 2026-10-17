package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	appdetection "nac-service-media/application/detection"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"

	"github.com/spf13/cobra"
)

var (
	detectInput string
	detectAt    []string
	detectEvery int
	detectSave  string
)

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Find the service start in a recording without trimming it",
	Long: `Run the cross detector on a recording and print what it sees: one line per
frame it looks at, then the start it would trim from. Nothing is trimmed,
uploaded, emailed or deleted, and no frames are left beside the video.

With --at, only the named frames are looked at, and every picture of the
cross says what it scored in each. With --every N, every Nth second of the
search range is looked at and reported, with no search for the start.

Examples:
  nac-service-media detect --input "2026-09-27 10-22-05.mp4"
  nac-service-media detect --input "2026-09-27 10-22-05.mp4" --at 00:02:54,00:08:42 --save-frames /tmp/frames
  nac-service-media detect --input "2026-09-27 10-22-05.mp4" --every 30`,
	RunE: runDetect,
}

func init() {
	rootCmd.AddCommand(detectCmd)
	detectCmd.Flags().StringVar(&detectInput, "input", "", "The recording: a path, or a file in the source directory (default: the newest there)")
	detectCmd.Flags().StringSliceVar(&detectAt, "at", nil, "Look only at these frames, HH:MM:SS each")
	detectCmd.Flags().IntVar(&detectEvery, "every", 0, "Look at every Nth second of the search range and report each frame")
	detectCmd.Flags().StringVar(&detectSave, "save-frames", "", "With --at or --every: save each frame looked at as HH-MM-SS.png in this folder")
}

func runDetect(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}
	run := detectRun{
		cfg:    cfg,
		input:  detectInput,
		at:     detectAt,
		every:  detectEvery,
		save:   detectSave,
		out:    os.Stdout,
		finder: &ProductionFileFinder{},
	}
	return run.execute(cmd.Context())
}

// detectRun is one detect command: the full search, or a look at the frames
// named by --at or spaced by --every.
type detectRun struct {
	cfg    *config.Config
	input  string
	at     []string
	every  int
	save   string
	out    io.Writer
	finder FileFinder
	frames infradetection.FrameExtractor // nil: ffmpeg
}

func (r detectRun) execute(ctx context.Context) error {
	videoPath, err := r.resolveVideo()
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Recording: %s\n", videoPath)
	if len(r.at) == 0 && r.every == 0 {
		return r.search(ctx, videoPath)
	}
	if len(r.at) > 0 && r.every > 0 {
		return fmt.Errorf("--at and --every cannot be used together")
	}
	return r.inspect(ctx, videoPath)
}

// resolveVideo is the recording: the path given, a file of the source
// directory, or the newest file there.
func (r detectRun) resolveVideo() (string, error) {
	if r.input == "" {
		newest, err := r.finder.FindNewestFile(r.cfg.Paths.SourceDirectory, ".mp4")
		if err != nil {
			return "", fmt.Errorf("failed to find video file: %w", err)
		}
		return newest, nil
	}
	path := r.input
	if !filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join(r.cfg.Paths.SourceDirectory, path)
		}
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("recording not found: %w", err)
	}
	return path, nil
}

// search runs the start detector as process does, and prints the start.
func (r detectRun) search(ctx context.Context, videoPath string) error {
	service := appdetection.NewService(r.cfg.Detection, r.out)
	result, err := service.DetectStart(ctx, appdetection.DetectInput{VideoPath: videoPath, LeaveNoFrames: true, Frames: r.frames})
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Start: %s\n", result.Timestamp)
	return nil
}

// inspect looks at the frames of --at, or every Nth second of the search
// range, and reports each one.
func (r detectRun) inspect(ctx context.Context, videoPath string) error {
	opts := []infradetection.TemplateDetectorOption{infradetection.WithOutput(r.out), infradetection.WithSamplesDir("")}
	if r.frames != nil {
		opts = append(opts, infradetection.WithFrameExtractor(r.frames))
	}
	detector := infradetection.NewTemplateDetector(r.cfg.Detection, opts...)
	if err := detector.LoadTemplates(r.cfg.Detection.TemplatesDir); err != nil {
		return fmt.Errorf("failed to load detection templates: %w", err)
	}
	defer detector.Close()

	times, err := r.times()
	if err != nil {
		return err
	}
	for _, seconds := range times {
		if err := ctx.Err(); err != nil {
			return err
		}
		at := secondsToTimestamp(seconds)
		savePath := ""
		if r.save != "" {
			savePath = filepath.Join(r.save, fmt.Sprintf("%02d-%02d-%02d.png", seconds/3600, (seconds%3600)/60, seconds%60))
		}
		in, err := detector.Inspect(ctx, videoPath, seconds, savePath)
		if err != nil {
			fmt.Fprintf(r.out, "  %s  could not read the frame: %v\n", at, err)
			continue
		}
		if r.every > 0 {
			fmt.Fprintln(r.out, in.Line)
			continue
		}
		infradetection.WriteInspection(r.out, in)
	}
	return nil
}

// times are the seconds to look at: --at's, or every Nth from the search
// range's start to its end.
func (r detectRun) times() ([]int, error) {
	var times []int
	for _, at := range r.at {
		ts, err := video.ParseTimestamp(at)
		if err != nil {
			return nil, fmt.Errorf("--at %q: %w", at, err)
		}
		times = append(times, ts.TotalSeconds())
	}
	if r.every > 0 {
		for t := r.cfg.Detection.SearchRange.StartMinutes * 60; t <= r.cfg.Detection.SearchRange.EndMinutes*60; t += r.every {
			times = append(times, t)
		}
	}
	return times, nil
}
