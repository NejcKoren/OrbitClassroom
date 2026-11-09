// Package screen captures the local screen. On Windows it uses GDI
// (BitBlt/StretchBlt into a DIB section); on other systems it renders an
// animated test picture so the whole system can be developed and tested
// without Windows.
package screen

import (
	"errors"
	"image"
	"orbit/internal/proto"
)

// ErrUnavailable is returned while the desktop cannot be captured
// (e.g. a UAC prompt or the Windows lock screen is shown).
var ErrUnavailable = errors.New("screen not available")

// Area returns the capture rectangle for a monitor index (-1 = all monitors).
func Area(mon int) image.Rectangle {
	ms := Monitors()
	if mon >= 0 && mon < len(ms) {
		m := ms[mon]
		return image.Rect(m.X, m.Y, m.X+m.W, m.Y+m.H)
	}
	return VirtualScreen()
}

// PrimaryArea returns the rectangle of the primary monitor.
func PrimaryArea() image.Rectangle {
	for _, m := range Monitors() {
		if m.Primary {
			return image.Rect(m.X, m.Y, m.X+m.W, m.Y+m.H)
		}
	}
	return VirtualScreen()
}

// FitSize scales w,h down to fit maxW (keeping aspect ratio, even numbers).
func FitSize(w, h, maxW int) (int, int) {
	if maxW <= 0 || w <= maxW {
		return w, h
	}
	nh := h * maxW / w
	return maxW &^ 1, max(2, nh&^1)
}

// MonitorsProto converts monitors for the wire format.
func MonitorsProto() []proto.Monitor { return Monitors() }

// bgraToRGBA converts a BGRA pixel buffer to RGBA with opaque alpha.
func bgraToRGBA(dst, src []byte) {
	n := min(len(dst), len(src))
	for i := 0; i+3 < n; i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = src[i+2], src[i+1], src[i], 255
	}
}

func ensure(dst *image.RGBA, w, h int) *image.RGBA {
	if dst == nil || dst.Rect.Dx() != w || dst.Rect.Dy() != h || dst.Rect.Min != (image.Point{}) {
		return image.NewRGBA(image.Rect(0, 0, w, h))
	}
	return dst
}
