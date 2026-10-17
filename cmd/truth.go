package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"nac-service-media/domain/detection"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"

	"github.com/spf13/cobra"
)

var (
	truthTrimmedFrom string
	truthDate        string
	truthRepo        string
)

var truthCmd = &cobra.Command{
	Use:   "truth",
	Short: "Keep a recording's true start as a test case for the cross detector",
	Long: `The truth table (testdata/truth/truth.tsv) holds recordings whose true
start a person checked, each with the frames the detector needs to find it.
go test replays every row and fails when the detector's start is more than
10 seconds from the true one, so a fixed wrong trim stays fixed.`,
}

var truthAddCmd = &cobra.Command{
	Use:          "add <recording> <hh:mm:ss>",
	Short:        "Add a recording's true start to the truth table, with its frames",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(2),
	Long: `Add a recording to the truth table after a wrong trim: give the recording
and the true start, the moment the cross lights (HH:MM:SS into the recording,
not into the trimmed video). The detector is run on the recording; the frames
around the true start and the detected one, and the coarse scan's frames up to
them, are cut into testdata/truth/<recording>/ and the row is added (or
replaces the recording's row). The detector is then run again on the frames
alone, to say whether go test passes on the new row: after a wrong trim it
fails until the detector is fixed.

Run it in the nac-service-media folder (or give --repo), then commit
testdata/truth.

Examples:
  nac-service-media truth add "2026-09-27 10-22-05.mp4" 00:08:42 --trimmed-from 00:02:54
  nac-service-media truth add clip.mp4 00:05:48 --date 2026-10-18`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		if cfg == nil {
			return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
		}
		run := truthRun{
			cfg:         cfg,
			recording:   args[0],
			trueStart:   args[1],
			trimmedFrom: truthTrimmedFrom,
			date:        truthDate,
			repo:        truthRepo,
			out:         os.Stdout,
		}
		return run.execute(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(truthCmd)
	truthCmd.AddCommand(truthAddCmd)
	truthAddCmd.Flags().StringVar(&truthTrimmedFrom, "trimmed-from", "", "Where the wrong trim started, HH:MM:SS into the recording (its frames are kept too)")
	truthAddCmd.Flags().StringVar(&truthDate, "date", "", "The service's date, YYYY-MM-DD (default: from the recording's name)")
	truthAddCmd.Flags().StringVar(&truthRepo, "repo", ".", "The nac-service-media folder whose testdata/truth gets the case")
}

// truthRun is one truth add: run the detector on the recording, keep the
// frames, add the row, and replay the frames.
type truthRun struct {
	cfg         *config.Config
	recording   string
	trueStart   string
	trimmedFrom string
	date        string
	repo        string
	out         io.Writer
	frames      infradetection.FrameExtractor // nil: ffmpeg
}

func (r truthRun) execute(ctx context.Context) error {
	trueStart, err := video.ParseTimestamp(r.trueStart)
	if err != nil {
		return fmt.Errorf("the true start: %w", err)
	}
	trimmedFrom := -1
	if r.trimmedFrom != "" {
		ts, err := video.ParseTimestamp(r.trimmedFrom)
		if err != nil {
			return fmt.Errorf("--trimmed-from: %w", err)
		}
		trimmedFrom = ts.TotalSeconds()
	}
	videoPath, err := detectRun{cfg: r.cfg, input: r.recording}.resolveVideo()
	if err != nil {
		return err
	}
	date, err := r.serviceDate(videoPath)
	if err != nil {
		return err
	}
	frames := r.frames
	if frames == nil {
		frames = infradetection.FFmpegFrames{}
	}
	c := infradetection.TruthCase{Date: date, Recording: filepath.Base(videoPath), TrueStart: trueStart.TotalSeconds(), TrimmedFrom: trimmedFrom}

	fmt.Fprintf(r.out, "Recording: %s\nRunning the detector on the recording...\n", videoPath)
	res, err := r.detect(ctx, frames, videoPath, r.out)
	switch {
	case notFound(err):
		c.Detected = -1
		fmt.Fprintln(r.out, "The detector found no start on the recording.")
	case err != nil:
		return err
	default:
		c.Detected = res.Timestamp.TotalSeconds()
		fmt.Fprintf(r.out, "Detected start on the recording: %s\n", res.Timestamp)
	}

	truthDir := filepath.Join(r.repo, "testdata", "truth")
	framesDir := c.FramesDir(truthDir)
	if err := r.keepFrames(ctx, frames, videoPath, c, framesDir); err != nil {
		return err
	}
	table := filepath.Join(truthDir, infradetection.TruthTableFile)
	if err := infradetection.WriteTruthCase(table, c); err != nil {
		return fmt.Errorf("failed to write the truth table: %w", err)
	}
	fmt.Fprintf(r.out, "Wrote the row for %s (true start %s) to %s\n", c.Recording, secondsToTimestamp(c.TrueStart), table)

	return r.replay(ctx, c, framesDir)
}

// serviceDate is --date, or the date the recording's name begins with.
func (r truthRun) serviceDate(videoPath string) (string, error) {
	date := r.date
	if date == "" {
		base := filepath.Base(videoPath)
		if len(base) >= 10 {
			date = base[:10]
		}
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", fmt.Errorf("the service's date: %q is not YYYY-MM-DD; give --date when the recording's name does not begin with it", date)
	}
	return date, nil
}

// detect runs the cross detector on the frames.
func (r truthRun) detect(ctx context.Context, frames infradetection.FrameExtractor, videoPath string, log io.Writer) (detection.DetectionResult, error) {
	detector := infradetection.NewTemplateDetector(r.cfg.Detection,
		infradetection.WithFrameExtractor(frames), infradetection.WithOutput(log), infradetection.WithSamplesDir(""))
	if err := detector.LoadTemplates(r.cfg.Detection.TemplatesDir); err != nil {
		return detection.DetectionResult{}, fmt.Errorf("failed to load detection templates: %w", err)
	}
	defer detector.Close()
	return detector.DetectStart(ctx, videoPath)
}

// notFound says whether a search looked and did not see the cross light,
// as against one that could not run.
func notFound(err error) bool {
	var miss *infradetection.CrossNotFoundError
	return errors.As(err, &miss)
}

// keepFrames cuts the case's frames into framesDir, replacing what was there
// only once every frame is cut.
func (r truthRun) keepFrames(ctx context.Context, frames infradetection.FrameExtractor, videoPath string, c infradetection.TruthCase, framesDir string) error {
	cutDir := framesDir + ".new"
	if err := os.RemoveAll(cutDir); err != nil {
		return err
	}
	defer os.RemoveAll(cutDir)
	tmp, err := os.MkdirTemp("", "nac-truth-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	times := infradetection.TruthFrameTimes(c, r.cfg.Detection.Thresholds.CoarseStepSeconds, r.cfg.Detection.SearchRange.EndMinutes*60)
	fmt.Fprintf(r.out, "Cutting %d frames into %s...\n", len(times), framesDir)
	kept, missed := 0, 0
	for _, seconds := range times {
		if err := ctx.Err(); err != nil {
			return err
		}
		at := secondsToTimestamp(seconds)
		framePath := filepath.Join(tmp, "frame.png")
		err := frames.ExtractFrame(ctx, videoPath, at, framePath)
		if err == nil {
			err = infradetection.SaveTruthFrame(framePath, cutDir, seconds)
		}
		if err != nil {
			if seconds == c.TrueStart {
				return fmt.Errorf("could not keep the frame at the true start %s: %w", at, err)
			}
			missed++
			continue
		}
		kept++
	}
	if err := os.RemoveAll(framesDir); err != nil {
		return err
	}
	if err := os.Rename(cutDir, framesDir); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Kept %d frames", kept)
	if missed > 0 {
		fmt.Fprintf(r.out, "; %d could not be cut (past the end of the recording?)", missed)
	}
	fmt.Fprintln(r.out)
	return nil
}

// replay runs the detector on the kept frames alone, as go test does, and
// says whether the case passes.
func (r truthRun) replay(ctx context.Context, c infradetection.TruthCase, framesDir string) error {
	replay, err := infradetection.NewReplayFrames(framesDir)
	if err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Running the detector on the kept frames alone, as go test does...")
	res, err := r.detect(ctx, replay, c.Recording, io.Discard)
	found := -1
	switch {
	case notFound(err):
		fmt.Fprintln(r.out, "Replayed from the saved frames: no start")
	case err != nil:
		return fmt.Errorf("replaying the kept frames: %w", err)
	default:
		found = res.Timestamp.TotalSeconds()
		fmt.Fprintf(r.out, "Replayed from the saved frames: %s\n", res.Timestamp)
	}
	if found != c.Detected {
		fmt.Fprintln(r.out, "Note: on the frames alone the detector does not find what it found on the recording; the kept frames do not show all it saw there.")
	}
	if miss := infradetection.TruthMiss(c, res, err); miss != "" {
		fmt.Fprintf(r.out, "%s\ngo test fails on this row until the detector finds the true start: commit it with the fix.\n", miss)
		return nil
	}
	fmt.Fprintf(r.out, "That is within %d s of the true start: go test passes on this row.\n", infradetection.TruthToleranceSeconds)
	return nil
}
