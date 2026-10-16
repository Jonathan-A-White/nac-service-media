package detection

import (
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
)

// fakeFrames stands in for ffmpeg: it writes the frame given for each time.
type fakeFrames struct {
	frames map[string]image.Image
	asked  []string
}

func (f *fakeFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	f.asked = append(f.asked, at)
	img, ok := f.frames[at]
	if !ok {
		return errors.New("no frame at " + at)
	}
	return writePNGFile(outPath, img)
}

func writePNGFile(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func readPNGFile(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return img
}

// crossBlob is the cross-shaped blob drawn into the synthetic frames: a
// vertical bar 12 wide and 90 tall, and a cross bar 60 wide and 12 tall, as in
// the wide shot of the whole room (about 60x90 px of a 1080p frame).
var crossBlob = image.Rect(700, 300, 760, 390)

func inCross(x, y int) bool {
	vertical := x >= 724 && x < 736 && y >= 300 && y < 390
	bar := x >= 700 && x < 760 && y >= 320 && y < 332
	return vertical || bar
}

// syntheticFrame is a dark 1280x720 frame with some lighter furniture, and the
// cross drawn at the given grey level (or not at all when cross is 0).
func syntheticFrame(cross uint8) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			v := uint8(25 + (x*7+y*13)%9) // a little texture, the same in both frames
			if y > 500 && (x/80)%2 == 0 {
				v = 70 // pews
			}
			if cross != 0 && inCross(x, y) {
				v = cross
			}
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return img
}

func TestAddTemplate_FindsTheCrossBoxFromTheBrightnessRise(t *testing.T) {
	dir := t.TempDir()
	frames := &fakeFrames{frames: map[string]image.Image{
		"00:14:06": syntheticFrame(0),
		"00:14:07": syntheticFrame(240),
	}}

	res, err := AddTemplate(context.Background(), frames, AddTemplateRequest{
		VideoPath:    "/videos/2026-10-11 10-17-09.mp4",
		UnlitAt:      "00:14:06",
		LitAt:        "00:14:07",
		Name:         "room",
		TemplatesDir: dir,
	})
	if err != nil {
		t.Fatalf("AddTemplate: %v", err)
	}

	if !crossBlob.In(res.Box) {
		t.Errorf("box %v does not contain the cross %v", res.Box, crossBlob)
	}
	if res.Box.Dx()*2 > crossBlob.Dx()*3 || res.Box.Dy()*2 > crossBlob.Dy()*3 {
		t.Errorf("box %v is more than 50%% larger than the cross %v", res.Box, crossBlob)
	}
	if res.FromRegion {
		t.Error("FromRegion should be false when the box was found")
	}

	wantLit := filepath.Join(dir, "room_lit.png")
	wantUnlit := filepath.Join(dir, "room_unlit.png")
	if res.LitPath != wantLit || res.UnlitPath != wantUnlit {
		t.Errorf("paths = %q, %q; want %q, %q", res.LitPath, res.UnlitPath, wantLit, wantUnlit)
	}
	for _, path := range []string{wantLit, wantUnlit} {
		img := readPNGFile(t, path)
		if img.Bounds().Dx() != res.Box.Dx() || img.Bounds().Dy() != res.Box.Dy() {
			t.Errorf("%s is %v, want the box's size %dx%d", path, img.Bounds().Size(), res.Box.Dx(), res.Box.Dy())
		}
	}

	// The lit crop shows the bright cross; the unlit crop does not.
	lit := readPNGFile(t, wantLit)
	unlit := readPNGFile(t, wantUnlit)
	cx, cy := 730-res.Box.Min.X, 350-res.Box.Min.Y
	if y := luma(lit.At(cx, cy)); y < 200 {
		t.Errorf("lit crop at the cross centre is %d, want bright", y)
	}
	if y := luma(unlit.At(cx, cy)); y > 60 {
		t.Errorf("unlit crop at the cross centre is %d, want dark", y)
	}
}

func TestAddTemplate_RegionOverridesTheSearch(t *testing.T) {
	dir := t.TempDir()
	frames := &fakeFrames{frames: map[string]image.Image{
		"00:14:06": syntheticFrame(0),
		"00:14:07": syntheticFrame(240),
	}}
	region := image.Rect(100, 50, 140, 110)

	res, err := AddTemplate(context.Background(), frames, AddTemplateRequest{
		VideoPath:    "v.mp4",
		UnlitAt:      "00:14:06",
		LitAt:        "00:14:07",
		Name:         "room",
		TemplatesDir: dir,
		Region:       region,
	})
	if err != nil {
		t.Fatalf("AddTemplate: %v", err)
	}
	if res.Box != region {
		t.Errorf("box = %v, want the given region %v", res.Box, region)
	}
	if !res.FromRegion {
		t.Error("FromRegion should be true when --region was given")
	}
	if got := readPNGFile(t, res.LitPath).Bounds().Size(); got != region.Size() {
		t.Errorf("lit template is %v, want %v", got, region.Size())
	}
}

func TestAddTemplate_RegionOutsideTheFrameIsRefused(t *testing.T) {
	frames := &fakeFrames{frames: map[string]image.Image{
		"00:14:06": syntheticFrame(0),
		"00:14:07": syntheticFrame(240),
	}}
	_, err := AddTemplate(context.Background(), frames, AddTemplateRequest{
		VideoPath: "v.mp4", UnlitAt: "00:14:06", LitAt: "00:14:07", Name: "room",
		TemplatesDir: t.TempDir(), Region: image.Rect(1250, 700, 1300, 760),
	})
	if err == nil || !strings.Contains(err.Error(), "outside the frame") {
		t.Fatalf("want an 'outside the frame' error, got %v", err)
	}
}

func TestAddTemplate_RefusesAnExistingNameWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wide_lit.png"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	frames := &fakeFrames{frames: map[string]image.Image{
		"00:14:06": syntheticFrame(0),
		"00:14:07": syntheticFrame(240),
	}}
	req := AddTemplateRequest{
		VideoPath: "v.mp4", UnlitAt: "00:14:06", LitAt: "00:14:07", Name: "wide", TemplatesDir: dir,
	}

	_, err := AddTemplate(context.Background(), frames, req)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want a refusal naming --force, got %v", err)
	}
	if len(frames.asked) != 0 {
		t.Errorf("refused before extracting anything, but asked for frames %v", frames.asked)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "wide_lit.png")); string(b) != "old" {
		t.Error("the existing template was changed")
	}

	req.Force = true
	res, err := AddTemplate(context.Background(), frames, req)
	if err != nil {
		t.Fatalf("with --force: %v", err)
	}
	if got := readPNGFile(t, res.LitPath).Bounds().Size(); got != res.Box.Size() {
		t.Errorf("replaced template is %v, want %v", got, res.Box.Size())
	}
}

func TestAddTemplate_NoBrightnessRiseSaysSo(t *testing.T) {
	frames := &fakeFrames{frames: map[string]image.Image{
		"00:14:06": syntheticFrame(0),
		"00:14:07": syntheticFrame(0),
	}}
	_, err := AddTemplate(context.Background(), frames, AddTemplateRequest{
		VideoPath: "v.mp4", UnlitAt: "00:14:06", LitAt: "00:14:07", Name: "room", TemplatesDir: t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "--region") {
		t.Fatalf("want an error suggesting --region, got %v", err)
	}
}

func TestAddTemplate_RefusesBadNamesAndTimes(t *testing.T) {
	frames := &fakeFrames{}
	for _, req := range []AddTemplateRequest{
		{Name: "../room", LitAt: "00:14:07", UnlitAt: "00:14:06"},
		{Name: "room_lit", LitAt: "00:14:07", UnlitAt: "00:14:06"},
		{Name: "", LitAt: "00:14:07", UnlitAt: "00:14:06"},
		{Name: "room", LitAt: "14:07", UnlitAt: "00:14:06"},
		{Name: "room", LitAt: "00:14:07", UnlitAt: "nope"},
	} {
		req.VideoPath = "v.mp4"
		req.TemplatesDir = t.TempDir()
		if _, err := AddTemplate(context.Background(), frames, req); err == nil {
			t.Errorf("%+v: want an error", req)
		}
	}
}

func TestParseRegion(t *testing.T) {
	r, err := ParseRegion("700, 300,60,90")
	if err != nil {
		t.Fatalf("ParseRegion: %v", err)
	}
	if want := image.Rect(700, 300, 760, 390); r != want {
		t.Errorf("got %v, want %v", r, want)
	}
	for _, bad := range []string{"", "1,2,3", "1,2,0,4", "a,b,c,d", "-1,2,3,4"} {
		if _, err := ParseRegion(bad); err == nil {
			t.Errorf("ParseRegion(%q): want an error", bad)
		}
	}
}

func TestTemplateFiles_FindsEveryShotWithBothFrames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"wide_lit.png", "wide_unlit.png", "room_lit.png", "room_unlit.png", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := TemplateFiles(dir)
	if err != nil {
		t.Fatalf("TemplateFiles: %v", err)
	}
	want := map[string]string{
		"wide_lit":   filepath.Join(dir, "wide_lit.png"),
		"wide_unlit": filepath.Join(dir, "wide_unlit.png"),
		"room_lit":   filepath.Join(dir, "room_lit.png"),
		"room_unlit": filepath.Join(dir, "room_unlit.png"),
	}
	if len(files) != len(want) {
		t.Fatalf("got %v, want %v", files, want)
	}
	for k, v := range want {
		if files[k] != v {
			t.Errorf("%s = %q, want %q", k, files[k], v)
		}
	}
}

func TestTemplateFiles_AShotWithoutItsPairIsAnError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "room_lit.png"), nil, 0o644)
	if _, err := TemplateFiles(dir); err == nil || !strings.Contains(err.Error(), "room_unlit.png") {
		t.Fatalf("want an error naming room_unlit.png, got %v", err)
	}
	if _, err := TemplateFiles(t.TempDir()); err == nil {
		t.Fatal("an empty folder should be an error")
	}
}

// TestAddTemplate_FromARealVideo runs ffmpeg on a generated video in which a
// small white box appears after one second.
func TestAddTemplate_FromARealVideo(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "service.mp4")
	gen := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "color=c=0x202020:s=320x240:d=3:r=10",
		"-vf", "drawbox=x=150:y=90:w=12:h=20:color=white:t=fill:enable='gte(t,1.5)'",
		"-pix_fmt", "yuv420p", "-y", videoPath)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate test video: %v\n%s", err, out)
	}

	res, err := AddTemplate(context.Background(), FFmpegFrames{FFmpegPath: ffmpeg}, AddTemplateRequest{
		VideoPath: videoPath, UnlitAt: "00:00:00", LitAt: "00:00:02", Name: "room", TemplatesDir: filepath.Join(dir, "templates"),
	})
	if err != nil {
		t.Fatalf("AddTemplate: %v", err)
	}
	box := image.Rect(150, 90, 162, 110)
	if !box.In(res.Box) {
		t.Errorf("box %v does not contain the drawn box %v", res.Box, box)
	}
	if res.Box.Dx() > 24 || res.Box.Dy() > 34 {
		t.Errorf("box %v is far larger than the drawn box %v", res.Box, box)
	}
	if got := readPNGFile(t, res.UnlitPath).Bounds().Size(); got != res.Box.Size() {
		t.Errorf("unlit template is %v, want %v", got, res.Box.Size())
	}
}

func TestFFmpegFrames_WritesOneFullFrame(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "testsrc.mp4")
	gen := exec.Command(ffmpeg, "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=3:size=320x240:rate=10",
		"-pix_fmt", "yuv420p", "-y", videoPath)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate test video: %v\n%s", err, out)
	}
	out := filepath.Join(dir, "frame.png")
	if err := (FFmpegFrames{FFmpegPath: ffmpeg}).ExtractFrame(context.Background(), videoPath, "00:00:01", out); err != nil {
		t.Fatalf("ExtractFrame: %v", err)
	}
	if got := readPNGFile(t, out).Bounds().Size(); got != image.Pt(320, 240) {
		t.Errorf("frame is %v, want 320x240", got)
	}

	// A time past the end of the video is an error, not an empty file.
	err := (FFmpegFrames{FFmpegPath: ffmpeg}).ExtractFrame(context.Background(), videoPath, "00:01:00", filepath.Join(dir, "late.png"))
	if err == nil {
		t.Error("want an error for a time past the end of the video")
	}
}

func requireFFmpeg(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed, so this test cannot generate a test video: install ffmpeg to run it")
	}
	return path
}
