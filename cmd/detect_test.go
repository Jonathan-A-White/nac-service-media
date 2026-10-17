package cmd

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nac-service-media/infrastructure/config"
)

// flatFrames serves a plain gray 1280x720 frame for any time: no cross.
type flatFrames struct{ asked []string }

func (f *flatFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	f.asked = append(f.asked, at)
	img := image.NewGray(image.Rect(0, 0, 1280, 720))
	for i := range img.Pix {
		img.Pix[i] = 40
	}
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(file, img)
}

func detectTestConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "2026-09-27 10-22-05.mp4")
	if err := os.WriteFile(videoPath, []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Paths.SourceDirectory = dir
	cfg.Detection = config.DetectionConfig{
		Enabled:      true,
		TemplatesDir: "../config/detection_templates",
		SearchRange:  config.SearchRangeConfig{StartMinutes: 0, EndMinutes: 2},
		Thresholds:   config.DetectionThresholdsConfig{CoarseStepSeconds: 60},
	}
	return cfg, videoPath
}

// AC2: detect --at prints what every picture scored in the named frame and
// can save the frame, and touches nothing else.
func TestDetect_AtPrintsEveryPictureAndSavesTheFrame(t *testing.T) {
	cfg, videoPath := detectTestConfig(t)
	save := filepath.Join(t.TempDir(), "frames")
	var out bytes.Buffer
	frames := &flatFrames{}
	run := detectRun{cfg: cfg, input: "2026-09-27 10-22-05.mp4", at: []string{"00:00:05"}, save: save, out: &out, frames: frames}

	if err := run.execute(context.Background()); err != nil {
		t.Fatalf("execute: %v\n%s", err, out.String())
	}

	for _, want := range []string{"Recording: " + videoPath, "00:00:05  not in view", "wide_lit", "wide_unlit", "closeup_lit", "(threshold 0.75)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should contain %q, got:\n%s", want, out.String())
		}
	}
	if len(frames.asked) != 1 || frames.asked[0] != "00:00:05" {
		t.Errorf("--at should look at that frame only, asked %v", frames.asked)
	}
	if _, err := os.Stat(filepath.Join(save, "00-00-05.png")); err != nil {
		t.Errorf("the frame should be saved as 00-00-05.png: %v", err)
	}
}

// AC2: a detect search prints one line per frame and the miss, leaving no
// frames beside the video.
func TestDetect_SearchPrintsEachFrameAndLeavesNothingBesideTheVideo(t *testing.T) {
	cfg, videoPath := detectTestConfig(t)
	var out bytes.Buffer
	frames := &flatFrames{}
	run := detectRun{cfg: cfg, input: videoPath, out: &out, frames: frames}

	err := run.execute(context.Background())

	if err == nil || !strings.Contains(err.Error(), "could not detect cross lighting up") {
		t.Fatalf("want the miss as the error, got %v\n%s", err, out.String())
	}
	for _, want := range []string{"00:00:05  not in view", "00:01:00  not in view", "00:02:00  not in view"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should contain %q, got:\n%s", want, out.String())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(videoPath))
	if len(entries) != 1 {
		t.Errorf("detect must leave nothing beside the video, found %d entries", len(entries))
	}
}
