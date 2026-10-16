package detection

import (
	"image"
	"image/color"
	"math"
)

// roomSpec describes a synthetic view of the room for the detector's tests:
// the picture's size, where the cross is and how tall its bar is, whether it
// is lit, the exposure (gain 1 is the exposure the templates were taken at)
// and a seed for the noise.
type roomSpec struct {
	width, height int
	centre        image.Point // the centre of the cross's bar
	crossHeight   int         // the bar's height; every other size follows it
	lit           bool
	gain          float64
	seed          uint32
}

// crossBox is the box of the cross's bar and arm (not the rays) in a room
// painted from spec, before padding.
func (s roomSpec) crossBox() image.Rectangle {
	h := s.crossHeight
	top := s.centre.Y - h/2
	return image.Rect(s.centre.X-h/4, top, s.centre.X+h/4, top+h)
}

// paintRoom paints the room: a wall with a soft gradient, the altar below
// the cross, the pews in the lower half, and the cross: a vertical bar, a
// cross-arm and seven rays. Lit, the bar is bright with a glow around it;
// unlit, it is a little lighter than the wall, like the gold cross on the
// grey wall. Everything scales with crossHeight, so the same room can be
// painted as a wider or a closer shot.
func paintRoom(s roomSpec) *image.Gray {
	gain := s.gain
	if gain == 0 {
		gain = 1
	}
	img := image.NewGray(image.Rect(0, 0, s.width, s.height))
	h := float64(s.crossHeight)
	bar := s.crossBox()
	barW := math.Max(2, h/6)
	bar.Min.X, bar.Max.X = int(float64(s.centre.X)-barW/2), int(float64(s.centre.X)+barW/2)
	arm := image.Rect(s.centre.X-int(h/4), bar.Min.Y+int(h/5), s.centre.X+int(h/4), bar.Min.Y+int(h/5)+int(math.Max(2, h/8)))
	altar := image.Rect(s.centre.X-int(h*0.8), bar.Max.Y+int(h*0.3), s.centre.X+int(h*0.8), bar.Max.Y+int(h*0.7))
	rayOrigin := image.Point{X: s.centre.X, Y: bar.Max.Y}
	rayThick := math.Max(1, h/25)
	glowRadius := h / 3

	state := s.seed*2654435761 + 1
	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			v := 125 + 15*float64(y)/float64(s.height) // the wall
			if y > s.height*55/100 && ((y-s.height*55/100)/max(1, s.height/18))%2 == 0 {
				v = 60 + 15*float64((x/max(1, s.width/16))%2) // the pews
			}
			p := image.Pt(x, y)
			if p.In(altar) {
				v = 160
			}
			if onRay(p, rayOrigin, h, rayThick) {
				v = 165
			}
			if p.In(bar) || p.In(arm) {
				if s.lit {
					v = 245
				} else {
					v = 160
				}
			} else if s.lit {
				d := distanceToRect(p, bar)
				if d < glowRadius {
					g := 1 - d/glowRadius
					v += 90 * g * g
				}
			}
			state = state*1664525 + 1013904223
			v += float64(int(state>>24)%9 - 4)
			v *= gain
			img.SetGray(x, y, color.Gray{Y: uint8(math.Max(0, math.Min(255, math.Round(v))))})
		}
	}
	return img
}

// onRay says whether p lies on one of the seven rays fanning up from origin,
// each from 0.35h to 0.9h away from it.
func onRay(p, origin image.Point, h, thick float64) bool {
	dx, dy := float64(p.X-origin.X), float64(p.Y-origin.Y)
	r := math.Hypot(dx, dy)
	if r < 0.35*h || r > 0.9*h || dy > 0 {
		return false
	}
	for k := 0; k < 7; k++ {
		angle := math.Pi + math.Pi/8*float64(k+1) // from the left, up over the bar, to the right
		ux, uy := math.Cos(angle), math.Sin(angle)
		// Distance from p to the ray's line, and the ray must run through it.
		along := dx*ux + dy*uy
		across := math.Abs(dx*uy - dy*ux)
		if along > 0 && across <= thick/2+0.5 {
			return true
		}
	}
	return false
}

func distanceToRect(p image.Point, r image.Rectangle) float64 {
	dx := math.Max(float64(r.Min.X-p.X), math.Max(0, float64(p.X-r.Max.X+1)))
	dy := math.Max(float64(r.Min.Y-p.Y), math.Max(0, float64(p.Y-r.Max.Y+1)))
	return math.Hypot(dx, dy)
}

// learnShot does what 'templates add' does: it finds the cross's box from the
// unlit and lit pictures and crops both to it.
func learnShot(unlit, lit *image.Gray) (unlitTmpl, litTmpl *image.Gray, box image.Rectangle, err error) {
	box, err = FindCrossBox(unlit, lit)
	if err != nil {
		return nil, nil, box, err
	}
	return toGray(cropImage(unlit, box)), toGray(cropImage(lit, box)), box, nil
}
