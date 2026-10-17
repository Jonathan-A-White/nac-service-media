package cmd

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"
)

// paintCross paints a small cross on a grey square: bright when lit, a little
// lighter than the wall when not.
func paintCross(img *image.Gray, at image.Point, lit bool) {
	level := uint8(150)
	if lit {
		level = 245
	}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			v := uint8(120)
			if (x >= 14 && x < 18 && y >= 4 && y < 28) || (y >= 10 && y < 14 && x >= 8 && x < 24) {
				v = level
			}
			img.SetGray(at.X+x, at.Y+y, color.Gray{Y: v})
		}
	}
}

// litFrames serves a 192x144 room whose cross lights at litFrom seconds.
type litFrames struct{ litFrom int }

func (f litFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	ts, err := video.ParseTimestamp(at)
	if err != nil {
		return err
	}
	img := image.NewGray(image.Rect(0, 0, 192, 144))
	seed := uint32(ts.TotalSeconds()*7919 + 1)
	for i := range img.Pix {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = uint8(90 + int(seed>>24)%9)
	}
	paintCross(img, image.Pt(80, 30), ts.TotalSeconds() >= f.litFrom)
	return writePNGTo(outPath, img)
}

func writePNGTo(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

// truthTestConfig has one shot of the cross, cut from litFrames, and a
// five-minute search range.
func truthTestConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	templates := filepath.Join(dir, "templates")
	os.MkdirAll(templates, 0o755)
	for name, lit := range map[string]bool{"room_lit.png": true, "room_unlit.png": false} {
		img := image.NewGray(image.Rect(0, 0, 32, 32))
		paintCross(img, image.Pt(0, 0), lit)
		if err := writePNGTo(filepath.Join(templates, name), img); err != nil {
			t.Fatal(err)
		}
	}
	videoPath := filepath.Join(dir, "2026-10-11 10-17-09.mp4")
	os.WriteFile(videoPath, []byte("not really a video"), 0o644)
	cfg := &config.Config{}
	cfg.Paths.SourceDirectory = dir
	cfg.Detection = config.DetectionConfig{
		Enabled:      true,
		TemplatesDir: templates,
		SearchRange:  config.SearchRangeConfig{StartMinutes: 0, EndMinutes: 5},
		Thresholds:   config.DetectionThresholdsConfig{MatchScore: 0.75, CoarseStepSeconds: 30},
	}
	return cfg, videoPath
}

// AC1: truth add cuts the frames around the true start and the detected one
// into testdata/truth/<recording>/ and adds the row to the table.
func TestTruthAdd_CutsTheFramesAndAddsTheRow(t *testing.T) {
	cfg, _ := truthTestConfig(t)
	repo := t.TempDir()
	var out bytes.Buffer
	run := truthRun{
		cfg:         cfg,
		recording:   "2026-10-11 10-17-09.mp4",
		trueStart:   "00:02:10",
		trimmedFrom: "00:00:50",
		repo:        repo,
		out:         &out,
		frames:      litFrames{litFrom: 100},
	}

	if err := run.execute(context.Background()); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	truth := filepath.Join(repo, "testdata", "truth")
	cases, err := infradetection.ReadTruthTable(filepath.Join(truth, infradetection.TruthTableFile))
	if err != nil {
		t.Fatal(err)
	}
	want := []infradetection.TruthCase{{Date: "2026-10-11", Recording: "2026-10-11 10-17-09.mp4", TrueStart: 130, Detected: 100, TrimmedFrom: 50}}
	if !reflect.DeepEqual(cases, want) {
		t.Errorf("table: got %+v, want %+v", cases, want)
	}

	entries, err := os.ReadDir(filepath.Join(truth, "2026-10-11 10-17-09"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range []string{
		"00-00-05.jpg", "00-00-30.jpg", "00-05-00.jpg", // the coarse scan
		"00-02-05.jpg", "00-02-10.jpg", "00-02-15.jpg", // around the true start
		"00-01-38.jpg", "00-01-40.jpg", "00-01-42.jpg", // around the detected one
		"00-00-48.jpg", "00-00-50.jpg", "00-00-52.jpg", // around where the trim started
	} {
		if sort.SearchStrings(names, name) == len(names) || names[sort.SearchStrings(names, name)] != name {
			t.Errorf("no frame %s in %v", name, names)
		}
	}

	for _, line := range []string{
		"Detected start on the recording: 00:01:40",
		"Replayed from the saved frames: 00:01:40",
		"30 s from the true start 00:02:10",
	} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("output should contain %q, got:\n%s", line, out.String())
		}
	}
}

// Adding a recording again replaces its row and its frames.
func TestTruthAdd_AgainReplacesTheRowAndTheFrames(t *testing.T) {
	cfg, _ := truthTestConfig(t)
	repo := t.TempDir()
	run := truthRun{cfg: cfg, recording: "2026-10-11 10-17-09.mp4", trueStart: "00:02:10", repo: repo, out: &bytes.Buffer{}, frames: litFrames{litFrom: 100}}
	if err := run.execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	run.trueStart = "00:01:40"
	var out bytes.Buffer
	run.out = &out
	if err := run.execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	truth := filepath.Join(repo, "testdata", "truth")
	cases, _ := infradetection.ReadTruthTable(filepath.Join(truth, infradetection.TruthTableFile))
	if len(cases) != 1 || cases[0].TrueStart != 100 || cases[0].TrimmedFrom != -1 {
		t.Errorf("want one row with the true start 100 s, got %+v", cases)
	}
	if _, err := os.Stat(filepath.Join(truth, "2026-10-11 10-17-09", "00-02-15.jpg")); !os.IsNotExist(err) {
		t.Errorf("the earlier case's frames should be gone, stat: %v", err)
	}
	if !strings.Contains(out.String(), "within 10 s of the true start") {
		t.Errorf("output should say the replay finds it, got:\n%s", out.String())
	}
}

func TestTruthAdd_ABadTimeOrNoDateIsAnError(t *testing.T) {
	cfg, videoPath := truthTestConfig(t)
	clip := filepath.Join(filepath.Dir(videoPath), "clip.mp4")
	os.WriteFile(clip, []byte("x"), 0o644)
	for _, run := range []truthRun{
		{cfg: cfg, recording: "2026-10-11 10-17-09.mp4", trueStart: "2:10"},
		{cfg: cfg, recording: "2026-10-11 10-17-09.mp4", trueStart: "00:02:10", trimmedFrom: "soon"},
		{cfg: cfg, recording: clip, trueStart: "00:02:10"},
		{cfg: cfg, recording: "missing.mp4", trueStart: "00:02:10"},
	} {
		run.repo, run.out, run.frames = t.TempDir(), &bytes.Buffer{}, litFrames{litFrom: 100}
		if err := run.execute(context.Background()); err == nil {
			t.Errorf("%+v: want an error", run)
		}
	}
}

// brokenFrames cannot read any frame.
type brokenFrames struct{}

func (brokenFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	return os.ErrNotExist
}

// A case whose frame at the true start cannot be cut leaves the kept frames
// as they were.
func TestTruthAdd_AFailedCutKeepsTheEarlierFrames(t *testing.T) {
	cfg, _ := truthTestConfig(t)
	repo := t.TempDir()
	run := truthRun{cfg: cfg, recording: "2026-10-11 10-17-09.mp4", trueStart: "00:02:10", repo: repo, out: &bytes.Buffer{}, frames: litFrames{litFrom: 100}}
	if err := run.execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	framesDir := filepath.Join(repo, "testdata", "truth", "2026-10-11 10-17-09")
	if err := run.keepFrames(context.Background(), brokenFrames{}, "v.mp4", infradetection.TruthCase{TrueStart: 130, Detected: -1, TrimmedFrom: -1}, framesDir); err == nil {
		t.Fatal("want an error when the frame at the true start cannot be cut")
	}
	if _, err := os.Stat(filepath.Join(framesDir, "00-02-10.jpg")); err != nil {
		t.Errorf("the earlier frames should be kept: %v", err)
	}
	if _, err := os.Stat(framesDir + ".new"); !os.IsNotExist(err) {
		t.Errorf("no half-cut folder should be left, stat: %v", err)
	}
}
