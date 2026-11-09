//go:build windows

package screen

import (
	"image"
	"orbit/internal/proto"
	"orbit/internal/win"
	"sync"
)

var (
	mu         sync.Mutex
	memDC      uintptr
	dib        uintptr
	dibBits    []byte
	dibW, dibH int
)

// Init enables per-monitor DPI awareness so we capture real pixels.
func Init() { win.EnableDPIAwareness() }

// VirtualScreen returns the bounding box of all monitors.
func VirtualScreen() image.Rectangle {
	x := win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)
	y := win.GetSystemMetrics(win.SM_YVIRTUALSCREEN)
	w := win.GetSystemMetrics(win.SM_CXVIRTUALSCREEN)
	h := win.GetSystemMetrics(win.SM_CYVIRTUALSCREEN)
	if w <= 0 || h <= 0 {
		w, h = win.GetSystemMetrics(win.SM_CXSCREEN), win.GetSystemMetrics(win.SM_CYSCREEN)
	}
	return image.Rect(x, y, x+w, y+h)
}

// Monitors lists all displays.
func Monitors() []proto.Monitor {
	var out []proto.Monitor
	for _, m := range win.Monitors() {
		out = append(out, proto.Monitor{X: int(m.R.Left), Y: int(m.R.Top), W: int(m.R.Right - m.R.Left), H: int(m.R.Bottom - m.R.Top), Primary: m.Primary})
	}
	if len(out) == 0 {
		v := VirtualScreen()
		out = append(out, proto.Monitor{X: v.Min.X, Y: v.Min.Y, W: v.Dx(), H: v.Dy(), Primary: true})
	}
	return out
}

// Capture grabs area (virtual desktop coordinates), scaled to outW x outH
// (0 = native size). dst is reused when it has the right size.
func Capture(dst *image.RGBA, area image.Rectangle, outW, outH int, cursor bool) (*image.RGBA, error) {
	mu.Lock()
	defer mu.Unlock()
	if outW <= 0 || outH <= 0 {
		outW, outH = area.Dx(), area.Dy()
	}
	screenDC := win.GetDC(0)
	if screenDC == 0 {
		return nil, ErrUnavailable
	}
	defer win.ReleaseDC(0, screenDC)
	if memDC == 0 {
		memDC = win.CreateCompatibleDC(screenDC)
	}
	if dib == 0 || dibW != outW || dibH != outH {
		if dib != 0 {
			win.DeleteObject(dib)
		}
		dib, dibBits = win.CreateDIB(screenDC, outW, outH)
		if dib == 0 {
			return nil, ErrUnavailable
		}
		dibW, dibH = outW, outH
		win.SelectObject(memDC, dib)
	}
	var ok bool
	if outW == area.Dx() && outH == area.Dy() {
		ok = win.BitBlt(memDC, 0, 0, outW, outH, screenDC, area.Min.X, area.Min.Y)
	} else {
		win.SetStretchBltMode(memDC, win.HALFTONE)
		ok = win.StretchBlt(memDC, 0, 0, outW, outH, screenDC, area.Min.X, area.Min.Y, area.Dx(), area.Dy())
	}
	if !ok {
		return nil, ErrUnavailable
	}
	if cursor {
		drawCursor(area, outW, outH)
	}
	win.GdiFlush()
	dst = ensure(dst, outW, outH)
	bgraToRGBA(dst.Pix, dibBits)
	return dst, nil
}

func drawCursor(area image.Rectangle, outW, outH int) {
	ci, ok := win.GetCursorInfo()
	if !ok || ci.Flags&win.CURSOR_SHOWING == 0 || ci.HCursor == 0 {
		return
	}
	hx, hy := win.CursorHotspot(ci.HCursor)
	cw := win.GetSystemMetrics(win.SM_CXCURSOR)
	ch := win.GetSystemMetrics(win.SM_CYCURSOR)
	sx := float64(outW) / float64(area.Dx())
	sy := float64(outH) / float64(area.Dy())
	x := int(float64(int(ci.Pt.X)-hx-area.Min.X) * sx)
	y := int(float64(int(ci.Pt.Y)-hy-area.Min.Y) * sy)
	win.DrawIcon(memDC, x, y, ci.HCursor, max(8, int(float64(cw)*sx)), max(8, int(float64(ch)*sy)))
}
