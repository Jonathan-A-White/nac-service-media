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
	"math"
	"os"
	"path/filepath"
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
	return b.String()
}

// frameAnalyzer reads the frame at a time and matches it against the
// templates. When keep is not nil it calls it with the extracted frame's PNG
// before removing it.
type frameAnalyzer func(ctx context.Context, videoPath string, seconds int, keep func(framePath string)) (detection.FrameAnalysis, error)

// crossScan finds when the cross lights up: a coarse scan for the first lit
// frame, a binary search back to the last unlit one, then a second-by-second
// look at the edge. It keeps the coarse scan's frames in samplesDir and leaves
// them there when the cross is not found.
type crossScan struct {
	startSeconds int
	endSeconds   int
	coarseStep   int
	analyze      frameAnalyzer
	samplesDir   string // empty: keep no frames
}

func newCrossScan(cfg config.DetectionConfig, analyze frameAnalyzer, samplesDir string) crossScan {
	step := cfg.Thresholds.CoarseStepSeconds
	if step == 0 {
		step = 30 // Default to 30 seconds
	}
	return crossScan{
		startSeconds: cfg.SearchRange.StartMinutes * 60,
		endSeconds:   cfg.SearchRange.EndMinutes * 60,
		coarseStep:   step,
		analyze:      analyze,
		samplesDir:   samplesDir,
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
	look := func(seconds int, keep bool) (detection.FrameAnalysis, error) {
		var keepFn func(string)
		if keep {
			keepFn = samples.keeper(seconds)
		}
		analysis, err := s.analyze(ctx, videoPath, seconds, keepFn)
		tally.tried++
		if err != nil {
			tally.unreadable++
			tally.lastErr = err
		}
		return analysis, err
	}

	result, err := s.search(ctx, look, &tally)
	if err != nil {
		var miss *CrossNotFoundError
		if errors.As(err, &miss) {
			miss.KeepErrors, miss.KeepErr = samples.failed, samples.lastErr
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

func (s crossScan) search(ctx context.Context, look func(seconds int, keep bool) (detection.FrameAnalysis, error), tally *frameTally) (detection.DetectionResult, error) {
	// Check if cross is already lit at the very beginning (recording started late)
	earlyCheck, err := look(5, true) // Check at 5 seconds
	if err == nil && earlyCheck.State == detection.StateLit {
		// Cross is already lit - return 00:00:00 as start
		return detection.DetectionResult{
			Confidence:     earlyCheck.Confidence,
			CameraAngle:    earlyCheck.CameraAngle,
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
	low, high := firstUnlitTime, firstLitTime
	var lastLitAnalysis detection.FrameAnalysis

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

// DefaultMatchScore is the template match score (TM_CCOEFF_NORMED, 0-1) a
// frame must reach to count as showing a shot.
const DefaultMatchScore = 0.85

// templateMatch is one template's best match in a frame.
type templateMatch struct {
	Name         string  // "room_lit", "room_unlit", ...
	Score        float64 // the normalised match score
	RegionMean   float64 // mean brightness of the frame under the match
	TemplateMean float64 // mean brightness of the template
}

// classifyMatches reads a frame's template matches. The best score says
// whether a shot is in view and which. Lit or unlit is then decided by
// brightness: whichever of the shot's two templates is nearer in mean
// brightness to the frame under the match. The normalised score alone cannot
// tell a lit cross from an unlit one of the same shape, since it ignores
// brightness and contrast; with a small cross in a wide shot that is most of
// what differs.
func classifyMatches(matches []templateMatch, threshold float64, seconds int) detection.FrameAnalysis {
	if threshold == 0 {
		threshold = DefaultMatchScore
	}
	var best *templateMatch
	for i := range matches {
		m := &matches[i]
		if best == nil || m.Score > best.Score || (m.Score == best.Score && m.Name < best.Name) {
			best = m
		}
	}
	if best == nil || best.Score < threshold {
		analysis := detection.FrameAnalysis{State: detection.StateNotVisible, TimestampSeconds: seconds}
		if best != nil {
			analysis.Confidence = best.Score
		}
		return analysis
	}

	shot := strings.TrimSuffix(strings.TrimSuffix(best.Name, "_lit"), "_unlit")
	state := detection.StateUnlit
	if strings.HasSuffix(best.Name, "_lit") {
		state = detection.StateLit
	}
	var lit, unlit *templateMatch
	for i := range matches {
		switch matches[i].Name {
		case shot + "_lit":
			lit = &matches[i]
		case shot + "_unlit":
			unlit = &matches[i]
		}
	}
	if lit != nil && unlit != nil {
		if math.Abs(best.RegionMean-lit.TemplateMean) < math.Abs(best.RegionMean-unlit.TemplateMean) {
			state = detection.StateLit
		} else {
			state = detection.StateUnlit
		}
	}
	return detection.FrameAnalysis{
		State:            state,
		CameraAngle:      shot,
		Confidence:       best.Score,
		TimestampSeconds: seconds,
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
