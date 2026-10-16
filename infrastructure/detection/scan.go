package detection

// The search for the cross lighting up, and how its results are read. This
// file has no build tag: the GoCV detector supplies one frame's analysis, and
// everything else here is tested by the gate.

import (
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nac-service-media/domain/detection"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// ErrNothingAnalysed marks a search that could not look at the video at all
// (no frame could be read, no templates, the amen script could not run), so
// it never reads as "not found".
var ErrNothingAnalysed = errors.New("nothing was analysed")

// CrossNotFoundError is a cross search that looked at the video and did not
// see the cross light up.
type CrossNotFoundError struct {
	Analysed   int   // frames read and matched
	Unlit      int   // of those, frames that matched an unlit template
	Unreadable int   // frames that could not be extracted or read
	LastErr    error // the last unreadable frame's error

	FramesDir  string // where the sampled frames were left; empty when none
	FramesKept int
	KeepErrors int   // sampled frames that could not be saved
	KeepErr    error // the last of those errors

	Threshold float64        // the score a frame had to reach to show the cross
	Best      []TemplateBest // each picture's best over the search, highest first
}

// TemplateBest is the best one picture of the cross scored over a search:
// the score, at which scale, in the frame at which time.
type TemplateBest struct {
	Name    string
	Score   float64
	Scale   float64
	Seconds int
}

func (e *CrossNotFoundError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "could not detect cross lighting up in search range (looked at %d frames: %d matched an unlit shot, %d matched no shot",
		e.Analysed, e.Unlit, e.Analysed-e.Unlit)
	if e.Unreadable > 0 {
		fmt.Fprintf(&b, "; %d could not be read, the last: %v", e.Unreadable, e.LastErr)
	}
	b.WriteString(")")
	if e.FramesDir != "" {
		fmt.Fprintf(&b, "; the %d frames it sampled are in %q, named by their time in the video", e.FramesKept, e.FramesDir)
	}
	if e.KeepErrors > 0 {
		fmt.Fprintf(&b, "; %d sampled frames could not be saved, the last: %v", e.KeepErrors, e.KeepErr)
	}
	if len(e.Best) > 0 {
		fmt.Fprintf(&b, "\nThe best each picture of the cross scored (a frame shows the cross from %.2f):", e.Threshold)
		width := 0
		for _, t := range e.Best {
			width = max(width, len(t.Name))
		}
		for _, t := range e.Best {
			fmt.Fprintf(&b, "\n  %-*s  %.2f at %.2fx in the frame at %s", width, t.Name, t.Score, t.Scale, secondsToTimestamp(t.Seconds))
		}
	}
	return b.String()
}

// secondsToTimestamp writes a time in the video as HH:MM:SS.
func secondsToTimestamp(seconds int) string {
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, (seconds%3600)/60, seconds%60)
}

// frameAnalyzer reads the frame at a time and matches it against the
// templates. When keep is not nil it calls it with the extracted frame's PNG
// before removing it.
type frameAnalyzer func(ctx context.Context, videoPath string, seconds int, keep func(framePath string)) (frameReport, error)

// crossScan finds when the cross lights up: a coarse scan for the first lit
// frame, a binary search back to the last unlit one, then a second-by-second
// look at the edge. It keeps the coarse scan's frames in samplesDir and leaves
// them there when the cross is not found.
type crossScan struct {
	startSeconds int
	endSeconds   int
	coarseStep   int
	threshold    float64
	analyze      frameAnalyzer
	samplesDir   string    // empty: keep no frames
	out          io.Writer // one line per frame looked at
}

func newCrossScan(cfg config.DetectionConfig, analyze frameAnalyzer, samplesDir string, out io.Writer) crossScan {
	step := cfg.Thresholds.CoarseStepSeconds
	if step == 0 {
		step = 30 // Default to 30 seconds
	}
	threshold := cfg.Thresholds.MatchScore
	if threshold == 0 {
		threshold = DefaultMatchScore
	}
	if out == nil {
		out = io.Discard
	}
	return crossScan{
		startSeconds: cfg.SearchRange.StartMinutes * 60,
		endSeconds:   cfg.SearchRange.EndMinutes * 60,
		coarseStep:   step,
		threshold:    threshold,
		analyze:      analyze,
		samplesDir:   samplesDir,
		out:          out,
	}
}

// frameLine is the log's one line for a frame: the time, what it showed, and
// the match behind that.
func (s crossScan) frameLine(r frameReport) string {
	at := secondsToTimestamp(r.TimestampSeconds)
	best := r.best()
	switch r.State {
	case detection.StateLit, detection.StateUnlit:
		return fmt.Sprintf("  %s  %-5s  %s %.2fx score %.2f (cross %.0f%% lit)", at, r.State, r.CameraAngle, best.Scale, best.Score, 100*math.Max(0, math.Min(1, r.Litness)))
	default:
		if best.Name == "" {
			return fmt.Sprintf("  %s  not in view  (no picture of the cross fits a frame this size)", at)
		}
		return fmt.Sprintf("  %s  not in view  best %s %.2f at %.2fx (threshold %.2f)", at, best.Name, best.Score, best.Scale, s.threshold)
	}
}

// sampleFramesDir is the folder beside the video where a missed search leaves
// the frames it sampled.
func sampleFramesDir(videoPath string) string {
	base := filepath.Base(videoPath)
	return filepath.Join(filepath.Dir(videoPath), strings.TrimSuffix(base, filepath.Ext(base))+" cross frames")
}

// frameTally counts the frames a search tried and the ones it could not read.
type frameTally struct {
	tried, unreadable, unlit int
	lastErr                  error
}

func (s crossScan) run(ctx context.Context, videoPath string) (detection.DetectionResult, error) {
	samples := &frameSamples{dir: s.samplesDir}
	var tally frameTally
	bests := map[string]TemplateBest{}
	look := func(seconds int, keep bool) (frameReport, error) {
		var keepFn func(string)
		if keep {
			keepFn = samples.keeper(seconds)
		}
		report, err := s.analyze(ctx, videoPath, seconds, keepFn)
		tally.tried++
		if err != nil {
			tally.unreadable++
			tally.lastErr = err
			fmt.Fprintf(s.out, "  %s  could not read the frame: %v\n", secondsToTimestamp(seconds), err)
			return report, err
		}
		for _, m := range report.Matches {
			if prev, ok := bests[m.Name]; !ok || m.Score > prev.Score {
				bests[m.Name] = TemplateBest{Name: m.Name, Score: m.Score, Scale: m.Scale, Seconds: seconds}
			}
		}
		fmt.Fprintln(s.out, s.frameLine(report))
		return report, nil
	}

	result, err := s.search(ctx, look, &tally)
	if err != nil {
		var miss *CrossNotFoundError
		if errors.As(err, &miss) {
			miss.KeepErrors, miss.KeepErr = samples.failed, samples.lastErr
			miss.Threshold = s.threshold
			for _, b := range bests {
				miss.Best = append(miss.Best, b)
			}
			sort.Slice(miss.Best, func(i, j int) bool {
				if miss.Best[i].Score != miss.Best[j].Score {
					return miss.Best[i].Score > miss.Best[j].Score
				}
				return miss.Best[i].Name < miss.Best[j].Name
			})
			if samples.kept > 0 {
				miss.FramesDir, miss.FramesKept = samples.dir, samples.kept
				return result, err
			}
		}
		samples.discard()
		return result, err
	}
	samples.discard()
	return result, nil
}

func (s crossScan) search(ctx context.Context, look func(seconds int, keep bool) (frameReport, error), tally *frameTally) (detection.DetectionResult, error) {
	// Check if cross is already lit at the very beginning (recording started late)
	earlyCheck, err := look(5, true) // Check at 5 seconds
	if err == nil && earlyCheck.State == detection.StateLit {
		// Cross is already lit - return 00:00:00 as start
		return detection.DetectionResult{
			Confidence:     earlyCheck.Confidence,
			CameraAngle:    earlyCheck.CameraAngle,
			Scale:          earlyCheck.Scale,
			FramesAnalyzed: tally.tried,
		}, nil
	}

	// Phase 1: Coarse scan to find bounds
	var firstUnlitTime, firstLitTime int
	foundUnlit, foundLit := false, false

	// Start from 0 if the early check showed unlit or not visible
	scanStart := 0
	if err == nil && earlyCheck.State == detection.StateUnlit {
		firstUnlitTime = 5
		foundUnlit = true
		tally.unlit++
		scanStart = s.coarseStep // Skip ahead since we already checked the beginning
	}

	for t := scanStart; t <= s.endSeconds; t += s.coarseStep {
		if err := ctx.Err(); err != nil {
			return detection.DetectionResult{}, err
		}

		analysis, err := look(t, true)
		if err != nil {
			continue // Counted by look; named in the error if the cross is not found
		}

		if analysis.State == detection.StateUnlit {
			tally.unlit++
			if !foundUnlit {
				firstUnlitTime = t
				foundUnlit = true
			}
		}
		if analysis.State == detection.StateLit && !foundLit {
			firstLitTime = t
			foundLit = true
			break // Found lit, we have our bounds
		}
	}

	if !foundLit {
		analysed := tally.tried - tally.unreadable
		if analysed == 0 {
			return detection.DetectionResult{FramesAnalyzed: tally.tried}, fmt.Errorf("cross search could not run: %d of %d frames could not be read (the last: %v): %w",
				tally.unreadable, tally.tried, tally.lastErr, ErrNothingAnalysed)
		}
		return detection.DetectionResult{FramesAnalyzed: tally.tried}, &CrossNotFoundError{
			Analysed:   analysed,
			Unlit:      tally.unlit,
			Unreadable: tally.unreadable,
			LastErr:    tally.lastErr,
		}
	}

	// If we found lit but no unlit, search backwards
	if !foundUnlit {
		for t := firstLitTime - s.coarseStep; t >= s.startSeconds; t -= s.coarseStep {
			analysis, err := look(t, false)
			if err != nil {
				continue
			}
			if analysis.State == detection.StateUnlit || analysis.State == detection.StateNotVisible {
				firstUnlitTime = t
				foundUnlit = true
				break
			}
		}
		if !foundUnlit {
			firstUnlitTime = s.startSeconds
		}
	}

	// Phase 2: Binary search to narrow down
	fmt.Fprintln(s.out, "  Phase 2: Binary search...")
	low, high := firstUnlitTime, firstLitTime
	var lastLitAnalysis frameReport

	for high-low > 1 {
		if err := ctx.Err(); err != nil {
			return detection.DetectionResult{}, err
		}

		mid := (low + high) / 2
		analysis, err := look(mid, false)
		if err != nil {
			// On error, bias towards the lit end
			low = mid
			continue
		}

		if analysis.State == detection.StateLit {
			high = mid
			lastLitAnalysis = analysis
		} else {
			low = mid
		}
	}

	// Phase 3: Refinement - scan second by second around the transition
	fmt.Fprintln(s.out, "  Phase 3: Refining...")
	transitionTime := high
	for t := high - 2; t <= high+2; t++ {
		if t < low {
			continue
		}
		analysis, err := look(t, false)
		if err != nil {
			continue
		}
		if analysis.State == detection.StateLit {
			transitionTime = t
			lastLitAnalysis = analysis
			break
		}
	}

	return detection.DetectionResult{
		Timestamp: video.Timestamp{
			Hours:   transitionTime / 3600,
			Minutes: (transitionTime % 3600) / 60,
			Seconds: transitionTime % 60,
		},
		Confidence:     lastLitAnalysis.Confidence,
		CameraAngle:    lastLitAnalysis.CameraAngle,
		Scale:          lastLitAnalysis.Scale,
		FramesAnalyzed: tally.tried,
	}, nil
}

// frameSamples saves the frames a search sampled as JPEGs named by their time
// (HH-MM-SS.jpg), so a person can find the lit and unlit times by eye.
type frameSamples struct {
	dir     string
	ready   bool
	kept    int
	failed  int
	lastErr error
}

func (s *frameSamples) keeper(seconds int) func(framePath string) {
	if s.dir == "" {
		return nil
	}
	return func(framePath string) {
		if err := s.keep(framePath, seconds); err != nil {
			s.failed++
			s.lastErr = err
			return
		}
		s.kept++
	}
}

func (s *frameSamples) keep(framePath string, seconds int) error {
	if !s.ready {
		// Start clean: a folder left by an earlier miss on this video goes.
		if err := os.RemoveAll(s.dir); err != nil {
			return err
		}
		if err := os.MkdirAll(s.dir, 0o755); err != nil {
			return err
		}
		s.ready = true
	}
	in, err := os.Open(framePath)
	if err != nil {
		return err
	}
	defer in.Close()
	img, err := png.Decode(in)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%02d-%02d-%02d.jpg", seconds/3600, (seconds%3600)/60, seconds%60)
	out, err := os.Create(filepath.Join(s.dir, name))
	if err != nil {
		return err
	}
	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: 85}); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (s *frameSamples) discard() {
	if s.ready {
		os.RemoveAll(s.dir)
	}
}

// amenNotDetected reads the amen script's "not detected" reason: a search that
// ran and matched too weakly is a miss; anything else (video too short, no
// template, audio not extracted, missing packages) means it analysed nothing.
func amenNotDetected(reason string) (detection.EndDetectionResult, error) {
	if strings.HasPrefix(reason, "Best match score") {
		return detection.EndDetectionResult{Detected: false, Error: reason}, nil
	}
	return detection.EndDetectionResult{}, fmt.Errorf("amen search could not run: %s (%w)", reason, ErrNothingAnalysed)
}
