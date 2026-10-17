package detection

// The truth table: recordings whose true start (the moment the cross lights)
// a person checked after a wrong trim, each kept with the frames the detector
// needs to find it, so a change to the detector can never miss it again
// unnoticed. 'nac-service-media truth add' writes a case; TestTruthTable
// replays every case. No build tag: the gate runs it.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nac-service-media/domain/detection"
	"nac-service-media/domain/video"
)

// TruthTableFile is the table's name in the truth folder (testdata/truth).
const TruthTableFile = "truth.tsv"

// TruthToleranceSeconds is how far from the true start the detector's may be.
const TruthToleranceSeconds = 10

// TruthCase is one row of the table. Times are seconds into the recording.
type TruthCase struct {
	Date        string // YYYY-MM-DD, the service
	Recording   string // the recording's file name
	TrueStart   int    // when the cross lit, as a person saw it
	Detected    int    // what the detector found when the row was added; -1 none
	TrimmedFrom int    // where the wrong trim started, when given; -1 not given
}

// FramesDir is the case's folder of frames in the truth folder: the
// recording's name without its extension.
func (c TruthCase) FramesDir(truthDir string) string {
	return filepath.Join(truthDir, strings.TrimSuffix(c.Recording, filepath.Ext(c.Recording)))
}

const truthHeader = `# The truth table: recordings whose true start (the moment the cross lights)
# a person checked, one row each, tab-separated. Each row's frames are in the
# folder named after the recording. go test replays every row and fails when
# the detector's start is more than 10 s from the true start.
# Add or replace a row with: nac-service-media truth add <recording> <hh:mm:ss>
# detected: what the detector found when the row was added (none: no start).
# trimmed_from: where the wrong trim started, when given (-: not given).
# date	recording	true_start	detected	trimmed_from
`

// ReadTruthTable reads the table's rows in order.
func ReadTruthTable(path string) ([]TruthCase, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var cases []TruthCase
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "#") {
			continue
		}
		c, err := parseTruthRow(text)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		cases = append(cases, c)
	}
	return cases, scanner.Err()
}

func parseTruthRow(text string) (TruthCase, error) {
	fields := strings.Split(text, "\t")
	if len(fields) != 5 {
		return TruthCase{}, fmt.Errorf("want 5 tab-separated fields (date, recording, true_start, detected, trimmed_from), got %d", len(fields))
	}
	c := TruthCase{Date: fields[0], Recording: fields[1]}
	var err error
	if c.TrueStart, err = parseSeconds(fields[2]); err != nil {
		return c, fmt.Errorf("true_start: %w", err)
	}
	if c.Detected, err = parseOptionalSeconds(fields[3], "none"); err != nil {
		return c, fmt.Errorf("detected: %w", err)
	}
	if c.TrimmedFrom, err = parseOptionalSeconds(fields[4], "-"); err != nil {
		return c, fmt.Errorf("trimmed_from: %w", err)
	}
	return c, nil
}

func parseSeconds(s string) (int, error) {
	ts, err := video.ParseTimestamp(s)
	if err != nil {
		return 0, err
	}
	return ts.TotalSeconds(), nil
}

func parseOptionalSeconds(s, absent string) (int, error) {
	if s == absent {
		return -1, nil
	}
	return parseSeconds(s)
}

func formatOptionalSeconds(seconds int, absent string) string {
	if seconds < 0 {
		return absent
	}
	return secondsToTimestamp(seconds)
}

// WriteTruthCase adds the case to the table, or replaces the row of the same
// recording, making the table when there is none.
func WriteTruthCase(path string, c TruthCase) error {
	cases, err := ReadTruthTable(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	replaced := false
	for i := range cases {
		if cases[i].Recording == c.Recording {
			cases[i], replaced = c, true
		}
	}
	if !replaced {
		cases = append(cases, c)
	}
	var b strings.Builder
	b.WriteString(truthHeader)
	for _, c := range cases {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", c.Date, c.Recording, secondsToTimestamp(c.TrueStart),
			formatOptionalSeconds(c.Detected, "none"), formatOptionalSeconds(c.TrimmedFrom, "-"))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// TruthMiss says how the detector missed the case, or "" when its start is
// within TruthToleranceSeconds of the true one.
func TruthMiss(c TruthCase, res detection.DetectionResult, err error) string {
	if err != nil {
		return fmt.Sprintf("%s: the detector found no start (true start %s): %v", c.Recording, secondsToTimestamp(c.TrueStart), err)
	}
	found := res.Timestamp.TotalSeconds()
	off := found - c.TrueStart
	if off < 0 {
		off = -off
	}
	if off <= TruthToleranceSeconds {
		return ""
	}
	return fmt.Sprintf("%s: the detector found %s, %d s from the true start %s (allowed %d s)",
		c.Recording, secondsToTimestamp(found), off, secondsToTimestamp(c.TrueStart), TruthToleranceSeconds)
}

// Seconds either side of a start whose every frame a case keeps: wider round
// the true start, which a person read off the video by eye.
const (
	truthAroundTrue  = 5
	truthAroundOther = 2
)

// TruthFrameTimes are the frames a case keeps: the coarse scan's, every step
// from 5 s to three minutes and a step past the latest of the case's starts
// (no further than end, the search range's end, when it is above 0), so the
// replayed search sees what it saw on the recording; and every second around
// the true start, the detected one and where the wrong trim started.
func TruthFrameTimes(c TruthCase, step, end int) []int {
	if step <= 0 {
		step = 30
	}
	latest := max(c.TrueStart, c.Detected, c.TrimmedFrom)
	limit := latest + step + confirmSeconds
	if end > 0 && limit > end {
		limit = end
	}
	set := map[int]bool{}
	for _, t := range (crossScan{coarseStep: step, endSeconds: limit}).sampleTimes() {
		set[t] = true
	}
	around := func(at, by int) {
		if at < 0 {
			return
		}
		for t := max(0, at-by); t <= at+by; t++ {
			set[t] = true
		}
	}
	around(c.TrueStart, truthAroundTrue)
	around(c.Detected, truthAroundOther)
	around(c.TrimmedFrom, truthAroundOther)
	times := make([]int, 0, len(set))
	for t := range set {
		times = append(times, t)
	}
	sort.Ints(times)
	return times
}

// truthFrameQuality is the JPEG quality of a kept frame: a third the size of
// the PNG, and every picture of the cross scores the same to two places.
const truthFrameQuality = 85

// SaveTruthFrame keeps a frame (a PNG) in dir as HH-MM-SS.jpg, gray as the
// matcher sees it.
func SaveTruthFrame(pngPath, dir string, seconds int) error {
	in, err := os.Open(pngPath)
	if err != nil {
		return err
	}
	img, err := png.Decode(in)
	in.Close()
	if err != nil {
		return fmt.Errorf("failed to read the frame at %s: %w", secondsToTimestamp(seconds), err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := strings.ReplaceAll(secondsToTimestamp(seconds), ":", "-") + ".jpg"
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	if err := jpeg.Encode(out, toGray(img), &jpeg.Options{Quality: truthFrameQuality}); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

var truthFrameName = regexp.MustCompile(`^(\d{2})-(\d{2})-(\d{2})\.jpg$`)

// ReplayFrames serves a case's kept frames in place of ffmpeg: for a time,
// the frame kept at or before it (the first, for a time before them all).
type ReplayFrames struct {
	dir     string
	seconds []int
}

// NewReplayFrames reads the names of the frames kept in dir.
func NewReplayFrames(dir string) (*ReplayFrames, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	r := &ReplayFrames{dir: dir}
	for _, e := range entries {
		m := truthFrameName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		seconds, err := parseSeconds(m[1] + ":" + m[2] + ":" + m[3])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(dir, e.Name()), err)
		}
		r.seconds = append(r.seconds, seconds)
	}
	if len(r.seconds) == 0 {
		return nil, fmt.Errorf("no frames (HH-MM-SS.jpg) in %s", dir)
	}
	sort.Ints(r.seconds)
	return r, nil
}

// ExtractFrame writes the kept frame for the time as a PNG.
func (r *ReplayFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	seconds, err := parseSeconds(at)
	if err != nil {
		return err
	}
	i := sort.Search(len(r.seconds), func(i int) bool { return r.seconds[i] > seconds }) - 1
	if i < 0 {
		i = 0
	}
	name := strings.ReplaceAll(secondsToTimestamp(r.seconds[i]), ":", "-") + ".jpg"
	in, err := os.Open(filepath.Join(r.dir, name))
	if err != nil {
		return err
	}
	img, err := jpeg.Decode(in)
	in.Close()
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", name, err)
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(out, img); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
