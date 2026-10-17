package detection

// The search for the cross lighting up, and how its results are read. This
// file has no build tag: the GoCV detector supplies one frame's analysis, and
// everything else here is tested by the gate.

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
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
	Fleeting   int   // frames that showed the cross lit, but elsewhere than it was seen unlit, or not for long
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
	if e.Fleeting > 0 {
		fmt.Fprintf(&b, "; %d showed the cross lit, but elsewhere than it was seen unlit or not staying lit for %d s (one lit frame never decides the start)", e.Fleeting, confirmSeconds)
	}
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
		return fmt.Sprintf("  %s  %-5s  %s %.2fx score %.2f (cross %.0f%% lit) at (%d,%d)", at, r.State, r.CameraAngle, best.Scale, best.Score, clampPercent(r.Litness), best.X, best.Y)
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

// confirmSeconds is how long the cross must stay lit after a frame shows it
// lit before that frame can be the start: one lit frame never decides. On
// 9/27 the camera panned to the choir rehearsal before the service; a
// half-size match on the moving picture of the unlit cross, then a white
// shirt in the choir close-up, each read as a lit cross for one frame, and
// the search stopped there, 5:48 early. Lit elsewhere than the cross was
// seen unlit is never the start: on 11/29 a match on the pianist's shot
// stayed "lit" for thirteen minutes while the cross was out of view.
const confirmSeconds = 180

// minPlaceOverlap is how much of the smaller of two matches' boxes must lie
// inside the larger for them to count as the same place. Pictures of
// different shots at different scales box the cross with different margins,
// so their boxes differ in size while their centres agree.
const minPlaceOverlap = 0.6

// samePlace says whether two matches sit on the same thing.
func samePlace(a, b image.Rectangle) bool {
	inter := a.Intersect(b)
	if inter.Empty() {
		return false
	}
	smaller := min(a.Dx()*a.Dy(), b.Dx()*b.Dy())
	if smaller <= 0 {
		return false
	}
	return float64(inter.Dx()*inter.Dy())/float64(smaller) >= minPlaceOverlap
}

// litAt says whether a frame shows the cross lit in the same place as box.
func litAt(r frameReport, box image.Rectangle) bool {
	return r.State == detection.StateLit && samePlace(r.box(), box)
}

// endOfVideoFrames is how many coarse-scan frames in a row must be
// unreadable for the scan to take the video as ended and stop.
const endOfVideoFrames = 10

// sampleTimes are the coarse scan's frame times: 5 s in (a recording that
// started late shows the cross lit at once), then every step to the end of
// the search range.
func (s crossScan) sampleTimes() []int {
	times := []int{5}
	for t := s.coarseStep; t <= s.endSeconds; t += s.coarseStep {
		times = append(times, t)
	}
	return times
}

// staysLit says whether the cross, lit in box at the sample i, stays lit
// through the next confirmSeconds of samples: no two later samples show
// the cross unlit anywhere (there is one cross), and at least half of the
// readable ones show it lit. When the cross was seen unlit before (anchored),
// lit anywhere counts: the camera may cut to another shot of the lit cross.
// When nothing has been seen unlit yet, only lit in the same place counts.
// A frame of the coarse scan is looked at once; the forward scan reuses it.
func (s crossScan) staysLit(i int, box image.Rectangle, anchored bool, times []int, look func(seconds int, keep bool) (frameReport, error)) (ok bool, why string) {
	readable, lit := 0, 0
	var unlitAt []string
	for j := i + 1; j < len(times) && times[j]-times[i] <= confirmSeconds; j++ {
		r, err := look(times[j], true)
		if err != nil {
			continue
		}
		readable++
		switch {
		case r.State == detection.StateUnlit:
			// One unlit frame is as fallible as one lit one: two say the
			// cross went dark.
			unlitAt = append(unlitAt, secondsToTimestamp(times[j]))
			if len(unlitAt) == 2 {
				return false, fmt.Sprintf("the cross was unlit at %s and at %s", unlitAt[0], unlitAt[1])
			}
		case r.State == detection.StateLit && (anchored || samePlace(r.box(), box)):
			lit++
		}
	}
	switch {
	case readable == 0:
		return false, "no later frame could be read"
	case lit < (readable+1)/2:
		return false, fmt.Sprintf("only %d of the next %d frames showed it lit", lit, readable)
	}
	return true, ""
}

func (s crossScan) search(ctx context.Context, rawLook func(seconds int, keep bool) (frameReport, error), tally *frameTally) (detection.DetectionResult, error) {
	seen := map[int]frameReport{}
	failed := map[int]error{}
	look := func(seconds int, keep bool) (frameReport, error) {
		if r, ok := seen[seconds]; ok {
			return r, nil
		}
		if err, ok := failed[seconds]; ok {
			return frameReport{}, err
		}
		r, err := rawLook(seconds, keep)
		if err != nil {
			failed[seconds] = err
			return r, err
		}
		seen[seconds] = r
		return r, nil
	}

	// Phase 1: the coarse scan, forward, for the first frame that shows the
	// cross lit where it was seen unlit (when it was), and that stays lit.
	times := s.sampleTimes()
	var unlitBoxes []image.Rectangle
	start, provisional := -1, -1
	fleeting, unreadableRun := 0, 0
	firstUnlit := -1 // the first unlit frame since the provisional start
	for i, t := range times {
		if err := ctx.Err(); err != nil {
			return detection.DetectionResult{}, err
		}
		r, err := look(t, true)
		if err != nil {
			if unreadableRun++; unreadableRun >= endOfVideoFrames {
				break // The video has ended.
			}
			continue
		}
		unreadableRun = 0
		switch r.State {
		case detection.StateUnlit:
			tally.unlit++
			unlitBoxes = append(unlitBoxes, r.box())
			if provisional >= 0 && firstUnlit < 0 {
				firstUnlit = t
			}
		case detection.StateLit:
			if len(unlitBoxes) == 0 {
				if provisional >= 0 {
					continue // Lit since the provisional start; nothing new.
				}
				if ok, why := s.staysLit(i, r.box(), false, times, look); !ok {
					fleeting++
					fmt.Fprintf(s.out, "  %s  not the start: %s\n", secondsToTimestamp(t), why)
					continue
				}
				// Lit before the cross was ever seen unlit, so nothing says
				// where the cross is (a recording that started late, or a
				// camera parked away from the cross): the start, unless the
				// rest of the search range shows the cross unlit and then
				// lit where it was unlit. On 8/9 the camera sat on the
				// lectern and a TV for ten minutes; a static match there
				// read "lit" until the wide shot showed the cross unlit,
				// and lit at 00:12:28.
				provisional = i
				continue
			}
			if !overlapsAny(r.box(), unlitBoxes) {
				if provisional < 0 {
					fleeting++
					fmt.Fprintf(s.out, "  %s  not the start: lit elsewhere than the cross was seen unlit\n", secondsToTimestamp(t))
				}
				continue
			}
			ok, why := s.staysLit(i, r.box(), true, times, look)
			if !ok {
				fleeting++
				fmt.Fprintf(s.out, "  %s  not the start: %s\n", secondsToTimestamp(t), why)
				continue
			}
			if provisional >= 0 {
				fleeting++
				fmt.Fprintf(s.out, "  %s  not the start after all: the cross was unlit at %s and lit again at %s\n", secondsToTimestamp(times[provisional]), secondsToTimestamp(firstUnlit), secondsToTimestamp(t))
			}
			start = i
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 && provisional >= 0 {
		start = provisional
	}

	if start < 0 {
		analysed := tally.tried - tally.unreadable
		if analysed == 0 {
			return detection.DetectionResult{FramesAnalyzed: tally.tried}, fmt.Errorf("cross search could not run: %d of %d frames could not be read (the last: %v): %w",
				tally.unreadable, tally.tried, tally.lastErr, ErrNothingAnalysed)
		}
		return detection.DetectionResult{FramesAnalyzed: tally.tried}, &CrossNotFoundError{
			Analysed:   analysed,
			Unlit:      tally.unlit,
			Fleeting:   fleeting,
			Unreadable: tally.unreadable,
			LastErr:    tally.lastErr,
		}
	}

	first := seen[times[start]]
	box := first.box()
	if start == 0 {
		// Lit 5 s in: the recording started late; the start is its beginning.
		return detection.DetectionResult{
			Confidence:     first.Confidence,
			CameraAngle:    first.CameraAngle,
			Scale:          first.Scale,
			FramesAnalyzed: tally.tried,
		}, nil
	}

	// Phase 2: binary search between the last earlier sample that did not
	// show the cross lit there and the first that did.
	fmt.Fprintln(s.out, "  Phase 2: Binary search...")
	low, high := times[start-1], times[start]
	lastLitAnalysis := first
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
		if litAt(analysis, box) {
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
		if t <= low {
			continue
		}
		analysis, err := look(t, false)
		if err != nil {
			continue
		}
		if litAt(analysis, box) {
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

// overlapsAny says whether box sits where any of the boxes is.
func overlapsAny(box image.Rectangle, boxes []image.Rectangle) bool {
	for _, b := range boxes {
		if samePlace(box, b) {
			return true
		}
	}
	return false
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
