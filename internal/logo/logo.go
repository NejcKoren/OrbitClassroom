// Package logo draws the Orbit Classroom logo procedurally (anti-aliased),
// so no binary image assets are needed. It is used for the .exe icons and
// the tray icon of the agent (with a coloured status badge).
package logo

import (
	"image"
	"image/color"
	"math"
)

// Badge colours for the tray icon.
var (
	BadgeNone   = color.RGBA{}
	BadgeLive   = color.RGBA{239, 68, 68, 255}  // being viewed / controlled
	BadgeHand   = color.RGBA{250, 204, 21, 255} // hand raised
	BadgeLocked = color.RGBA{148, 163, 184, 255}
)

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// Render draws the logo at size x size pixels (straight-alpha RGBA, not premultiplied).
func Render(size int, badge color.RGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling per axis
	s := float64(size)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var ar, ag, ab, aa float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) / s
					y := (float64(py) + (float64(sy)+0.5)/ss) / s
					r, g, b, a := sample(x, y, badge)
					ar += r * a
					ag += g * a
					ab += b * a
					aa += a
				}
			}
			n := float64(ss * ss)
			i := py*img.Stride + px*4
			if aa > 0 {
				img.Pix[i] = byte(math.Min(255, ar/aa))
				img.Pix[i+1] = byte(math.Min(255, ag/aa))
				img.Pix[i+2] = byte(math.Min(255, ab/aa))
			}
			img.Pix[i+3] = byte(math.Min(255, aa/n*255))
		}
	}
	return img
}

// sample returns the colour (0..255) and coverage (0..1) at normalised x,y.
func sample(x, y float64, badge color.RGBA) (r, g, b, a float64) {
	// rounded square background with diagonal gradient (indigo -> violet)
	const rad = 0.22
	dx := math.Max(math.Abs(x-0.5)-(0.5-rad), 0)
	dy := math.Max(math.Abs(y-0.5)-(0.5-rad), 0)
	if math.Hypot(dx, dy) > rad {
		return 0, 0, 0, 0
	}
	t := (x + y) / 2
	r = lerp(79, 147, t)
	g = lerp(110, 72, t)
	b = lerp(255, 240, t)
	a = 1

	// tilted orbit ring (ellipse outline), rotated -25 degrees
	cx, cy := x-0.5, y-0.52
	ang := -25 * math.Pi / 180
	rx := cx*math.Cos(ang) - cy*math.Sin(ang)
	ry := cx*math.Sin(ang) + cy*math.Cos(ang)
	e := math.Sqrt((rx*rx)/(0.36*0.36) + (ry*ry)/(0.14*0.14))
	ring := math.Abs(e-1) < 0.11
	// planet
	planet := math.Hypot(x-0.5, y-0.52) < 0.205
	// ring passes in front of the planet only on the lower half
	front := ry > 0
	if planet && !(ring && front) {
		r, g, b = 255, 255, 255
	}
	if ring && (front || !planet) {
		r, g, b = 255, 255, 255
		if !front {
			r, g, b = 226, 230, 255
		}
	}
	// small moon
	if math.Hypot(x-0.80, y-0.27) < 0.065 {
		r, g, b = 255, 214, 102
	}
	// status badge (bottom right)
	if badge.A > 0 {
		d := math.Hypot(x-0.78, y-0.78)
		if d < 0.215 {
			r, g, b = 255, 255, 255
		}
		if d < 0.165 {
			r, g, b = float64(badge.R), float64(badge.G), float64(badge.B)
		}
	}
	return
}
