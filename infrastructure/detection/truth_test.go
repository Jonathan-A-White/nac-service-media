package detection

// The truth table: every recording whose true start a person checked, after a
// wrong trim, replayed from its saved frames (testdata/truth/<recording>/
// HH-MM-SS.jpg) against the church's pictures of the cross. A row is added by
// 'nac-service-media truth add'; a detector that misses one by more than
// TruthToleranceSeconds fails here.

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"nac-service-media/domain/detection"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// truthRoot is the repo's truth folder: the table and one folder of frames
// per recording.
const truthRoot = "../../testdata/truth"

// churchTruthConfig is the detection config of the church PC
// (config/config.yaml there, 2026-10-17), where the trims are made.
func churchTruthConfig() config.DetectionConfig {
	return config.DetectionConfig{
		TemplatesDir: churchTemplates,
		SearchRange:  config.SearchRangeConfig{StartMinutes: 10, EndMinutes: 70},
		Thresholds:   config.DetectionThresholdsConfig{MatchScore: 0.75, CoarseStepSeconds: 30},
	}
}

// AC2: every row of the table is replayed, and a start more than 10 s from
// the true one fails.
func TestTruthTable(t *testing.T) {
	cases, err := ReadTruthTable(filepath.Join(truthRoot, TruthTableFile))
	if err != nil {
		t.Fatal(err)
	}
	// AC3: the rows his corrections seeded stay in the table.
	for _, want := range []string{"2026-09-27 10-22-05.mp4", "2026-10-04 10-21-26.mp4"} {
		found := false
		for _, c := range cases {
			found = found || c.Recording == want
		}
		if !found {
			t.Errorf("the truth table has no row for %s", want)
		}
	}
	// One row at a time: the FFT plans are shared by size and not safe for
	// two detectors at once (newFFT2D).
	for _, c := range cases {
		t.Run(c.Recording, func(t *testing.T) {
			frames, err := NewReplayFrames(c.FramesDir(truthRoot))
			if err != nil {
				t.Fatal(err)
			}
			var log bytes.Buffer
			d := NewTemplateDetector(churchTruthConfig(), WithFrameExtractor(frames), WithOutput(&log), WithSamplesDir(""))
			if err := d.LoadTemplates(churchTemplates); err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			res, err := d.DetectStart(context.Background(), c.Recording)
			if miss := TruthMiss(c, res, err); miss != "" {
				t.Errorf("%s\nlog:\n%s", miss, log.String())
			}
		})
	}
}

func TestTruthMiss_MoreThanTenSecondsOffIsAMiss(t *testing.T) {
	c := TruthCase{Date: "2026-09-27", Recording: "2026-09-27 10-22-05.mp4", TrueStart: 522, Detected: 522, TrimmedFrom: 174}
	for _, tc := range []struct {
		found int
		err   error
		miss  string
	}{
		{522, nil, ""},
		{512, nil, ""},
		{532, nil, ""},
		{511, nil, "2026-09-27 10-22-05.mp4: the detector found 00:08:31, 11 s from the true start 00:08:42 (allowed 10 s)"},
		{174, nil, "2026-09-27 10-22-05.mp4: the detector found 00:02:54, 348 s from the true start 00:08:42 (allowed 10 s)"},
		{0, &CrossNotFoundError{Analysed: 3}, "2026-09-27 10-22-05.mp4: the detector found no start (true start 00:08:42): could not detect cross"},
	} {
		got := TruthMiss(c, startAt(tc.found), tc.err)
		if tc.miss == "" && got != "" || tc.miss != "" && !strings.HasPrefix(got, tc.miss) {
			t.Errorf("found %d, err %v: got %q, want %q", tc.found, tc.err, got, tc.miss)
		}
	}
}

func TestTruthTable_WriteAddsARowAndReplacesTheSameRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truth", TruthTableFile)
	a := TruthCase{Date: "2026-09-27", Recording: "2026-09-27 10-22-05.mp4", TrueStart: 522, Detected: 174, TrimmedFrom: -1}
	b := TruthCase{Date: "2026-10-04", Recording: "2026-10-04 10-21-26.mp4", TrueStart: 583, Detected: -1, TrimmedFrom: 100}
	for _, c := range []TruthCase{a, b} {
		if err := WriteTruthCase(path, c); err != nil {
			t.Fatal(err)
		}
	}
	a.TrueStart = 523
	if err := WriteTruthCase(path, a); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTruthTable(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []TruthCase{a, b}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"# ", "2026-09-27\t2026-09-27 10-22-05.mp4\t00:08:43\t00:02:54\t-", "2026-10-04\t2026-10-04 10-21-26.mp4\t00:09:43\tnone\t00:01:40"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the table should hold %q, got:\n%s", want, data)
		}
	}
}

func TestReadTruthTable_ABadRowNamesItsLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), TruthTableFile)
	os.WriteFile(path, []byte("# header\n2026-09-27\t2026-09-27 10-22-05.mp4\t8:42\n"), 0o644)
	if _, err := ReadTruthTable(path); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("want an error naming line 2, got %v", err)
	}
}

// AC1: the frames kept are the coarse scan's up to three minutes and a step
// past the latest start, and every second around the true start, the
// detected one and the start the wrong trim used.
func TestTruthFrameTimes(t *testing.T) {
	c := TruthCase{TrueStart: 100, Detected: 40, TrimmedFrom: 50}
	got := TruthFrameTimes(c, 30, 3600)
	want := []int{5, 30, 38, 39, 40, 41, 42, 48, 49, 50, 51, 52, 60, 90, 95, 96, 97, 98, 99, 100, 101, 102, 103, 104, 105, 120, 150, 180, 210, 240, 270, 300}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	// No detected start and no wrong trim; the search range's end caps it.
	got = TruthFrameTimes(TruthCase{TrueStart: 3, Detected: -1, TrimmedFrom: -1}, 60, 120)
	if want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 60, 120}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// The replay serves, for a time, the saved frame at or before it (the first
// for a time before them all), as the PNG the detector reads.
func TestReplayFrames_ServesTheFrameAtOrBefore(t *testing.T) {
	dir := t.TempDir()
	for seconds, level := range map[int]uint8{30: 10, 90: 200} {
		img := image.NewGray(image.Rect(0, 0, 8, 8))
		for i := range img.Pix {
			img.Pix[i] = level
		}
		src := filepath.Join(t.TempDir(), "f.png")
		if err := writePNG(src, img); err != nil {
			t.Fatal(err)
		}
		if err := SaveTruthFrame(src, dir, seconds); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "00-01-30.jpg")); err != nil {
		t.Fatalf("frames are saved as HH-MM-SS.jpg: %v", err)
	}
	r, err := NewReplayFrames(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at    string
		level uint8
	}{{"00:00:05", 10}, {"00:00:30", 10}, {"00:01:29", 10}, {"00:01:30", 200}, {"01:00:00", 200}} {
		out := filepath.Join(t.TempDir(), "out.png")
		if err := r.ExtractFrame(context.Background(), "v.mp4", tc.at, out); err != nil {
			t.Fatal(err)
		}
		f, _ := os.Open(out)
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if got := toGray(img).Pix[0]; got < tc.level-3 || got > tc.level+3 {
			t.Errorf("at %s got level %d, want about %d", tc.at, got, tc.level)
		}
	}
	if _, err := NewReplayFrames(t.TempDir()); err == nil {
		t.Error("a folder with no frames should be an error")
	}
}

// startAt is a detector's result: the start at a time in seconds.
func startAt(seconds int) detection.DetectionResult {
	return detection.DetectionResult{Timestamp: video.Timestamp{Hours: seconds / 3600, Minutes: seconds % 3600 / 60, Seconds: seconds % 60}}
}
