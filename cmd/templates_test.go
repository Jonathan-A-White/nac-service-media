package cmd

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"
)

// stillFrames stands in for ffmpeg: a dark frame, with a bright block in it
// at the lit time.
type stillFrames struct{ litAt string }

func (f stillFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	img := image.NewGray(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			v := uint8(30)
			if at == f.litAt && x >= 300 && x < 330 && y >= 100 && y < 145 {
				v = 230
			}
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	file, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func TestTemplatesAdd_PrintsTheBoxAndThePathsWritten(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer

	err := templatesAdd(context.Background(), stillFrames{litAt: "00:14:07"}, infradetection.AddTemplateRequest{
		VideoPath: "/videos/2026-10-11 10-17-09.mp4", UnlitAt: "00:14:06", LitAt: "00:14:07", Name: "room", TemplatesDir: dir,
	}, &out)
	if err != nil {
		t.Fatalf("templatesAdd: %v", err)
	}
	for _, want := range []string{
		"Cross box: x=", "found from the brightness rise",
		filepath.Join(dir, "room_lit.png"),
		filepath.Join(dir, "room_unlit.png"),
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestTemplatesAdd_ExistingNameIsRefusedWithoutForce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "room_unlit.png"), []byte("old"), 0o644)
	var out bytes.Buffer
	err := templatesAdd(context.Background(), stillFrames{litAt: "00:14:07"}, infradetection.AddTemplateRequest{
		VideoPath: "v.mp4", UnlitAt: "00:14:06", LitAt: "00:14:07", Name: "room", TemplatesDir: dir,
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want a refusal naming --force, got %v", err)
	}
}

func TestTemplatesFrame_WritesAPNGFromATestVideo(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed, so this test cannot generate a test video: install ffmpeg to run it")
	}
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "testsrc.mp4")
	gen := exec.Command(ffmpegPath, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=3:size=320x240:rate=10",
		"-pix_fmt", "yuv420p", "-y", videoPath)
	if b, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate test video: %v\n%s", err, b)
	}
	outPath := filepath.Join(dir, "look", "frame.png")
	var out bytes.Buffer

	if err := templatesFrame(context.Background(), infradetection.FFmpegFrames{FFmpegPath: ffmpegPath}, videoPath, "00:00:02", outPath, &out); err != nil {
		t.Fatalf("templatesFrame: %v", err)
	}

	file, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("no frame written: %v", err)
	}
	defer file.Close()
	cfg, err := png.DecodeConfig(file)
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	if cfg.Width != 320 || cfg.Height != 240 {
		t.Errorf("frame is %dx%d, want 320x240", cfg.Width, cfg.Height)
	}
	if !strings.Contains(out.String(), outPath) {
		t.Errorf("output should name the file written:\n%s", out.String())
	}
}

func TestTemplatesFrame_BadTimeIsRefused(t *testing.T) {
	err := templatesFrame(context.Background(), failingFrames{}, "v.mp4", "14:07", filepath.Join(t.TempDir(), "f.png"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "HH:MM:SS") {
		t.Fatalf("want an error naming HH:MM:SS, got %v", err)
	}
}

type failingFrames struct{}

func (failingFrames) ExtractFrame(context.Context, string, string, string) error {
	return errors.New("should not be called")
}

func TestResolveVideoInput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "2026-10-11 10-17-09.mp4"), nil, 0o644)
	cfg := &config.Config{Paths: config.PathsConfig{SourceDirectory: dir}}

	if got := resolveVideoInput("2026-10-11 10-17-09.mp4", cfg); got != filepath.Join(dir, "2026-10-11 10-17-09.mp4") {
		t.Errorf("a bare file name should be found in the source folder, got %q", got)
	}
	abs := filepath.Join(dir, "x.mp4")
	if got := resolveVideoInput(abs, cfg); got != abs {
		t.Errorf("an absolute path is kept, got %q", got)
	}
	if got := resolveVideoInput("x.mp4", nil); got != "x.mp4" {
		t.Errorf("without config the path is kept, got %q", got)
	}
}

func TestTemplatesDirFromConfig(t *testing.T) {
	if got := templatesDirFrom(nil); got != "config/detection_templates" {
		t.Errorf("default = %q", got)
	}
	cfg := &config.Config{Detection: config.DetectionConfig{TemplatesDir: "/x/templates"}}
	if got := templatesDirFrom(cfg); got != "/x/templates" {
		t.Errorf("configured = %q", got)
	}
}
