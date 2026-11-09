//go:build !windows

package screen

import (
	"image"
	"image/color"
	"math"
	"orbit/internal/proto"
	"os"
	"strconv"
	"sync"
	"time"
)

// Development/test implementation: renders an animated "desktop".

var (
	testMu   sync.Mutex
	testFull *image.RGBA
	seed     = time.Now().UnixNano()
)

func Init() {}

func testSize() (int, int) {
	w, h := 1600, 900
	if v, err := strconv.Atoi(os.Getenv("ORBIT_TEST_W")); err == nil && v > 0 {
		w = v
	}
	if v, err := strconv.Atoi(os.Getenv("ORBIT_TEST_H")); err == nil && v > 0 {
		h = v
	}
	return w, h
}

func VirtualScreen() image.Rectangle {
	w, h := testSize()
	return image.Rect(0, 0, w, h)
}

func Monitors() []proto.Monitor {
	w, h := testSize()
	return []proto.Monitor{{X: 0, Y: 0, W: w, H: h, Primary: true}}
}

func render() *image.RGBA {
	w, h := testSize()
	if testFull == nil || testFull.Rect.Dx() != w || testFull.Rect.Dy() != h {
		testFull = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	img := testFull
	hue := float64(seed%360) / 360
	t := float64(time.Now().UnixMilli()) / 1000
	// wallpaper gradient
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			r := 40 + 60*fx + 40*math.Sin(hue*6.28)
			g := 50 + 50*fy + 40*math.Cos(hue*6.28)
			b := 110 + 80*(1-fy)
			i := y*img.Stride + x*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = byte(r), byte(g), byte(b), 255
		}
	}
	// taskbar
	fill(img, 0, h-48, w, 48, color.RGBA{24, 26, 34, 255})
	for i := 0; i < 6; i++ {
		fill(img, 16+i*56, h-40, 40, 32, color.RGBA{byte(70 + i*25), 120, 200, 255})
	}
	// "window"
	wx, wy := w/8, h/8
	fill(img, wx, wy, w/2, h/2, color.RGBA{245, 246, 250, 255})
	fill(img, wx, wy, w/2, 36, color.RGBA{60, 90, 200, 255})
	for l := 0; l < 9; l++ {
		fill(img, wx+24, wy+60+l*34, (w/2-48)*(5+(l*7)%5)/10, 14, color.RGBA{180, 186, 200, 255})
	}
	// moving element (so deltas are visible)
	bx := int(float64(w/2) + float64(w/4)*math.Sin(t))
	by := int(float64(h/2) + float64(h/5)*math.Cos(t*0.7))
	fill(img, bx, by, 90, 90, color.RGBA{255, 170, 40, 255})
	// clock bar
	sec := time.Now().Second()
	fill(img, w-220, h-36, 200*sec/60+4, 20, color.RGBA{120, 220, 140, 255})
	return img
}

func fill(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(img.Rect)
	for yy := r.Min.Y; yy < r.Max.Y; yy++ {
		for xx := r.Min.X; xx < r.Max.X; xx++ {
			i := yy*img.Stride + xx*4
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
}

func Capture(dst *image.RGBA, area image.Rectangle, outW, outH int, cursor bool) (*image.RGBA, error) {
	testMu.Lock()
	defer testMu.Unlock()
	src := render()
	area = area.Intersect(src.Rect)
	if outW <= 0 || outH <= 0 {
		outW, outH = area.Dx(), area.Dy()
	}
	dst = ensure(dst, outW, outH)
	for y := 0; y < outH; y++ {
		sy := area.Min.Y + y*area.Dy()/outH
		for x := 0; x < outW; x++ {
			sx := area.Min.X + x*area.Dx()/outW
			si := sy*src.Stride + sx*4
			di := y*dst.Stride + x*4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst, nil
}
