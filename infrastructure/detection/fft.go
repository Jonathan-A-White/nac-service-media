package detection

// A fast Fourier transform for the template match. The working frame is
// padded to a size whose prime factors are 2, 3 and 5, so the mixed-radix
// transform below is all the match needs; any other size still works, just
// more slowly.

import (
	"math"
	"math/cmplx"
	"sync"
)

// fftPlan transforms sequences of one length.
type fftPlan struct {
	n       int
	factors []int                // the prime factors of n, largest first
	twiddle []complex128         // exp(-2πi k/n) for k in [0, n)
	roots   map[int][]complex128 // for each radix r in factors, exp(-2πi jk/r) at [j*r+k]
	scratch []complex128
	tmp     []complex128 // one radix's worth of values
}

func newFFTPlan(n int) *fftPlan {
	p := &fftPlan{n: n, twiddle: make([]complex128, n), roots: map[int][]complex128{}, scratch: make([]complex128, n)}
	for k := range p.twiddle {
		p.twiddle[k] = cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n)))
	}
	largest := 1
	for m, f := n, 2; m > 1; {
		if m%f == 0 {
			p.factors = append([]int{f}, p.factors...)
			m /= f
			largest = max(largest, f)
			if p.roots[f] == nil {
				table := make([]complex128, f*f)
				for j := 0; j < f; j++ {
					for k := 0; k < f; k++ {
						table[j*f+k] = p.twiddle[((j*k)%f)*(n/f)]
					}
				}
				p.roots[f] = table
			}
		} else {
			f++
		}
	}
	p.tmp = make([]complex128, largest)
	return p
}

// transform replaces data, of the plan's length, by its discrete Fourier
// transform, or by its inverse (scaled by 1/n) when inverse is set.
func (p *fftPlan) transform(data []complex128, inverse bool) {
	if p.n == 1 {
		return
	}
	if inverse {
		for i := range data {
			data[i] = cmplx.Conj(data[i])
		}
	}
	copy(p.scratch, data)
	p.recurse(p.scratch, 1, data, p.n, 0)
	if inverse {
		scale := complex(1/float64(p.n), 0)
		for i := range data {
			data[i] = cmplx.Conj(data[i]) * scale
		}
	}
}

// recurse is decimation in time: the n values at in[0], in[stride], ... are
// split into radix interleaved sequences, each transformed into its part of
// out, then combined with the twiddles.
func (p *fftPlan) recurse(in []complex128, stride int, out []complex128, n, level int) {
	radix := p.factors[level]
	m := n / radix
	step := p.n / n // twiddle index step for a transform of length n
	roots := p.roots[radix]
	if m == 1 {
		// The leaf: a plain radix-point transform.
		if radix == 2 {
			a, b := in[0], in[stride]
			out[0], out[1] = a+b, a-b
			return
		}
		for k := 0; k < radix; k++ {
			var sum complex128
			row := roots[k*radix : (k+1)*radix]
			for j, w := range row {
				sum += in[j*stride] * w
			}
			out[k] = sum
		}
		return
	}
	for r := 0; r < radix; r++ {
		p.recurse(in[r*stride:], stride*radix, out[r*m:(r+1)*m], m, level+1)
	}
	if radix == 2 {
		a, b := out[:m], out[m:2*m]
		for k := 0; k < m; k++ {
			x, y := a[k], b[k]*p.twiddle[k*step]
			a[k], b[k] = x+y, x-y
		}
		return
	}
	tmp := p.tmp[:radix]
	for k := 0; k < m; k++ {
		for r := 0; r < radix; r++ {
			tmp[r] = out[r*m+k] * p.twiddle[r*k*step] // r*k*step < n always
		}
		for q := 0; q < radix; q++ {
			var sum complex128
			row := roots[q*radix : (q+1)*radix]
			for r, w := range row {
				sum += tmp[r] * w
			}
			out[k+q*m] = sum
		}
	}
}

// fft2D transforms row-major w x h arrays.
type fft2D struct {
	w, h   int
	rows   *fftPlan
	cols   *fftPlan
	flip   []complex128 // the array transposed, for the column pass
	mirror []int        // the index of frequency -k for each k
}

var (
	fftPlansMu sync.Mutex
	fftPlans   = map[[2]int]*fft2D{}
)

// newFFT2D returns the transform for w x h arrays, shared by size. It is
// not safe for use from two goroutines at once.
func newFFT2D(w, h int) *fft2D {
	fftPlansMu.Lock()
	defer fftPlansMu.Unlock()
	if f, ok := fftPlans[[2]int{w, h}]; ok {
		return f
	}
	f := &fft2D{w: w, h: h, rows: newFFTPlan(w), cols: newFFTPlan(h), flip: make([]complex128, w*h), mirror: make([]int, w*h)}
	if w == h {
		f.cols = f.rows
	}
	for i := range f.mirror {
		x, y := i%w, i/w
		f.mirror[i] = ((h-y)%h)*w + (w-x)%w
	}
	fftPlans[[2]int{w, h}] = f
	return f
}

// transform replaces data (row-major, w*h) by its 2D transform, or by its
// inverse (scaled by 1/(w*h)) when inverse is set.
func (f *fft2D) transform(data []complex128, inverse bool) {
	w, h := f.w, f.h
	for y := 0; y < h; y++ {
		f.rows.transform(data[y*w:(y+1)*w], inverse)
	}
	// The columns, transformed as the rows of the transposed array.
	for y := 0; y < h; y++ {
		row := data[y*w : (y+1)*w]
		for x, v := range row {
			f.flip[x*h+y] = v
		}
	}
	for x := 0; x < w; x++ {
		f.cols.transform(f.flip[x*h:(x+1)*h], inverse)
	}
	for x := 0; x < w; x++ {
		col := f.flip[x*h : (x+1)*h]
		for y, v := range col {
			data[y*w+x] = v
		}
	}
}

// nextSmooth is the smallest size at or above n whose prime factors are all
// 2, 3 or 5, the sizes the transform is quick at.
func nextSmooth(n int) int {
	for m := max(n, 1); ; m++ {
		r := m
		for _, f := range []int{2, 3, 5} {
			for r%f == 0 {
				r /= f
			}
		}
		if r == 1 {
			return m
		}
	}
}
