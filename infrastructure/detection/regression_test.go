package detection

// Real recordings the detector once got wrong, replayed from a few of their
// frames (testdata/<date>/HH-MM-SS.png, 1080p, gray as the matcher sees
// them) against the church's own pictures of the cross in
// config/detection_templates.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// replayFrame is a frame the replay serves from a time on, until the next.
type replayFrame struct {
	from int
	file string
}

// replayFrames serves saved frames in place of ffmpeg: for a time, the
// frame with the latest from at or before it.
type replayFrames struct {
	dir    string
	frames []replayFrame
}

func (r *replayFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	ts, err := video.ParseTimestamp(at)
	if err != nil {
		return err
	}
	seconds := ts.TotalSeconds()
	i := sort.Search(len(r.frames), func(i int) bool { return r.frames[i].from > seconds }) - 1
	if i < 0 {
		i = 0
	}
	data, err := os.ReadFile(filepath.Join(r.dir, r.frames[i].file))
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, data, 0o644)
}

// churchTemplates is the folder of the church's pictures of the cross.
const churchTemplates = "../../config/detection_templates"

func churchConfig() config.DetectionConfig {
	return config.DetectionConfig{
		TemplatesDir: churchTemplates,
		SearchRange:  config.SearchRangeConfig{StartMinutes: 0, EndMinutes: 70},
		Thresholds:   config.DetectionThresholdsConfig{MatchScore: 0.75, CoarseStepSeconds: 60},
	}
}

func churchDetector(t *testing.T, frames FrameExtractor, log *bytes.Buffer) *TemplateDetector {
	t.Helper()
	d := NewTemplateDetector(churchConfig(), WithFrameExtractor(frames), WithOutput(log), WithSamplesDir(""))
	if err := d.LoadTemplates(churchTemplates); err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

// 2026-09-27 (mw-6drexa.5): the service started at 00:08:42, and the
// detector trimmed from 00:02:54. Before the service the camera panned from
// the wide shot to the choir rehearsing at the left. Mid-pan (00:02:54) the
// still-unlit cross, moving and brighter, matched the wide2 lit picture at
// half size, in the wrong place; on the choir (00:03:00 to 00:05:00) a white
// shirt matched the closeup lit picture. Each read as a lit cross, and one
// lit frame ended the search. The cross lit at 00:08:42 in the wide shot,
// where it had been seen unlit all along.
func TestRegression_20260927_TheChoirRehearsalIsNotTheStart(t *testing.T) {
	replay := &replayFrames{dir: "testdata/2026-09-27", frames: []replayFrame{
		{0, "00-02-53.png"},   // the wide shot, the cross unlit
		{174, "00-02-54.png"}, // the camera panning to the choir
		{180, "00-03-00.png"}, // the choir close-up
		{330, "00-02-53.png"}, // back on the wide shot, the cross unlit
		{522, "00-08-42.png"}, // the cross lit: the service
	}}
	var log bytes.Buffer
	d := churchDetector(t, replay, &log)

	// What the detector saw in each frame, as the evidence of the cause.
	for _, want := range []struct {
		seconds int
		line    string
		best    string
	}{
		{173, "00:02:53  unlit  wide2 0.77x score 0.77", "wide2_unlit    0.77 at 0.77x, 168x124 at (840,56)"},
		{174, "00:02:54  lit    wide2 0.50x score 0.82", "wide2_lit      0.82 at 0.50x, 112x80 at (1424,172)"},
		{180, "00:03:00  lit    closeup 0.65x score 0.83", "closeup_lit    0.83 at 0.65x, 272x164 at (600,792)"},
		{522, "00:08:42  lit    wide2 0.77x score 0.85", "wide2_lit      0.85 at 0.77x, 168x124 at (840,56)"},
	} {
		in, err := d.Inspect(context.Background(), "2026-09-27 10-22-05.mp4", want.seconds, "")
		if err != nil {
			t.Fatalf("Inspect %d: %v", want.seconds, err)
		}
		if !strings.Contains(in.Line, want.line) || len(in.Details) == 0 || !strings.Contains(in.Details[0], want.best) {
			t.Errorf("the frame at %d s should read %q with best %q, got %q\n%s", want.seconds, want.line, want.best, in.Line, strings.Join(in.Details, "\n"))
		}
	}

	res, err := d.DetectStart(context.Background(), "2026-09-27 10-22-05.mp4")
	if err != nil {
		t.Fatalf("DetectStart: %v\nlog:\n%s", err, log.String())
	}
	if got := res.Timestamp.String(); got != "00:08:42" {
		t.Errorf("found %s, want 00:08:42 (the detector once trimmed from 00:02:54)\nlog:\n%s", got, log.String())
	}
	for _, want := range []string{
		"00:03:00  not the start: the cross was unlit at 00:06:00, and it was lit elsewhere than the cross had been seen unlit",
		"00:08:42  lit    wide2 0.77x score 0.85",
	} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("the log should say %q, got:\n%s", want, log.String())
		}
	}
}
