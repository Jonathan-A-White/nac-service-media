package detection

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nac-service-media/domain/detection"
	"nac-service-media/infrastructure/config"
)

// fakeAnalyzer answers each frame from a rule, writing a tiny PNG for the
// frames the scan asks to keep, and counts the frames it was asked for.
type fakeAnalyzer struct {
	dir        string
	state      func(seconds int) detection.FrameState
	unreadable func(seconds int) bool
	asked      []int
}

func (f *fakeAnalyzer) analyze(ctx context.Context, videoPath string, seconds int, keep func(framePath string)) (frameReport, error) {
	f.asked = append(f.asked, seconds)
	if f.unreadable != nil && f.unreadable(seconds) {
		return frameReport{}, fmt.Errorf("failed to extract frame at %d", seconds)
	}
	if keep != nil {
		path := filepath.Join(f.dir, fmt.Sprintf("frame_%d.png", seconds))
		if err := writePNGFile(path, image.NewGray(image.Rect(0, 0, 8, 6))); err != nil {
			return frameReport{}, err
		}
		keep(path)
		os.Remove(path)
	}
	// An honest fake: a match has a place in the frame, as the real one does.
	match := templateMatch{Name: "room_lit", Score: 0.9, Scale: 1, X: 100, Y: 50, Width: 200, Height: 150}
	return frameReport{FrameAnalysis: detection.FrameAnalysis{State: f.state(seconds), CameraAngle: "room", Confidence: 0.9, Scale: 1, TimestampSeconds: seconds}, Matches: []templateMatch{match}}, nil
}

func scanConfig() config.DetectionConfig {
	return config.DetectionConfig{
		SearchRange: config.SearchRangeConfig{StartMinutes: 0, EndMinutes: 10},
		Thresholds:  config.DetectionThresholdsConfig{CoarseStepSeconds: 30},
	}
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestCrossScan_MissLeavesTheSampledFramesAndNamesTheFolder(t *testing.T) {
	fa := &fakeAnalyzer{dir: t.TempDir(), state: func(int) detection.FrameState { return detection.StateNotVisible }}
	samples := filepath.Join(t.TempDir(), "2026-10-11 10-17-09 cross frames")

	_, err := newCrossScan(scanConfig(), fa.analyze, samples, io.Discard).run(context.Background(), "v.mp4")

	var miss *CrossNotFoundError
	if !errors.As(err, &miss) {
		t.Fatalf("want a *CrossNotFoundError, got %v", err)
	}
	if errors.Is(err, ErrNothingAnalysed) {
		t.Error("a scan that looked at every frame is not 'nothing analysed'")
	}
	if miss.FramesDir != samples {
		t.Errorf("FramesDir = %q, want %q", miss.FramesDir, samples)
	}
	for _, want := range []string{"could not detect cross lighting up in search range", samples} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	names := listDir(t, samples)
	// The early check at 5 s and every 30 s from 30 to 600 s.
	if len(names) != 21 || miss.FramesKept != 21 {
		t.Fatalf("kept %d frames (FramesKept %d), want 21: %v", len(names), miss.FramesKept, names)
	}
	for _, want := range []string{"00-00-05.jpg", "00-00-30.jpg", "00-10-00.jpg"} {
		if sort.SearchStrings(names, want) == len(names) || names[sort.SearchStrings(names, want)] != want {
			t.Errorf("no sampled frame %s in %v", want, names)
		}
	}
	if miss.Analysed != 21 || miss.Unreadable != 0 {
		t.Errorf("Analysed %d, Unreadable %d; want 21, 0", miss.Analysed, miss.Unreadable)
	}
}

func TestCrossScan_EveryFrameUnreadableIsNothingAnalysedNotNotFound(t *testing.T) {
	fa := &fakeAnalyzer{
		dir:        t.TempDir(),
		state:      func(int) detection.FrameState { return detection.StateNotVisible },
		unreadable: func(int) bool { return true },
	}
	samples := filepath.Join(t.TempDir(), "frames")

	_, err := newCrossScan(scanConfig(), fa.analyze, samples, io.Discard).run(context.Background(), "v.mp4")

	if !errors.Is(err, ErrNothingAnalysed) {
		t.Fatalf("want ErrNothingAnalysed, got %v", err)
	}
	var miss *CrossNotFoundError
	if errors.As(err, &miss) {
		t.Error("a scan that read no frame must not read as 'cross not found'")
	}
	for _, want := range []string{"10 of 10", "failed to extract frame"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if _, statErr := os.Stat(samples); !os.IsNotExist(statErr) {
		t.Errorf("an empty frames folder was left behind: %v", statErr)
	}
}

func TestCrossScan_UnreadableFramesAreCountedInTheMiss(t *testing.T) {
	fa := &fakeAnalyzer{
		dir:        t.TempDir(),
		state:      func(int) detection.FrameState { return detection.StateUnlit },
		unreadable: func(s int) bool { return s >= 300 },
	}

	_, err := newCrossScan(scanConfig(), fa.analyze, filepath.Join(t.TempDir(), "frames"), io.Discard).run(context.Background(), "v.mp4")

	var miss *CrossNotFoundError
	if !errors.As(err, &miss) {
		t.Fatalf("want a *CrossNotFoundError, got %v", err)
	}
	// The early check at 5 s and 30..270 are read; from 300 on, ten frames
	// in a row cannot be, and the scan takes the video as ended.
	if miss.Unreadable != 10 || miss.Analysed != 10 {
		t.Errorf("Analysed %d, Unreadable %d; want 10, 10", miss.Analysed, miss.Unreadable)
	}
	if !strings.Contains(err.Error(), "10 could not be read") {
		t.Errorf("error %q does not count the unreadable frames", err)
	}
}

func TestCrossScan_FoundRemovesTheSampledFrames(t *testing.T) {
	fa := &fakeAnalyzer{dir: t.TempDir(), state: func(s int) detection.FrameState {
		if s >= 247 {
			return detection.StateLit
		}
		return detection.StateUnlit
	}}
	samples := filepath.Join(t.TempDir(), "frames")

	res, err := newCrossScan(scanConfig(), fa.analyze, samples, io.Discard).run(context.Background(), "v.mp4")

	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := res.Timestamp.String(); got != "00:04:07" {
		t.Errorf("found %s, want 00:04:07", got)
	}
	if _, statErr := os.Stat(samples); !os.IsNotExist(statErr) {
		t.Errorf("the frames folder should be removed when the cross is found: %v", statErr)
	}
}

func TestCrossScan_LitAtTheStartIsZero(t *testing.T) {
	fa := &fakeAnalyzer{dir: t.TempDir(), state: func(int) detection.FrameState { return detection.StateLit }}
	res, err := newCrossScan(scanConfig(), fa.analyze, "", io.Discard).run(context.Background(), "v.mp4")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := res.Timestamp.String(); got != "00:00:00" {
		t.Errorf("found %s, want 00:00:00", got)
	}
}

func TestSampleFramesDir_IsBesideTheVideo(t *testing.T) {
	got := sampleFramesDir("/mnt/c/Videos/2026-10-11 10-17-09.mp4")
	if want := "/mnt/c/Videos/2026-10-11 10-17-09 cross frames"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAmenNotDetected(t *testing.T) {
	_, err := amenNotDetected("Video too short: 3884s, need at least 4320s")
	if !errors.Is(err, ErrNothingAnalysed) || !strings.Contains(err.Error(), "Video too short: 3884s") {
		t.Errorf("a search that could not run should be ErrNothingAnalysed and keep the reason, got %v", err)
	}

	res, err := amenNotDetected("Best match score 0.31 below threshold 0.50")
	if err != nil {
		t.Fatalf("a search that ran and found nothing is not an error, got %v", err)
	}
	if res.Detected || res.Error != "Best match score 0.31 below threshold 0.50" {
		t.Errorf("got %+v", res)
	}
}

// --- the match threshold on a small template from the wide shot ---

// noisyRoom is a 480x270 view of the whole room (a quarter of 1080p) with the
// cross about 15x22 px above the altar, a quarter of the real one, so smaller
// than any shot the camera gives today. The unlit cross is a little lighter
// than the wall, so the lit and unlit crosses have the same shape and
// polarity, the hard case for a normalised match. seed varies the noise.
func noisyRoom(crossLevel uint8, seed uint32) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, 480, 270))
	state := seed*2654435761 + 1
	for y := 0; y < 270; y++ {
		for x := 0; x < 480; x++ {
			v := 90
			if y > 150 && (x/20)%2 == 0 {
				v = 50 // pews
			}
			if y > 110 && y < 140 && x > 200 && x < 280 {
				v = 130 // altar
			}
			vertical := x >= 236 && x < 240 && y >= 60 && y < 82
			bar := x >= 230 && x < 245 && y >= 65 && y < 69
			if vertical || bar {
				v = int(crossLevel)
			}
			state = state*1664525 + 1013904223
			v += int(state>>24)%11 - 5
			img.SetGray(x, y, color.Gray{Y: uint8(v)})
		}
	}
	return img
}

// referenceScoreAt is OpenCV's TM_CCOEFF_NORMED written out in Go: the
// normalised score of tmpl over frame at one position.
func referenceScoreAt(frame, tmpl *image.Gray, ox, oy int) float64 {
	tw, th := tmpl.Bounds().Dx(), tmpl.Bounds().Dy()
	n := float64(tw * th)
	var tsum float64
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			tsum += float64(tmpl.Pix[y*tmpl.Stride+x])
		}
	}
	tmean := tsum / n
	var sum, sq, cross, tnorm float64
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			d := float64(tmpl.Pix[y*tmpl.Stride+x]) - tmean
			tnorm += d * d
			v := float64(frame.Pix[(oy+y)*frame.Stride+ox+x])
			sum += v
			sq += v * v
			cross += v * d
		}
	}
	fvar := sq - sum*sum/n
	if fvar <= 0 || tnorm <= 0 {
		return 0
	}
	return cross / math.Sqrt(fvar*tnorm)
}

// referenceMatch is the best reference score of tmpl over frame, with the
// contrast under the best match as the matcher computes it: the shot's lit
// picture marks the cross and the wall.
func referenceMatch(name string, frame, tmpl, litTmpl, unlitTmpl *image.Gray) templateMatch {
	tw, th := tmpl.Bounds().Dx(), tmpl.Bounds().Dy()
	best := templateMatch{Name: name, Score: -2, Scale: 1}
	fw, fh := frame.Bounds().Dx(), frame.Bounds().Dy()
	for oy := 0; oy+th <= fh; oy++ {
		for ox := 0; ox+tw <= fw; ox++ {
			if score := referenceScoreAt(frame, tmpl, ox, oy); score > best.Score {
				best.Score, best.X, best.Y = score, ox, oy
			}
		}
	}
	bar, wall := contrastMasks(litTmpl)
	best.Contrast = contrastUnder(frame, best.X, best.Y, tw, th, bar, wall)
	best.LitContrast = contrastUnder(litTmpl, 0, 0, tw, th, bar, wall)
	best.UnlitContrast = contrastUnder(unlitTmpl, 0, 0, tw, th, bar, wall)
	return best
}

func TestClassifyMatches_SmallTemplateFromTheWideShot(t *testing.T) {
	// Learn the shot as 'templates add' does: from one unlit and one lit frame.
	unlitCapture, litCapture := noisyRoom(110, 1), noisyRoom(235, 2)
	box, err := FindCrossBox(unlitCapture, litCapture)
	if err != nil {
		t.Fatalf("FindCrossBox: %v", err)
	}
	if box.Dx() > 24 || box.Dy() > 34 {
		t.Fatalf("box %v is far larger than the 15x22 cross", box)
	}
	litTmpl := toGray(cropImage(litCapture, box))
	unlitTmpl := toGray(cropImage(unlitCapture, box))

	// No minimum template size and the default threshold: later frames, with
	// their own noise, are told apart.
	for _, tc := range []struct {
		frame *image.Gray
		want  detection.FrameState
	}{
		{noisyRoom(235, 7), detection.StateLit},
		{noisyRoom(110, 8), detection.StateUnlit},
		{noisyRoom(235, 9), detection.StateLit},
		{noisyRoom(110, 10), detection.StateUnlit},
	} {
		matches := []templateMatch{
			referenceMatch("room_lit", tc.frame, litTmpl, litTmpl, unlitTmpl),
			referenceMatch("room_unlit", tc.frame, unlitTmpl, litTmpl, unlitTmpl),
		}
		got, _ := classifyMatches(matches, DefaultMatchScore, 60)
		if got.State != tc.want || got.CameraAngle != "room" {
			t.Errorf("want %s on room, got %+v from %+v", tc.want, got, matches)
		}
		if got.Confidence < DefaultMatchScore {
			t.Errorf("confidence %.3f is below the threshold %.2f", got.Confidence, DefaultMatchScore)
		}
	}
}

func TestClassifyMatches_BelowTheThresholdIsNotVisible(t *testing.T) {
	got, _ := classifyMatches([]templateMatch{
		{Name: "room_lit", Score: 0.6, Scale: 1, Contrast: 1.2, LitContrast: 1.2, UnlitContrast: 0.2},
		{Name: "room_unlit", Score: 0.5, Scale: 1, Contrast: 1.2, LitContrast: 1.2, UnlitContrast: 0.2},
	}, DefaultMatchScore, 60)
	if got.State != detection.StateNotVisible {
		t.Errorf("got %+v, want not visible", got)
	}
}

func TestCrossScan_MissSaysWhenNoSampledFrameCouldBeSaved(t *testing.T) {
	fa := &fakeAnalyzer{dir: t.TempDir(), state: func(int) detection.FrameState { return detection.StateNotVisible }}
	blocker := filepath.Join(t.TempDir(), "a file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newCrossScan(scanConfig(), fa.analyze, filepath.Join(blocker, "frames"), io.Discard).run(context.Background(), "v.mp4")

	var miss *CrossNotFoundError
	if !errors.As(err, &miss) {
		t.Fatalf("want a *CrossNotFoundError, got %v", err)
	}
	if miss.FramesDir != "" {
		t.Errorf("FramesDir = %q, want none: no frame was saved", miss.FramesDir)
	}
	if !strings.Contains(err.Error(), "21 sampled frames could not be saved") || !strings.Contains(err.Error(), blocker) {
		t.Errorf("error %q should count the frames that could not be saved and say why", err)
	}
}
