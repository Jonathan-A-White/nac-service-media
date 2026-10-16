package detection

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

// naiveDFT is the definition, for checking the FFT.
func naiveDFT(in []complex128, inverse bool) []complex128 {
	n := len(in)
	out := make([]complex128, n)
	sign := -1.0
	if inverse {
		sign = 1
	}
	for k := 0; k < n; k++ {
		var sum complex128
		for j := 0; j < n; j++ {
			sum += in[j] * cmplx.Exp(complex(0, sign*2*math.Pi*float64(j*k)/float64(n)))
		}
		if inverse {
			sum /= complex(float64(n), 0)
		}
		out[k] = sum
	}
	return out
}

func randomComplex(n int, seed int64) []complex128 {
	r := rand.New(rand.NewSource(seed))
	out := make([]complex128, n)
	for i := range out {
		out[i] = complex(r.Float64()*2-1, r.Float64()*2-1)
	}
	return out
}

func maxDiff(a, b []complex128) float64 {
	d := 0.0
	for i := range a {
		d = math.Max(d, cmplx.Abs(a[i]-b[i]))
	}
	return d
}

func TestFFT_MatchesTheDefinitionForEverySmoothAndPrimeSize(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 15, 16, 18, 20, 25, 27, 30, 32, 45, 64, 100, 270, 432, 480} {
		in := randomComplex(n, int64(n))
		plan := newFFTPlan(n)
		got := append([]complex128(nil), in...)
		plan.transform(got, false)
		if d := maxDiff(got, naiveDFT(in, false)); d > 1e-9*float64(n) {
			t.Errorf("n=%d: forward FFT differs from the DFT by %g", n, d)
		}
		back := append([]complex128(nil), got...)
		plan.transform(back, true)
		if d := maxDiff(back, in); d > 1e-9*float64(n) {
			t.Errorf("n=%d: inverse FFT does not give the input back (differs by %g)", n, d)
		}
	}
}

func TestFFT2D_MatchesTheDefinitionAndRoundTrips(t *testing.T) {
	w, h := 6, 4
	in := randomComplex(w*h, 7)
	// The definition: transform every row, then every column.
	want := make([]complex128, w*h)
	for y := 0; y < h; y++ {
		copy(want[y*w:(y+1)*w], naiveDFT(in[y*w:(y+1)*w], false))
	}
	for x := 0; x < w; x++ {
		col := make([]complex128, h)
		for y := 0; y < h; y++ {
			col[y] = want[y*w+x]
		}
		col = naiveDFT(col, false)
		for y := 0; y < h; y++ {
			want[y*w+x] = col[y]
		}
	}
	got := append([]complex128(nil), in...)
	f := newFFT2D(w, h)
	f.transform(got, false)
	if d := maxDiff(got, want); d > 1e-9 {
		t.Errorf("2D FFT differs from the definition by %g", d)
	}
	f.transform(got, true)
	if d := maxDiff(got, in); d > 1e-9 {
		t.Errorf("2D inverse FFT does not give the input back (differs by %g)", d)
	}
}

func TestNextSmooth(t *testing.T) {
	for n, want := range map[int]int{1: 1, 2: 2, 7: 8, 427: 432, 480: 480, 270: 270, 241: 243, 500: 500, 101: 108} {
		if got := nextSmooth(n); got != want {
			t.Errorf("nextSmooth(%d) = %d, want %d", n, got, want)
		}
	}
}
