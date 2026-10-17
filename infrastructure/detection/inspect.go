package detection

// Looking at single frames: what every picture of the cross scores in one
// frame, for the detect command's --at and --every, and the option that
// keeps a miss from leaving frames beside the video.

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// WithSamplesDir is where a miss leaves the frames it sampled; the empty
// string keeps none. Without it they go in a folder beside the video.
func WithSamplesDir(dir string) TemplateDetectorOption {
	return func(d *TemplateDetector) {
		d.samplesDir = &dir
	}
}

// samplesDirFor is where a miss on this video leaves its sampled frames.
func (d *TemplateDetector) samplesDirFor(videoPath string) string {
	if d.samplesDir != nil {
		return *d.samplesDir
	}
	return sampleFramesDir(videoPath)
}

// FrameInspection is one frame as the detector sees it: the log line the
// search would write for it, and one line per picture of the cross with its
// best match over every scale.
type FrameInspection struct {
	Seconds int
	State   string // lit, unlit or not_visible
	Line    string
	Details []string
}

// Inspect reads the frame at a time and reports what every picture scored.
// When savePath is not empty the frame's PNG is written there too.
func (d *TemplateDetector) Inspect(ctx context.Context, videoPath string, seconds int, savePath string) (FrameInspection, error) {
	if d.matcher == nil {
		return FrameInspection{}, fmt.Errorf("no templates loaded (%w)", ErrNothingAnalysed)
	}
	if d.tempDir == "" {
		var err error
		d.tempDir, err = os.MkdirTemp("", "nac-detection-*")
		if err != nil {
			return FrameInspection{}, fmt.Errorf("failed to create temp directory: %w (%w)", err, ErrNothingAnalysed)
		}
	}
	var keep func(string)
	var saveErr error
	if savePath != "" {
		keep = func(framePath string) { saveErr = copyFile(framePath, savePath) }
	}
	report, err := d.analyzeFrame(ctx, videoPath, seconds, keep)
	if err != nil {
		return FrameInspection{}, err
	}
	if saveErr != nil {
		return FrameInspection{}, fmt.Errorf("could not save the frame at %s: %w", secondsToTimestamp(seconds), saveErr)
	}
	scan := crossScan{threshold: d.matcher.threshold}
	return FrameInspection{
		Seconds: seconds,
		State:   string(report.State),
		Line:    scan.frameLine(report),
		Details: report.describe(),
	}, nil
}

// describe is one line per picture of the cross: its best score and scale,
// where in the frame, and the contrast there set between the shot's unlit
// and lit pictures.
func (r frameReport) describe() []string {
	width := 0
	for _, m := range r.Matches {
		width = max(width, len(m.Name))
	}
	var lines []string
	for _, m := range r.Matches {
		lines = append(lines, fmt.Sprintf("    %-*s  %.2f at %.2fx, %dx%d at (%d,%d), contrast %.2f (unlit picture %.2f, lit %.2f: %.2f lit)",
			width, m.Name, m.Score, m.Scale, m.Width, m.Height, m.X, m.Y, m.Contrast, m.UnlitContrast, m.LitContrast, m.litFraction()))
	}
	return lines
}

// WriteInspection prints an inspection: the frame's line, then its details.
func WriteInspection(out io.Writer, in FrameInspection) {
	fmt.Fprintln(out, in.Line)
	for _, line := range in.Details {
		fmt.Fprintln(out, line)
	}
}

// copyFile copies src to dst, making dst's folder.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// clampPercent is a 0-1 fraction as a whole percentage between 0 and 100.
func clampPercent(f float64) float64 {
	return 100 * math.Max(0, math.Min(1, f))
}
