package detection

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// paintedFrames stands in for ffmpeg: it paints the room at each time asked
// for, with the cross lit from litAt seconds on, at the given scale of the
// learned shot. The 1280x720 view is a third of 1080p's width, so the
// detector works at a reduced size and pads it, as it does on the church PC.
type paintedFrames struct {
	scale   float64
	litAt   int
	gain    float64
	noCross bool
	asked   []string
}

func (p *paintedFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	p.asked = append(p.asked, at)
	ts, err := video.ParseTimestamp(at)
	if err != nil {
		return err
	}
	seconds := ts.TotalSeconds()
	spec := roomSpec{width: 1280, height: 720, centre: image.Pt(640, 180), crossHeight: int(120 * p.scale), lit: seconds >= p.litAt, gain: p.gain, seed: uint32(seconds + 100)}
	if p.noCross {
		spec.crossHeight = 0
		spec.centre = image.Pt(-1000, -1000)
	}
	img := paintRoom(spec)
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(file, img)
}

// writeTestTemplates learns the "room" shot from the 1280x720 view at scale 1
// and writes it into a templates folder, as 'templates add' would.
func writeTestTemplates(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	paint := func(lit bool, seed uint32) *image.Gray {
		return paintRoom(roomSpec{width: 1280, height: 720, centre: image.Pt(640, 180), crossHeight: 120, lit: lit, seed: seed})
	}
	unlitTmpl, litTmpl, _, err := learnShot(paint(false, 1), paint(true, 2))
	if err != nil {
		t.Fatal(err)
	}
	for name, img := range map[string]*image.Gray{"room_lit.png": litTmpl, "room_unlit.png": unlitTmpl} {
		if err := writePNGFile(filepath.Join(dir, name), img); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func detectorConfig() config.DetectionConfig {
	return config.DetectionConfig{
		SearchRange: config.SearchRangeConfig{StartMinutes: 0, EndMinutes: 3},
		Thresholds:  config.DetectionThresholdsConfig{CoarseStepSeconds: 60},
	}
}

func TestTemplateDetector_FindsWhenTheCrossLightsUpAtEachScale(t *testing.T) {
	templates := writeTestTemplates(t)
	for _, scale := range []float64{0.5, 1, 2} {
		var log bytes.Buffer
		frames := &paintedFrames{scale: scale, litAt: 97}
		d := NewTemplateDetector(detectorConfig(), WithFrameExtractor(frames), WithOutput(&log))
		if err := d.LoadTemplates(templates); err != nil {
			t.Fatalf("LoadTemplates: %v", err)
		}
		res, err := d.DetectStart(context.Background(), filepath.Join(t.TempDir(), "v.mp4"))
		d.Close()
		if err != nil {
			t.Errorf("at %gx: %v\nlog:\n%s", scale, err, log.String())
			continue
		}
		if got := res.Timestamp.String(); got != "00:01:37" {
			t.Errorf("at %gx: found %s, want 00:01:37 (asked %v)\nlog:\n%s", scale, got, frames.asked, log.String())
		}
		if res.CameraAngle != "room" || res.Confidence < DefaultMatchScore {
			t.Errorf("at %gx: want the room shot at or over %.2f, got %+v", scale, DefaultMatchScore, res)
		}
		if res.Scale < scale*0.9 || res.Scale > scale*1.1 {
			t.Errorf("at %gx: the scale found is %.3fx", scale, res.Scale)
		}
		// Every frame looked at is one line in the log, so a run can be
		// read back later.
		if !strings.Contains(log.String(), "00:01:37  lit") || !strings.Contains(log.String(), "00:01:00  unlit") {
			t.Errorf("at %gx: the log should say what each frame showed, got:\n%s", scale, log.String())
		}
	}
}

// AC3: a miss says, per template, the best it scored, at which scale, in
// which frame, and the threshold, in the error and in the log.
func TestTemplateDetector_MissSaysWhatEachTemplateScored(t *testing.T) {
	templates := writeTestTemplates(t)
	var log bytes.Buffer
	frames := &paintedFrames{scale: 1, litAt: 1 << 30} // never lit
	d := NewTemplateDetector(detectorConfig(), WithFrameExtractor(frames), WithOutput(&log))
	if err := d.LoadTemplates(templates); err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	defer d.Close()

	_, err := d.DetectStart(context.Background(), filepath.Join(t.TempDir(), "v.mp4"))

	var miss *CrossNotFoundError
	if !errors.As(err, &miss) {
		t.Fatalf("want a *CrossNotFoundError, got %v", err)
	}
	if miss.Threshold != DefaultMatchScore {
		t.Errorf("Threshold = %v, want the default %v", miss.Threshold, DefaultMatchScore)
	}
	if len(miss.Best) != 2 || miss.Best[0].Name != "room_unlit" || miss.Best[1].Name != "room_lit" {
		t.Fatalf("Best should hold both templates, the higher score first, got %+v", miss.Best)
	}
	unlit, lit := miss.Best[0], miss.Best[1]
	if unlit.Score < DefaultMatchScore || lit.Score >= unlit.Score {
		t.Errorf("the unlit picture should score at least %.2f and above the lit one on unlit frames, got %+v", DefaultMatchScore, miss.Best)
	}
	if unlit.Scale < 0.9 || unlit.Scale > 1.1 {
		t.Errorf("the unlit picture's best scale should be about 1x, got %+v", unlit)
	}
	if unlit.Seconds%60 != 0 && unlit.Seconds != 5 {
		t.Errorf("the best frame should be one the coarse scan looked at, got %d s", unlit.Seconds)
	}
	text := err.Error()
	for _, want := range []string{
		"looked at 4 frames: 4 matched an unlit shot, 0 matched no shot",
		fmt.Sprintf("The best each picture of the cross scored (a frame shows the cross from %.2f):", DefaultMatchScore),
		fmt.Sprintf("  room_unlit  %.2f at %.2fx in the frame at %s", unlit.Score, unlit.Scale, secondsToTimestamp(unlit.Seconds)),
		fmt.Sprintf("  room_lit    %.2f at %.2fx in the frame at %s", lit.Score, lit.Scale, secondsToTimestamp(lit.Seconds)),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the error should contain %q, got:\n%s", want, text)
		}
	}
	if !strings.Contains(log.String(), "00:00:05  unlit") || !strings.Contains(log.String(), "room 1.00x") {
		t.Errorf("the log should say what each frame showed, got:\n%s", log.String())
	}
}

func TestTemplateDetector_NoTemplatesIsNothingAnalysed(t *testing.T) {
	d := NewTemplateDetector(detectorConfig())
	err := d.LoadTemplates(t.TempDir())
	if !errors.Is(err, ErrNothingAnalysed) {
		t.Errorf("an empty templates folder should be ErrNothingAnalysed, got %v", err)
	}
}
