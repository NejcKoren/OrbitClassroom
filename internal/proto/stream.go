package proto

// Orbit screen stream ("OF" frames)
//
// A stream is a sequence of frames. Every frame contains a list of JPEG
// encoded rectangles. A key frame covers the whole screen; delta frames only
// contain the 64x64 tiles that changed since the previous frame, merged into
// larger rectangles. A static screen therefore costs (almost) no bandwidth.
//
//	frame  := "O" "F" flags:u8 reserved:u8 width:u16 height:u16 count:u16 rect*count
//	rect   := x:u16 y:u16 w:u16 h:u16 length:u32 jpeg[length]
//
// All integers are big endian. flags bit 0 = key frame. A frame with zero
// rectangles is a keep-alive.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"runtime"
	"sync"
	"time"
)

const (
	TileSize     = 64
	FlagKey      = 1
	frameHdrLen  = 10
	rectHdrLen   = 12
	maxRectBytes = 32 << 20
)

type rect struct{ x, y, w, h int }

// Encoder turns successive screen captures into stream frames.
type Encoder struct {
	Quality int
	prev    []byte
	w, h    int
	lastOut time.Time
	changed []bool
}

// Reset forces the next frame to be a key frame.
func (e *Encoder) Reset() { e.prev = nil }

// Encode encodes img (which must start at 0,0). It returns nil when nothing
// changed and no keep-alive is due yet.
func (e *Encoder) Encode(img *image.RGBA, forceKey bool) (data []byte, key bool, err error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 || w > 65535 || h > 65535 {
		return nil, false, errors.New("invalid frame size")
	}
	key = forceKey || e.prev == nil || e.w != w || e.h != h
	var rects []rect
	if !key {
		rects = e.diff(img)
		if len(rects) == 0 && time.Since(e.lastOut) < time.Second {
			return nil, false, nil
		}
		area := 0
		for _, r := range rects {
			area += r.w * r.h
		}
		if area*10 > w*h*6 { // more than 60% changed: a key frame is cheaper
			key = true
		}
	}
	if key {
		rects = bands(w, h)
	}
	q := e.Quality
	if q <= 0 || q > 100 {
		q = 70
	}
	blobs, err := encodeRects(img, rects, q)
	if err != nil {
		return nil, false, err
	}
	size := frameHdrLen
	for _, b := range blobs {
		size += rectHdrLen + len(b)
	}
	out := make([]byte, 0, size)
	var flags byte
	if key {
		flags |= FlagKey
	}
	out = append(out, 'O', 'F', flags, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(w))
	out = binary.BigEndian.AppendUint16(out, uint16(h))
	out = binary.BigEndian.AppendUint16(out, uint16(len(rects)))
	for i, r := range rects {
		out = binary.BigEndian.AppendUint16(out, uint16(r.x))
		out = binary.BigEndian.AppendUint16(out, uint16(r.y))
		out = binary.BigEndian.AppendUint16(out, uint16(r.w))
		out = binary.BigEndian.AppendUint16(out, uint16(r.h))
		out = binary.BigEndian.AppendUint32(out, uint32(len(blobs[i])))
		out = append(out, blobs[i]...)
	}
	// remember this frame for the next diff
	if len(e.prev) != w*h*4 {
		e.prev = make([]byte, w*h*4)
	}
	for y := 0; y < h; y++ {
		copy(e.prev[y*w*4:(y+1)*w*4], img.Pix[y*img.Stride:y*img.Stride+w*4])
	}
	e.w, e.h = w, h
	e.lastOut = time.Now()
	return out, key, nil
}

func (e *Encoder) tileChanged(img *image.RGBA, tx, y0, y1 int) bool {
	x0 := tx * TileSize
	x1 := min(x0+TileSize, e.w)
	for y := y0; y < y1; y++ {
		a := img.Pix[y*img.Stride+x0*4 : y*img.Stride+x1*4]
		b := e.prev[y*e.w*4+x0*4 : y*e.w*4+x1*4]
		if !bytes.Equal(a, b) {
			return true
		}
	}
	return false
}

// diff returns the changed regions: horizontal runs of changed tiles, merged
// vertically when consecutive rows have identical runs.
func (e *Encoder) diff(img *image.RGBA) []rect {
	tx := (e.w + TileSize - 1) / TileSize
	ty := (e.h + TileSize - 1) / TileSize
	if len(e.changed) < tx {
		e.changed = make([]bool, tx)
	}
	var out []rect
	prevRow := map[[2]int]int{}
	for j := 0; j < ty; j++ {
		y0 := j * TileSize
		y1 := min(y0+TileSize, e.h)
		for i := 0; i < tx; i++ {
			e.changed[i] = e.tileChanged(img, i, y0, y1)
		}
		curRow := map[[2]int]int{}
		for i := 0; i < tx; {
			if !e.changed[i] {
				i++
				continue
			}
			start := i
			for i < tx && e.changed[i] {
				i++
			}
			x0, x1 := start*TileSize, min(i*TileSize, e.w)
			k := [2]int{x0, x1}
			if idx, ok := prevRow[k]; ok {
				out[idx].h = y1 - out[idx].y
				curRow[k] = idx
			} else {
				out = append(out, rect{x0, y0, x1 - x0, y1 - y0})
				curRow[k] = len(out) - 1
			}
		}
		prevRow = curRow
	}
	return out
}

// bands splits a key frame into horizontal bands so they can be encoded in parallel.
func bands(w, h int) []rect {
	n := min(4, runtime.NumCPU())
	if h < 256 || n < 1 {
		n = 1
	}
	bh := ((h/n + 15) / 16) * 16
	var out []rect
	for y := 0; y < h; y += bh {
		out = append(out, rect{0, y, w, min(bh, h-y)})
	}
	return out
}

func encodeRects(img *image.RGBA, rects []rect, q int) ([][]byte, error) {
	blobs := make([][]byte, len(rects))
	errs := make([]error, len(rects))
	sem := make(chan struct{}, runtime.NumCPU())
	var wg sync.WaitGroup
	for i, r := range rects {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r rect) {
			defer func() { <-sem; wg.Done() }()
			var b bytes.Buffer
			sub := img.SubImage(image.Rect(r.x, r.y, r.x+r.w, r.y+r.h))
			errs[i] = jpeg.Encode(&b, sub, &jpeg.Options{Quality: q})
			blobs[i] = b.Bytes()
		}(i, r)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return blobs, nil
}

// FrameRect is one decoded rectangle header plus its JPEG data.
type FrameRect struct {
	X, Y, W, H int
	Data       []byte
}

// Frame is a parsed stream frame.
type Frame struct {
	Key   bool
	W, H  int
	Rects []FrameRect
	Raw   []byte // the complete frame as received (for relaying)
}

// ReadFrame reads exactly one frame from r.
func ReadFrame(r io.Reader) (*Frame, error) {
	hdr := make([]byte, frameHdrLen)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	if hdr[0] != 'O' || hdr[1] != 'F' {
		return nil, errors.New("bad stream magic")
	}
	f := &Frame{
		Key: hdr[2]&FlagKey != 0,
		W:   int(binary.BigEndian.Uint16(hdr[4:])),
		H:   int(binary.BigEndian.Uint16(hdr[6:])),
	}
	n := int(binary.BigEndian.Uint16(hdr[8:]))
	raw := append(make([]byte, 0, 64<<10), hdr...)
	type pos struct{ x, y, w, h, off, l int }
	ps := make([]pos, 0, n)
	rh := make([]byte, rectHdrLen)
	for i := 0; i < n; i++ {
		if _, err := io.ReadFull(r, rh); err != nil {
			return nil, err
		}
		l := int(binary.BigEndian.Uint32(rh[8:]))
		if l > maxRectBytes {
			return nil, errors.New("rectangle too large")
		}
		raw = append(raw, rh...)
		off := len(raw)
		raw = append(raw, make([]byte, l)...)
		if _, err := io.ReadFull(r, raw[off:off+l]); err != nil {
			return nil, err
		}
		ps = append(ps, pos{
			int(binary.BigEndian.Uint16(rh[0:])), int(binary.BigEndian.Uint16(rh[2:])),
			int(binary.BigEndian.Uint16(rh[4:])), int(binary.BigEndian.Uint16(rh[6:])), off, l,
		})
	}
	f.Raw = raw
	for _, p := range ps {
		f.Rects = append(f.Rects, FrameRect{p.x, p.y, p.w, p.h, raw[p.off : p.off+p.l]})
	}
	return f, nil
}

// Canvas is a BGRA frame buffer that frames are painted onto (used by the
// agent to display the teacher's screen). BGRA is what Windows GDI expects.
type Canvas struct {
	mu    sync.Mutex
	W, H  int
	Pix   []byte
	Ready bool
	Seq   uint64
}

// ErrNeedKey is returned when a delta frame arrives before a key frame.
var ErrNeedKey = errors.New("waiting for key frame")

// Apply decodes the frame and paints it onto the canvas.
func (c *Canvas) Apply(f *Frame) error {
	if len(f.Rects) == 0 {
		return nil
	}
	imgs := make([]*image.RGBA, len(f.Rects))
	errs := make([]error, len(f.Rects))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, r := range f.Rects {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r FrameRect) {
			defer func() { <-sem; wg.Done() }()
			src, err := jpeg.Decode(bytes.NewReader(r.Data))
			if err != nil {
				errs[i] = err
				return
			}
			dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
			draw.Draw(dst, dst.Rect, src, src.Bounds().Min, draw.Src)
			imgs[i] = dst
		}(i, r)
	}
	wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if f.W != c.W || f.H != c.H || !c.Ready {
		if !f.Key {
			return ErrNeedKey
		}
		c.W, c.H = f.W, f.H
		c.Pix = make([]byte, f.W*f.H*4)
		c.Ready = true
	}
	for i, r := range f.Rects {
		img := imgs[i]
		if errs[i] != nil || img == nil {
			continue
		}
		w := min(img.Rect.Dx(), c.W-r.X)
		h := min(img.Rect.Dy(), c.H-r.Y)
		for y := 0; y < h; y++ {
			s := img.Pix[y*img.Stride : y*img.Stride+w*4]
			d := c.Pix[((r.Y+y)*c.W+r.X)*4:]
			for x := 0; x < w*4; x += 4 {
				d[x], d[x+1], d[x+2], d[x+3] = s[x+2], s[x+1], s[x], 255
			}
		}
	}
	c.Seq++
	return nil
}

// Lock gives exclusive access to Pix (for painting).
func (c *Canvas) Lock()   { c.mu.Lock() }
func (c *Canvas) Unlock() { c.mu.Unlock() }

// RGBA returns a copy of the canvas as an RGBA image (debugging/tests).
func (c *Canvas) RGBA() *image.RGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	img := image.NewRGBA(image.Rect(0, 0, c.W, c.H))
	for i := 0; i+3 < len(c.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.Pix[i+2], c.Pix[i+1], c.Pix[i], 255
	}
	return img
}
