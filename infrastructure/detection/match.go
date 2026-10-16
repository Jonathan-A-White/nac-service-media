package detection

// The template match: OpenCV's TM_CCOEFF_NORMED (the normalised correlation
// of a zero-mean template over every position of the frame), computed with
// the FFT at a reduced working size, for every picture of the cross at every
// scale from half to double. No build tag: the gate tests all of it.

import (
	"errors"
	"fmt"
	"image"
	"math"
	"math/cmplx"
	"sort"
	"strings"

	"nac-service-media/domain/detection"
)

// DefaultMatchScore is the match score (TM_CCOEFF_NORMED, 0-1) a frame must
// reach, with some picture of the cross at some scale, to count as showing
// the cross. The 10/11 service's frames scored 0.83 with the wide picture at
// its own scale, under the old 0.85; the best score anywhere else in those
// frames, with a picture at least 16 px wide and high, was 0.64.
const DefaultMatchScore = 0.75

// workingWidth is the width the frame is shrunk to, about, before matching:
// a 1080p frame is matched at a quarter of its size.
const workingWidth = 480

// minTemplateSide and minTemplateArea are the smallest a scaled picture may
// be, in working pixels: smaller ones match any bright speck (pictures of
// 14x10 to 20x11 scored over 0.8 on a lamp and on the pews).
const (
	minTemplateSide = 8
	minTemplateArea = 400
)

// minContrastGap is the least the lit and the unlit picture must differ in
// contrast for contrast to decide lit or unlit; under it the scores decide.
const minContrastGap = 0.05

// workingFactor is the whole number the frame is shrunk by.
func workingFactor(width int) int {
	return max(1, int(math.Round(float64(width)/workingWidth)))
}

// crossScales are the scales each picture is tried at: half to double, in
// steps of 2^(1/8) (about 9%), since a picture 10% off the cross's size
// scores about 0.13 lower.
func crossScales() []float64 {
	var scales []float64
	for i := -8; i <= 8; i++ {
		scales = append(scales, math.Round(math.Pow(2, float64(i)/8)*1000)/1000)
	}
	return scales
}

// templateMatch is one picture's best match in a frame over every scale.
type templateMatch struct {
	Name          string  // "wide_lit", "wide_unlit", ...
	Score         float64 // the normalised match score
	Scale         float64 // the picture's scale at the best match
	X, Y          int     // the match's top-left corner, in the full frame
	Width, Height int     // the scaled picture's size, in the full frame
	Contrast      float64 // the frame's cross-to-wall contrast under the match
	LitContrast   float64 // the shot's lit picture's contrast
	UnlitContrast float64 // the shot's unlit picture's contrast
}

// litFraction says where the frame's contrast falls between the unlit
// picture's (0) and the lit picture's (1).
func (m templateMatch) litFraction() float64 {
	gap := m.LitContrast - m.UnlitContrast
	if gap < minContrastGap {
		return 0
	}
	return (m.Contrast - m.UnlitContrast) / gap
}

// shotOf is the shot a template belongs to: "wide" for "wide_lit".
func shotOf(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, "_lit"), "_unlit")
}

// zeroMeanTemplate is a picture with its mean taken out, ready to correlate.
type zeroMeanTemplate struct {
	w, h   int
	values []float64
	norm   float64 // the sum of the squared values
}

func newZeroMeanTemplate(img *image.Gray) *zeroMeanTemplate {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	t := &zeroMeanTemplate{w: w, h: h, values: make([]float64, w*h)}
	var sum float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum += float64(img.Pix[y*img.Stride+x])
		}
	}
	mean := sum / float64(w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := float64(img.Pix[y*img.Stride+x]) - mean
			t.values[y*w+x] = d
			t.norm += d * d
		}
	}
	return t
}

// workingFrame is a frame shrunk to the working size, with its spectrum and
// its running sums, ready to be matched against any number of pictures.
type workingFrame struct {
	gray          *image.Gray
	factor        int
	width, height int // the padded size the spectrum has
	fft           *fft2D
	spectrum      []complex128
	sum, sumSq    []float64 // integral images, (gw+1) x (gh+1)
	product       []complex128
}

// newWorkingFrame shrinks img by factor and prepares it.
func newWorkingFrame(img image.Image, factor int) *workingFrame {
	gray := toGray(img)
	if factor > 1 {
		gray = resample(gray, gray.Bounds().Dx()/factor, gray.Bounds().Dy()/factor)
	}
	gw, gh := gray.Bounds().Dx(), gray.Bounds().Dy()
	wf := &workingFrame{gray: gray, factor: factor, width: nextSmooth(gw), height: nextSmooth(gh)}
	wf.fft = newFFT2D(wf.width, wf.height)
	wf.spectrum = make([]complex128, wf.width*wf.height)
	wf.product = make([]complex128, wf.width*wf.height)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			wf.spectrum[y*wf.width+x] = complex(float64(gray.Pix[y*gray.Stride+x]), 0)
		}
	}
	wf.fft.transform(wf.spectrum, false)
	wf.sum = make([]float64, (gw+1)*(gh+1))
	wf.sumSq = make([]float64, (gw+1)*(gh+1))
	for y := 1; y <= gh; y++ {
		var rowSum, rowSq float64
		for x := 1; x <= gw; x++ {
			v := float64(gray.Pix[(y-1)*gray.Stride+x-1])
			rowSum += v
			rowSq += v * v
			wf.sum[y*(gw+1)+x] = wf.sum[(y-1)*(gw+1)+x] + rowSum
			wf.sumSq[y*(gw+1)+x] = wf.sumSq[(y-1)*(gw+1)+x] + rowSq
		}
	}
	return wf
}

// windowSums are the sum and the sum of squares of the frame under a w x h
// window at (x, y).
func (wf *workingFrame) windowSums(x, y, w, h int) (sum, sumSq float64) {
	stride := wf.gray.Bounds().Dx() + 1
	a, b, c, d := y*stride+x, y*stride+x+w, (y+h)*stride+x, (y+h)*stride+x+w
	return wf.sum[d] - wf.sum[b] - wf.sum[c] + wf.sum[a], wf.sumSq[d] - wf.sumSq[b] - wf.sumSq[c] + wf.sumSq[a]
}

// padSpectrum returns the spectrum, at the frame's padded size, of a picture
// pair packed as lit + i*unlit (either may be nil).
func (wf *workingFrame) padSpectrum(lit, unlit *zeroMeanTemplate) []complex128 {
	data := make([]complex128, wf.width*wf.height)
	if lit != nil {
		for y := 0; y < lit.h; y++ {
			for x := 0; x < lit.w; x++ {
				data[y*wf.width+x] = complex(lit.values[y*lit.w+x], 0)
			}
		}
	}
	if unlit != nil {
		for y := 0; y < unlit.h; y++ {
			for x := 0; x < unlit.w; x++ {
				data[y*wf.width+x] += complex(0, unlit.values[y*unlit.w+x])
			}
		}
	}
	wf.fft.transform(data, false)
	return data
}

// correlatePair correlates the frame with two pictures of one size at once,
// from the spectrum of their packed pair, and returns the raw correlations
// (the first picture's in the real parts, the second's in the imaginary).
func (wf *workingFrame) correlatePair(packed []complex64) []complex128 {
	n := len(wf.spectrum)
	for i := 0; i < n; i++ {
		// Unpack the two spectra from the packed one: Z[k] = A[k] + i B[k]
		// with A and B the spectra of real pictures, so
		// A[k] = (Z[k] + conj(Z[-k]))/2 and B[k] = (Z[k] - conj(Z[-k]))/2i.
		z, zm := complex128(packed[i]), cmplx.Conj(complex128(packed[wf.fft.mirror[i]]))
		a := (z + zm) / 2
		b := (z - zm) / complex(0, 2)
		f := wf.spectrum[i]
		wf.product[i] = f*cmplx.Conj(a) + complex(0, 1)*f*cmplx.Conj(b)
	}
	wf.fft.transform(wf.product, true)
	return wf.product
}

// normalise turns raw correlations into scores: for every position the
// picture fits at, the correlation over the picture's and the window's
// standard deviations; elsewhere -1.
func (wf *workingFrame) normalise(raw func(i int) float64, t *zeroMeanTemplate, scores []float64) {
	gw, gh := wf.gray.Bounds().Dx(), wf.gray.Bounds().Dy()
	n := float64(t.w * t.h)
	for i := range scores {
		scores[i] = -1
	}
	for y := 0; y+t.h <= gh; y++ {
		for x := 0; x+t.w <= gw; x++ {
			sum, sumSq := wf.windowSums(x, y, t.w, t.h)
			variance := sumSq - sum*sum/n
			denom := math.Sqrt(math.Max(variance, 0) * t.norm)
			if denom < 1e-9 {
				scores[y*wf.width+x] = 0
				continue
			}
			scores[y*wf.width+x] = math.Max(-1, math.Min(1, raw(y*wf.width+x)/denom))
		}
	}
}

// scores is one picture's score at every position (row-major at the padded
// width; -1 where the picture does not fit).
func (wf *workingFrame) scores(t *zeroMeanTemplate) []float64 {
	spec := wf.padSpectrum(t, nil)
	for i := range spec {
		wf.product[i] = wf.spectrum[i] * cmplx.Conj(spec[i])
	}
	wf.fft.transform(wf.product, true)
	out := make([]float64, len(spec))
	wf.normalise(func(i int) float64 { return real(wf.product[i]) }, t, out)
	return out
}

// bestPosition is the top-scoring position of a score map.
func (wf *workingFrame) bestPosition(scores []float64, t *zeroMeanTemplate) (x, y int, score float64) {
	gw, gh := wf.gray.Bounds().Dx(), wf.gray.Bounds().Dy()
	score = -2
	for oy := 0; oy+t.h <= gh; oy++ {
		for ox := 0; ox+t.w <= gw; ox++ {
			if s := scores[oy*wf.width+ox]; s > score {
				x, y, score = ox, oy, s
			}
		}
	}
	return x, y, score
}

// shot is one camera shot of the cross: its lit and its unlit picture.
type shot struct {
	name       string
	lit, unlit *image.Gray
}

// scaledShot is a shot's two pictures at one scale and working size.
type scaledShot struct {
	shot                       *shot
	scale                      float64
	lit, unlit                 *zeroMeanTemplate
	bar, wall                  []bool // from the lit picture: its brightest tenth and its darker half
	litContrast, unlitContrast float64
	spectrum                   []complex64 // of lit + i*unlit at the frame's padded size
}

// matcher holds every shot and matches frames against all of them.
type matcher struct {
	shots     []*shot
	scales    []float64
	threshold float64

	// Prepared for the frame size last seen.
	factor                 int
	width, height          int
	prepared               []*scaledShot
	litScores, unlitScores []float64 // scratch
}

// newMatcher takes the pictures by template name ("wide_lit", "wide_unlit")
// and the score a frame must reach to show the cross (0: the default).
func newMatcher(templates map[string]*image.Gray, threshold float64) (*matcher, error) {
	if threshold == 0 {
		threshold = DefaultMatchScore
	}
	byShot := map[string]*shot{}
	for name, img := range templates {
		s := byShot[shotOf(name)]
		if s == nil {
			s = &shot{name: shotOf(name)}
			byShot[s.name] = s
		}
		if strings.HasSuffix(name, "_lit") {
			s.lit = img
		} else if strings.HasSuffix(name, "_unlit") {
			s.unlit = img
		} else {
			return nil, fmt.Errorf("template %q is neither <shot>_lit nor <shot>_unlit", name)
		}
	}
	m := &matcher{scales: crossScales(), threshold: threshold}
	for _, s := range byShot {
		if s.lit == nil || s.unlit == nil {
			return nil, fmt.Errorf("shot %q needs both %s_lit and %s_unlit", s.name, s.name, s.name)
		}
		if s.lit.Bounds().Size() != s.unlit.Bounds().Size() {
			return nil, fmt.Errorf("shot %q: %s_lit is %v and %s_unlit is %v; they must be the same size", s.name, s.name, s.lit.Bounds().Size(), s.name, s.unlit.Bounds().Size())
		}
		m.shots = append(m.shots, s)
	}
	if len(m.shots) == 0 {
		return nil, errors.New("no cross templates")
	}
	sort.Slice(m.shots, func(i, j int) bool { return m.shots[i].name < m.shots[j].name })
	return m, nil
}

// prepare scales every shot for frames of the given working size.
func (m *matcher) prepare(wf *workingFrame) {
	if m.factor == wf.factor && m.width == wf.width && m.height == wf.height {
		return
	}
	m.factor, m.width, m.height = wf.factor, wf.width, wf.height
	m.prepared = nil
	gw, gh := wf.gray.Bounds().Dx(), wf.gray.Bounds().Dy()
	for _, s := range m.shots {
		fullW, fullH := s.lit.Bounds().Dx(), s.lit.Bounds().Dy()
		for _, scale := range m.scales {
			w := int(math.Round(float64(fullW) * scale / float64(wf.factor)))
			h := int(math.Round(float64(fullH) * scale / float64(wf.factor)))
			if w < minTemplateSide || h < minTemplateSide || w*h < minTemplateArea || w > gw || h > gh {
				continue
			}
			lit, unlit := resample(s.lit, w, h), resample(s.unlit, w, h)
			ss := &scaledShot{shot: s, scale: scale, lit: newZeroMeanTemplate(lit), unlit: newZeroMeanTemplate(unlit)}
			ss.bar, ss.wall = contrastMasks(lit)
			ss.litContrast = contrastUnder(lit, 0, 0, w, h, ss.bar, ss.wall)
			ss.unlitContrast = contrastUnder(unlit, 0, 0, w, h, ss.bar, ss.wall)
			spec := wf.padSpectrum(ss.lit, ss.unlit)
			ss.spectrum = make([]complex64, len(spec))
			for i, v := range spec {
				ss.spectrum[i] = complex64(v)
			}
			m.prepared = append(m.prepared, ss)
		}
	}
	m.litScores = make([]float64, wf.width*wf.height)
	m.unlitScores = make([]float64, wf.width*wf.height)
}

// frameReport is what one frame showed: the reading, how lit the cross was
// (0: as the unlit picture, 1: as the lit one), and every picture's best
// match over all scales, highest score first.
type frameReport struct {
	detection.FrameAnalysis
	Litness float64
	Matches []templateMatch
}

// best is the highest-scoring match, or the zero match when there is none.
func (r frameReport) best() templateMatch {
	if len(r.Matches) == 0 {
		return templateMatch{}
	}
	return r.Matches[0]
}

// analyze matches one frame against every shot at every scale and reads it.
func (m *matcher) analyze(img image.Image, seconds int) frameReport {
	wf := newWorkingFrame(img, workingFactor(img.Bounds().Dx()))
	m.prepare(wf)
	bests := map[string]templateMatch{}
	for _, ss := range m.prepared {
		raw := wf.correlatePair(ss.spectrum)
		wf.normalise(func(i int) float64 { return real(raw[i]) }, ss.lit, m.litScores)
		wf.normalise(func(i int) float64 { return imag(raw[i]) }, ss.unlit, m.unlitScores)
		for _, pick := range []struct {
			suffix string
			scores []float64
			t      *zeroMeanTemplate
		}{{"_lit", m.litScores, ss.lit}, {"_unlit", m.unlitScores, ss.unlit}} {
			x, y, score := wf.bestPosition(pick.scores, pick.t)
			name := ss.shot.name + pick.suffix
			if prev, ok := bests[name]; ok && prev.Score >= score {
				continue
			}
			bests[name] = templateMatch{
				Name:          name,
				Score:         score,
				Scale:         ss.scale,
				X:             x * wf.factor,
				Y:             y * wf.factor,
				Width:         pick.t.w * wf.factor,
				Height:        pick.t.h * wf.factor,
				Contrast:      contrastUnder(wf.gray, x, y, pick.t.w, pick.t.h, ss.bar, ss.wall),
				LitContrast:   ss.litContrast,
				UnlitContrast: ss.unlitContrast,
			}
		}
	}
	matches := make([]templateMatch, 0, len(bests))
	for _, b := range bests {
		matches = append(matches, b)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})
	analysis, litness := classifyMatches(matches, m.threshold, seconds)
	return frameReport{FrameAnalysis: analysis, Litness: litness, Matches: matches}
}

// classifyMatches reads a frame from its matches. The best score says whether
// a shot is in view, which, and at what scale. Lit or unlit is then decided
// by how lit the cross looks, from 0 (as the shot's unlit picture) to 1 (as
// its lit picture): the frame's cross-to-wall contrast set between the two
// pictures' contrasts, plus the lit picture's score over the unlit one's.
// Lit is from 0.5. Neither alone is safe: the score ignores brightness and
// contrast, so it cannot tell a lit cross from an unlit one of the same
// shape; the contrast fails when a brighter exposure clips the lit cross to
// white, where the shape still tells. Brightness alone (the frame's mean
// under the match against the pictures' means) changes with exposure more
// than the wide shot's two pictures differ. When the two pictures hardly
// differ in contrast, the contrast counts as halfway and the scores decide.
func classifyMatches(matches []templateMatch, threshold float64, seconds int) (detection.FrameAnalysis, float64) {
	if threshold == 0 {
		threshold = DefaultMatchScore
	}
	var best *templateMatch
	for i := range matches {
		m := &matches[i]
		if best == nil || m.Score > best.Score || (m.Score == best.Score && m.Name < best.Name) {
			best = m
		}
	}
	if best == nil || best.Score < threshold {
		analysis := detection.FrameAnalysis{State: detection.StateNotVisible, TimestampSeconds: seconds}
		if best != nil {
			analysis.Confidence, analysis.Scale = best.Score, best.Scale
		}
		return analysis, 0
	}

	shot := shotOf(best.Name)
	var litScore, unlitScore float64
	for _, m := range matches {
		switch m.Name {
		case shot + "_lit":
			litScore = m.Score
		case shot + "_unlit":
			unlitScore = m.Score
		}
	}
	litness := 0.5
	if best.LitContrast-best.UnlitContrast >= minContrastGap {
		litness = best.litFraction()
	}
	litness += litScore - unlitScore
	state := detection.StateUnlit
	if litness >= 0.5 {
		state = detection.StateLit
	}
	return detection.FrameAnalysis{
		State:            state,
		CameraAngle:      shot,
		Confidence:       best.Score,
		Scale:            best.Scale,
		TimestampSeconds: seconds,
	}, litness
}

// contrastMasks marks, in a lit picture, the cross (its brightest tenth of
// pixels) and the wall (its darker half).
func contrastMasks(lit *image.Gray) (bar, wall []bool) {
	w, h := lit.Bounds().Dx(), lit.Bounds().Dy()
	sorted := make([]uint8, 0, w*h)
	for y := 0; y < h; y++ {
		sorted = append(sorted, lit.Pix[y*lit.Stride:y*lit.Stride+w]...)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	barLevel, wallLevel := sorted[len(sorted)*9/10], sorted[len(sorted)/2]
	bar, wall = make([]bool, w*h), make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := lit.Pix[y*lit.Stride+x]
			bar[y*w+x] = v >= barLevel
			wall[y*w+x] = v <= wallLevel && v < barLevel
		}
	}
	return bar, wall
}

// contrastUnder is how much brighter the cross is than the wall in the w x h
// window of img at (x, y), as a fraction of the wall's brightness, with the
// cross and the wall where the masks say.
func contrastUnder(img *image.Gray, x, y, w, h int, bar, wall []bool) float64 {
	var barSum, barN, wallSum, wallN float64
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			v := float64(img.Pix[(y+j)*img.Stride+x+i])
			switch {
			case bar[j*w+i]:
				barSum += v
				barN++
			case wall[j*w+i]:
				wallSum += v
				wallN++
			}
		}
	}
	if barN == 0 || wallN == 0 {
		return 0
	}
	wallMean := wallSum / wallN
	return (barSum/barN - wallMean) / math.Max(wallMean, 1)
}

// resample scales src to w x h with a tent filter as wide as the change of
// size, so shrinking averages and growing interpolates.
func resample(src *image.Gray, w, h int) *image.Gray {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	w, h = max(w, 1), max(h, 1)
	xk, yk := resampleKernels(sw, w), resampleKernels(sh, h)
	tmp := make([]float64, w*sh)
	for y := 0; y < sh; y++ {
		row := src.Pix[y*src.Stride : y*src.Stride+sw]
		for x, k := range xk {
			var v float64
			for j, wt := range k.weights {
				v += wt * float64(row[k.start+j])
			}
			tmp[y*w+x] = v
		}
	}
	out := image.NewGray(image.Rect(0, 0, w, h))
	for y, k := range yk {
		for x := 0; x < w; x++ {
			var v float64
			for j, wt := range k.weights {
				v += wt * tmp[(k.start+j)*w+x]
			}
			out.Pix[y*out.Stride+x] = uint8(math.Max(0, math.Min(255, math.Round(v))))
		}
	}
	return out
}

type resampleKernel struct {
	start   int
	weights []float64
}

func resampleKernels(srcN, dstN int) []resampleKernel {
	scale := float64(srcN) / float64(dstN)
	support := math.Max(1, scale)
	kernels := make([]resampleKernel, dstN)
	for i := range kernels {
		centre := (float64(i) + 0.5) * scale
		lo := max(0, int(math.Floor(centre-support)))
		hi := min(srcN-1, int(math.Ceil(centre+support)))
		var weights []float64
		var total float64
		for j := lo; j <= hi; j++ {
			wt := math.Max(0, 1-math.Abs(float64(j)+0.5-centre)/support)
			weights = append(weights, wt)
			total += wt
		}
		if total == 0 {
			weights, total = []float64{1}, 1
			lo = min(srcN-1, int(centre))
		}
		for j := range weights {
			weights[j] /= total
		}
		kernels[i] = resampleKernel{start: lo, weights: weights}
	}
	return kernels
}
