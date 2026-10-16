package detection

import (
	"image"
	"math"
	"testing"

	"nac-service-media/domain/detection"
)

// The matcher is OpenCV's TM_CCOEFF_NORMED over every position, computed with
// the FFT; referenceMatch (in scan_test.go) is the same formula written out.

func TestMatchScores_EqualTheReferenceAtEveryPosition(t *testing.T) {
	frame := paintRoom(roomSpec{width: 96, height: 54, centre: image.Pt(48, 14), crossHeight: 16, lit: true, seed: 3})
	tmpl := toGray(cropImage(frame, image.Rect(38, 2, 60, 30)))
	// Noise elsewhere, so the match is not trivially 1 everywhere.
	tmpl2 := paintRoom(roomSpec{width: 22, height: 28, centre: image.Pt(11, 10), crossHeight: 16, lit: false, seed: 9})

	for _, tc := range []struct {
		name string
		tmpl *image.Gray
	}{{"the crop itself", tmpl}, {"a different picture", tmpl2}} {
		wf := newWorkingFrame(frame, 1)
		scores := wf.scores(newZeroMeanTemplate(tc.tmpl))
		tw, th := tc.tmpl.Bounds().Dx(), tc.tmpl.Bounds().Dy()
		worst := 0.0
		for oy := 0; oy+th <= 54; oy++ {
			for ox := 0; ox+tw <= 96; ox++ {
				want := referenceScoreAt(frame, tc.tmpl, ox, oy)
				got := scores[oy*wf.width+ox]
				worst = math.Max(worst, math.Abs(got-want))
			}
		}
		if worst > 1e-6 {
			t.Errorf("%s: FFT scores differ from the reference by up to %g", tc.name, worst)
		}
	}
}

func TestWorkingFactor(t *testing.T) {
	for width, want := range map[int]int{1920: 4, 1280: 3, 960: 2, 640: 1, 480: 1, 3840: 8} {
		if got := workingFactor(width); got != want {
			t.Errorf("workingFactor(%d) = %d, want %d", width, got, want)
		}
	}
}

// roomAt paints the test room with the cross at a scale of the shot the
// templates were learned from (scale 1: the bar 60 px tall in a 640x360 view).
func roomAt(scale float64, lit bool, gain float64, seed uint32) (*image.Gray, roomSpec) {
	spec := roomSpec{width: 640, height: 360, centre: image.Pt(320, 90), crossHeight: int(60 * scale), lit: lit, gain: gain, seed: seed}
	return paintRoom(spec), spec
}

// learnTestShot learns the "room" shot from the 1x view, as 'templates add'
// does, and returns a matcher holding it.
func learnTestShot(t *testing.T, threshold float64) *matcher {
	t.Helper()
	unlit, _ := roomAt(1, false, 1, 1)
	lit, _ := roomAt(1, true, 1, 2)
	unlitTmpl, litTmpl, box, err := learnShot(unlit, lit)
	if err != nil {
		t.Fatalf("learnShot: %v", err)
	}
	t.Logf("learned the shot from box %v", box)
	m, err := newMatcher(map[string]*image.Gray{"room_lit": litTmpl, "room_unlit": unlitTmpl}, threshold)
	if err != nil {
		t.Fatalf("newMatcher: %v", err)
	}
	return m
}

func TestMatcher_FindsTheCrossAtHalfSingleAndDoubleScale(t *testing.T) {
	m := learnTestShot(t, 0)
	for _, scale := range []float64{0.5, 1, 2} {
		frame, spec := roomAt(scale, true, 1, 20)
		report := m.analyze(frame, 90)
		got := report.FrameAnalysis
		if got.State != detection.StateLit || got.CameraAngle != "room" {
			t.Errorf("at %gx: want the room shot lit, got %+v (matches %+v)", scale, got, report.Matches)
			continue
		}
		if got.Confidence < DefaultMatchScore {
			t.Errorf("at %gx: score %.3f is under the threshold %.2f", scale, got.Confidence, DefaultMatchScore)
		}
		if math.Abs(got.Scale/scale-1) > 0.1 {
			t.Errorf("at %gx: the scale found is %.3fx", scale, got.Scale)
		}
		best := report.best()
		centre := image.Pt(best.X+best.Width/2, best.Y+best.Height/2)
		if d := math.Hypot(float64(centre.X-spec.centre.X), float64(centre.Y-spec.centre.Y)); d > 8*scale+4 {
			t.Errorf("at %gx: the match's centre %v is %.0f px from the cross's %v", scale, centre, d, spec.centre)
		}
		t.Logf("at %gx: %s %.3f at %.3fx, cross %.0f%% lit", scale, best.Name, best.Score, best.Scale, 100*best.litFraction())
	}
}

func TestMatcher_AnEmptyFrameIsNotInView(t *testing.T) {
	m := learnTestShot(t, 0)
	spec := roomSpec{width: 640, height: 360, centre: image.Pt(320, 90), crossHeight: 60, lit: false, seed: 30}
	empty := paintRoom(spec)
	// Paint the wall over the cross and its rays.
	for y := 0; y < 200; y++ {
		for x := 200; x < 440; x++ {
			empty.Pix[y*empty.Stride+x] = uint8(125 + 15*y/360 + (x*7+y*3)%5 - 2)
		}
	}
	report := m.analyze(empty, 90)
	if report.FrameAnalysis.State != detection.StateNotVisible {
		t.Errorf("an empty wall should not show the cross, got %+v (matches %+v)", report.FrameAnalysis, report.Matches)
	}
	if best := report.best(); best.Score >= DefaultMatchScore {
		t.Errorf("the best score on an empty wall, %.3f (%s at %.2fx), reaches the threshold %.2f", best.Score, best.Name, best.Scale, DefaultMatchScore)
	}
}

// AC4: the lit/unlit decision for a small cross, at the exposure the shot was
// learned at and a quarter darker or brighter.
func TestMatcher_LitOrUnlitIsRightForASmallCrossAtAnyExposure(t *testing.T) {
	m := learnTestShot(t, 0)
	for _, tc := range []struct {
		lit  bool
		gain float64
	}{
		{true, 1}, {false, 1},
		{true, 0.75}, {false, 0.75},
		{true, 1.25}, {false, 1.25},
	} {
		frame, _ := roomAt(0.5, tc.lit, tc.gain, uint32(40+int(tc.gain*10)))
		report := m.analyze(frame, 60)
		want := detection.StateUnlit
		if tc.lit {
			want = detection.StateLit
		}
		if got := report.FrameAnalysis; got.State != want || got.CameraAngle != "room" {
			t.Errorf("lit=%v gain=%g: want %s on room, got %+v (best %+v)", tc.lit, tc.gain, want, got, report.best())
		}
	}
}

func TestClassifyMatches_ScoresTooCloseToTellUseTheContrast(t *testing.T) {
	// The two pictures fit about equally (the same shape); the frame's
	// contrast is nearer the unlit picture's, so the frame is unlit.
	matches := []templateMatch{
		{Name: "room_lit", Score: 0.93, Scale: 1, Contrast: 0.25, LitContrast: 1.2, UnlitContrast: 0.2},
		{Name: "room_unlit", Score: 0.87, Scale: 1, Contrast: 0.25, LitContrast: 1.2, UnlitContrast: 0.2},
	}
	got, _ := classifyMatches(matches, DefaultMatchScore, 60)
	if got.State != detection.StateUnlit || got.CameraAngle != "room" || got.Scale != 1 {
		t.Errorf("got %+v, want unlit on room at 1x", got)
	}
}
