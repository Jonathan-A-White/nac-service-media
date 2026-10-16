package detection

// Learning a new cross shot from a video. This file has no build tag: it uses
// only Go's image packages and the ffmpeg binary, so the gate tests it.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"nac-service-media/domain/video"
)

// DefaultTemplatesDir is where the cross templates live when the config does
// not say.
const DefaultTemplatesDir = "config/detection_templates"

// FrameExtractor writes the full frame of a video at a time (HH:MM:SS) to a PNG.
type FrameExtractor interface {
	ExtractFrame(ctx context.Context, videoPath, at, outPath string) error
}

// FFmpegFrames extracts frames with the ffmpeg binary.
type FFmpegFrames struct {
	FFmpegPath string // "ffmpeg" when empty
}

// ExtractFrame writes the frame at the given time to outPath as a PNG. A time
// past the end of the video is an error.
func (f FFmpegFrames) ExtractFrame(ctx context.Context, videoPath, at, outPath string) error {
	ffmpeg := f.FFmpegPath
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	os.Remove(outPath)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpeg, "-loglevel", "error", "-ss", at, "-i", videoPath, "-frames:v", "1", "-y", outPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg could not extract the frame at %s from %s: %v: %s", at, videoPath, err, strings.TrimSpace(stderr.String()))
	}
	if info, err := os.Stat(outPath); err != nil || info.Size() == 0 {
		return fmt.Errorf("ffmpeg wrote no frame at %s from %s: is the video that long?", at, videoPath)
	}
	return nil
}

// ParseRegion reads "x,y,w,h" (pixels of the full frame) as a rectangle.
func ParseRegion(s string) (image.Rectangle, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, fmt.Errorf("region %q: want x,y,w,h", s)
	}
	var n [4]int
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || v < 0 {
			return image.Rectangle{}, fmt.Errorf("region %q: want four whole numbers x,y,w,h", s)
		}
		n[i] = v
	}
	if n[2] == 0 || n[3] == 0 {
		return image.Rectangle{}, fmt.Errorf("region %q: width and height must be more than 0", s)
	}
	return image.Rect(n[0], n[1], n[0]+n[2], n[1]+n[3]), nil
}

// FormatRegion writes a rectangle as "x=.. y=.. w=.. h=..".
func FormatRegion(r image.Rectangle) string {
	return fmt.Sprintf("x=%d y=%d w=%d h=%d", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

// minRise is the smallest brightness rise (0-255) counted as the cross
// lighting up rather than video noise.
const minRise = 24

// FindCrossBox finds the cross as the part of the frame that brightens most
// from the unlit frame to the lit one: the bounding box of the connected patch
// of brightened pixels with the largest total rise, padded by a fifth of its
// size on each side (so at most 40% larger) and kept inside the frame.
func FindCrossBox(unlit, lit image.Image) (image.Rectangle, error) {
	bounds := lit.Bounds()
	if unlit.Bounds() != bounds {
		return image.Rectangle{}, fmt.Errorf("the lit frame is %v and the unlit frame is %v: they must be from the same video", bounds.Size(), unlit.Bounds().Size())
	}
	w, h := bounds.Dx(), bounds.Dy()
	rise := make([]int, w*h)
	maxRise := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := int(luma(lit.At(bounds.Min.X+x, bounds.Min.Y+y))) - int(luma(unlit.At(bounds.Min.X+x, bounds.Min.Y+y)))
			if r > 0 {
				rise[y*w+x] = r
				if r > maxRise {
					maxRise = r
				}
			}
		}
	}
	if maxRise < minRise {
		return image.Rectangle{}, errors.New("nothing brightens between the unlit and the lit frame: check the two times (the cross must be off at --unlit-at and on at --lit-at), or give the cross's box with --region x,y,w,h")
	}
	threshold := maxRise / 2
	if threshold < minRise {
		threshold = minRise
	}

	// Label the patches of brightened pixels; pixels up to two apart join, so
	// a cross broken by a dark seam is one patch.
	const reach = 2
	seen := make([]bool, w*h)
	var best image.Rectangle
	bestTotal := 0
	var stack []int
	for start := range rise {
		if seen[start] || rise[start] < threshold {
			continue
		}
		seen[start] = true
		stack = append(stack[:0], start)
		box := image.Rect(start%w, start/w, start%w+1, start/w+1)
		total := 0
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := i%w, i/w
			total += rise[i]
			box = box.Union(image.Rect(x, y, x+1, y+1))
			for dy := -reach; dy <= reach; dy++ {
				for dx := -reach; dx <= reach; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					j := ny*w + nx
					if !seen[j] && rise[j] >= threshold {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		if total > bestTotal {
			best, bestTotal = box, total
		}
	}

	padX, padY := max(1, best.Dx()/5), max(1, best.Dy()/5)
	padded := image.Rect(best.Min.X-padX, best.Min.Y-padY, best.Max.X+padX, best.Max.Y+padY)
	return padded.Add(bounds.Min).Intersect(bounds), nil
}

// luma is a pixel's brightness, 0-255.
func luma(c color.Color) uint8 {
	return color.GrayModel.Convert(c).(color.Gray).Y
}

// cropImage copies the part of img inside r into a new image whose top-left
// corner is (0,0).
func cropImage(img image.Image, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), img, r.Min, draw.Src)
	return out
}

// toGray converts an image to greyscale, as the detector reads templates.
func toGray(img image.Image) *image.Gray {
	out := image.NewGray(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out
}

// AddTemplateRequest is one 'templates add': the video, the two times, the
// shot's name and where the templates go.
type AddTemplateRequest struct {
	VideoPath    string
	LitAt        string // HH:MM:SS, the cross on
	UnlitAt      string // HH:MM:SS, the cross off
	Name         string // the shot, e.g. "room"
	TemplatesDir string
	Region       image.Rectangle // the cross's box; empty: find it
	Force        bool            // replace a shot of the same name
}

// AddTemplateResult says which box was cropped and where the two PNGs went.
type AddTemplateResult struct {
	Box        image.Rectangle
	FromRegion bool // the box was given, not found
	FrameSize  image.Point
	LitPath    string
	UnlitPath  string
}

var shotName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// AddTemplate learns a cross shot: it extracts the unlit and the lit frame,
// finds the cross's box (or takes the given one), crops both frames to it and
// writes <name>_lit.png and <name>_unlit.png into the templates folder.
func AddTemplate(ctx context.Context, frames FrameExtractor, req AddTemplateRequest) (AddTemplateResult, error) {
	if !shotName.MatchString(req.Name) || strings.HasSuffix(req.Name, "_lit") || strings.HasSuffix(req.Name, "_unlit") {
		return AddTemplateResult{}, fmt.Errorf("shot name %q: use letters, digits, '-' and '_' (for example 'room'), not ending in _lit or _unlit", req.Name)
	}
	for flag, at := range map[string]string{"--lit-at": req.LitAt, "--unlit-at": req.UnlitAt} {
		if _, err := video.ParseTimestamp(at); err != nil {
			return AddTemplateResult{}, fmt.Errorf("%s: %w", flag, err)
		}
	}
	res := AddTemplateResult{
		LitPath:   filepath.Join(req.TemplatesDir, req.Name+"_lit.png"),
		UnlitPath: filepath.Join(req.TemplatesDir, req.Name+"_unlit.png"),
	}
	if !req.Force {
		for _, path := range []string{res.LitPath, res.UnlitPath} {
			if _, err := os.Stat(path); err == nil {
				return AddTemplateResult{}, fmt.Errorf("a shot named %q already exists (%s): choose another --name, or add --force to replace it", req.Name, path)
			}
		}
	}

	tmp, err := os.MkdirTemp("", "nac-templates-*")
	if err != nil {
		return AddTemplateResult{}, fmt.Errorf("failed to create a temporary folder: %w", err)
	}
	defer os.RemoveAll(tmp)
	unlit, err := frameAt(ctx, frames, req.VideoPath, req.UnlitAt, filepath.Join(tmp, "unlit.png"))
	if err != nil {
		return AddTemplateResult{}, err
	}
	lit, err := frameAt(ctx, frames, req.VideoPath, req.LitAt, filepath.Join(tmp, "lit.png"))
	if err != nil {
		return AddTemplateResult{}, err
	}
	res.FrameSize = lit.Bounds().Size()

	if req.Region.Empty() {
		res.Box, err = FindCrossBox(unlit, lit)
		if err != nil {
			return AddTemplateResult{}, err
		}
	} else {
		frame := image.Rectangle{Max: res.FrameSize}
		if !req.Region.In(frame) {
			return AddTemplateResult{}, fmt.Errorf("region %s is outside the frame (%dx%d)", FormatRegion(req.Region), res.FrameSize.X, res.FrameSize.Y)
		}
		res.Box, res.FromRegion = req.Region, true
	}

	if err := os.MkdirAll(req.TemplatesDir, 0o755); err != nil {
		return AddTemplateResult{}, fmt.Errorf("failed to create the templates folder: %w", err)
	}
	box := res.Box.Add(lit.Bounds().Min)
	if err := writePNG(res.LitPath, cropImage(lit, box)); err != nil {
		return AddTemplateResult{}, err
	}
	if err := writePNG(res.UnlitPath, cropImage(unlit, box)); err != nil {
		return AddTemplateResult{}, err
	}
	return res, nil
}

// frameAt extracts and decodes the frame at one time.
func frameAt(ctx context.Context, frames FrameExtractor, videoPath, at, path string) (image.Image, error) {
	if err := frames.ExtractFrame(ctx, videoPath, at, path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open the frame at %s: %w", at, err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read the frame at %s: %w", at, err)
	}
	return img, nil
}

func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// TemplateFiles finds every shot in the templates folder: each
// <shot>_lit.png with its <shot>_unlit.png. It returns template names
// ("room_lit", "room_unlit", ...) and their paths.
func TemplateFiles(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read the templates folder: %w", err)
	}
	present := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			present[e.Name()] = true
		}
	}
	files := map[string]string{}
	for name := range present {
		var shot, pair string
		switch {
		case strings.HasSuffix(name, "_unlit.png"):
			shot = strings.TrimSuffix(name, "_unlit.png")
			pair = shot + "_lit.png"
		case strings.HasSuffix(name, "_lit.png"):
			shot = strings.TrimSuffix(name, "_lit.png")
			pair = shot + "_unlit.png"
		default:
			continue
		}
		if !present[pair] {
			return nil, fmt.Errorf("template file not found: %s (shot %q has only %s)", filepath.Join(dir, pair), shot, name)
		}
		files[strings.TrimSuffix(name, ".png")] = filepath.Join(dir, name)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no cross templates (<shot>_lit.png and <shot>_unlit.png) in %s: add one with 'nac-service-media templates add'", dir)
	}
	return files, nil
}
