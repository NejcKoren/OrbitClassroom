//go:build windows

package sys

import (
	"image/color"
	"log"
	"math"
	"orbit/internal/logo"
	"orbit/internal/proto"
	"orbit/internal/win"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// All windows live on one OS thread with a classic Win32 message loop.
// Other goroutines talk to it through invoke(), which queues a closure and
// wakes the loop with a posted message.

const (
	wmInvoke = win.WM_APP + 1
	wmTray   = win.WM_APP + 2
)

type wndHandler interface {
	proc(hwnd uintptr, msg uint32, wp, lp uintptr) (uintptr, bool)
}

var (
	uiCalls        = make(chan func(), 1024)
	hostHwnd       uintptr
	uiHandler      UIHandler
	windows        = map[uintptr]wndHandler{}
	uiScale        = 1.0
	taskbarCreated uint32
	hookHandle     uintptr
	wndProcCB      uintptr
	hookCB         uintptr
	cursorArrow    uintptr
	cursorHand     uintptr
	appIcon        uintptr
	glyphFont      = "Segoe MDL2 Assets"
	hasGlyphFont   bool

	lockOv   *overlay
	demoOv   *overlay
	demoWin  *demoWindow
	demoHwnd atomic.Uintptr

	trayNID   win.NOTIFYICONDATAW
	trayOn    bool
	trayState TrayState
	trayIcons = map[color.RGBA]uintptr{}
)

const accent = 0xFF6E4F // RGB(79,110,255) as COLORREF (0x00BBGGRR)

// StartUI starts the UI thread. Must be called once before any other UI call.
func StartUI(h UIHandler) error {
	uiHandler = h
	if hostHwnd != 0 {
		return nil // already running: only the handler changes
	}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		uiScale = win.DPIScale()
		wndProcCB = syscall.NewCallback(wndProc)
		hookCB = syscall.NewCallback(kbHook)
		cursorArrow = win.LoadCursor(win.IDC_ARROW)
		cursorHand = win.LoadCursor(win.IDC_HAND)
		appIcon = makeIcon(32, logo.BadgeNone)
		hasGlyphFont = win.FontAvailable(glyphFont)
		if !hasGlyphFont && win.FontAvailable("Segoe Fluent Icons") {
			glyphFont, hasGlyphFont = "Segoe Fluent Icons", true
		}
		for _, c := range []struct {
			name  string
			style uint32
		}{{"OrbitHost", 0}, {"OrbitOverlay", win.CS_HREDRAW | win.CS_VREDRAW}, {"OrbitCard", win.CS_DROPSHADOW}, {"OrbitDemo", win.CS_HREDRAW | win.CS_VREDRAW}} {
			if err := win.RegisterClass(c.name, wndProcCB, c.style, cursorArrow, appIcon); err != nil {
				ready <- err
				return
			}
		}
		hostHwnd = win.CreateWindow(0, "OrbitHost", "Orbit Agent", win.WS_POPUP, 0, 0, 0, 0, 0)
		windows[hostHwnd] = hostWnd{}
		taskbarCreated = win.RegisterWindowMessage("TaskbarCreated")
		ready <- nil
		var m win.MSG
		for win.GetMessage(&m) > 0 {
			win.TranslateMessage(&m)
			win.DispatchMessage(&m)
		}
	}()
	return <-ready
}

func invoke(f func()) {
	if hostHwnd == 0 {
		return
	}
	select {
	case uiCalls <- f:
	default:
		go func() { uiCalls <- f }()
	}
	win.PostMessage(hostHwnd, wmInvoke, 0, 0)
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	if h, ok := windows[hwnd]; ok {
		if r, done := h.proc(hwnd, uint32(msg), wp, lp); done {
			return r
		}
	}
	return win.DefWindowProc(hwnd, uint32(msg), wp, lp)
}

func px(v float64) int { return int(math.Round(v * uiScale)) }

func makeIcon(size int, badge color.RGBA) uintptr {
	img := logo.Render(size, badge)
	return win.CreateIconRGBA(size, size, img.Pix)
}

// ------------------------------------------------------------ host window

type hostWnd struct{}

func (hostWnd) proc(hwnd uintptr, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case wmInvoke:
		for {
			select {
			case f := <-uiCalls:
				f()
			default:
				return 0, true
			}
		}
	case wmTray:
		switch uint32(lp & 0xFFFF) {
		case win.WM_RBUTTONUP, win.WM_LBUTTONUP, win.WM_CONTEXTMENU:
			showTrayMenu()
		}
		return 0, true
	case win.WM_HOTKEY:
		id := int(wp)
		go uiHandler.Hotkey(id)
		return 0, true
	}
	if taskbarCreated != 0 && msg == taskbarCreated && trayOn {
		trayOn = false
		addTray()
	}
	return 0, false
}

// ------------------------------------------------------------ tray

func setU16(dst []uint16, s string) {
	u := utf16.Encode([]rune(s))
	if len(u) > len(dst)-1 {
		u = u[:len(dst)-1]
	}
	copy(dst, u)
	dst[len(u)] = 0
}

func trayIcon() uintptr {
	b := logo.BadgeNone
	switch {
	case trayState.Live:
		b = logo.BadgeLive
	case trayState.Hand:
		b = logo.BadgeHand
	case trayState.Locked:
		b = logo.BadgeLocked
	}
	if ic, ok := trayIcons[b]; ok {
		return ic
	}
	size := win.GetSystemMetrics(win.SM_CXSMICON)
	if size <= 0 {
		size = 16
	}
	ic := makeIcon(size, b)
	trayIcons[b] = ic
	return ic
}

func addTray() {
	trayNID = win.NOTIFYICONDATAW{}
	trayNID.CbSize = uint32(unsafe.Sizeof(trayNID))
	trayNID.HWnd = hostHwnd
	trayNID.UID = 1
	trayNID.UFlags = win.NIF_MESSAGE | win.NIF_ICON | win.NIF_TIP
	trayNID.UCallbackMessage = wmTray
	trayNID.HIcon = trayIcon()
	setU16(trayNID.SzTip[:], tipText())
	trayOn = win.ShellNotifyIcon(win.NIM_ADD, &trayNID)
}

func tipText() string {
	if trayState.Tooltip != "" {
		return trayState.Tooltip
	}
	return "Orbit Agent"
}

// EnableTray shows the tray icon.
func EnableTray() { invoke(addTray) }

// RemoveTray removes the tray icon (on exit).
func RemoveTray() {
	done := make(chan struct{})
	invoke(func() {
		if trayOn {
			win.ShellNotifyIcon(win.NIM_DELETE, &trayNID)
			trayOn = false
		}
		close(done)
	})
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// SetTray updates badge and tooltip.
func SetTray(s TrayState) {
	invoke(func() {
		trayState = s
		if !trayOn {
			return
		}
		trayNID.UFlags = win.NIF_ICON | win.NIF_TIP
		trayNID.HIcon = trayIcon()
		setU16(trayNID.SzTip[:], tipText())
		win.ShellNotifyIcon(win.NIM_MODIFY, &trayNID)
	})
}

// Notify shows a Windows notification (balloon/toast) from the tray icon.
func Notify(title, text string) {
	invoke(func() {
		if !trayOn {
			return
		}
		n := trayNID
		n.UFlags = win.NIF_INFO
		setU16(n.SzInfoTitle[:], title)
		setU16(n.SzInfo[:], text)
		n.DwInfoFlags = win.NIIF_INFO
		win.ShellNotifyIcon(win.NIM_MODIFY, &n)
	})
}

func showTrayMenu() {
	items := uiHandler.TrayMenu()
	m := win.CreatePopupMenu()
	for i, it := range items {
		if it.Separator {
			win.AppendMenu(m, win.MF_SEPARATOR, 0, "")
			continue
		}
		flags := uint32(win.MF_STRING)
		if it.Checked {
			flags |= win.MF_CHECKED
		}
		if it.Disabled {
			flags |= win.MF_GRAYED
		}
		win.AppendMenu(m, flags, uintptr(i+1), it.Label)
	}
	pt := win.GetCursorPos()
	win.SetForegroundWindow(hostHwnd)
	cmd := win.TrackPopupMenu(m, int(pt.X), int(pt.Y), hostHwnd)
	win.PostMessage(hostHwnd, win.WM_NULL, 0, 0)
	win.DestroyMenu(m)
	if cmd > 0 && cmd <= len(items) {
		id := items[cmd-1].ID
		go uiHandler.TrayCommand(id)
	}
}

// RegisterHandHotkey registers Ctrl+Alt+H (hotkey id 1).
func RegisterHandHotkey() {
	invoke(func() {
		if !win.RegisterHotKey(hostHwnd, 1, win.MOD_CONTROL|win.MOD_ALT|win.MOD_NOREPEAT, 'H') {
			log.Printf("could not register Ctrl+Alt+H (used by another program?) - the tray menu still works")
		}
	})
}

// Beep plays the Windows notification sound.
func Beep() { win.MessageBeep(0x40) }

// ------------------------------------------------------------ keyboard block

func updateHook() {
	need := lockOv != nil || demoOv != nil
	if need && hookHandle == 0 {
		hookHandle = win.SetWindowsHookEx(win.WH_KEYBOARD_LL, hookCB)
	} else if !need && hookHandle != 0 {
		win.UnhookWindowsHookEx(hookHandle)
		hookHandle = 0
	}
}

func kbHook(code, wp, lp uintptr) uintptr {
	if int32(code) >= 0 && hookHandle != 0 && lp != 0 {
		kb := (*win.KBDLLHOOKSTRUCT)(unsafe.Pointer(lp))
		if kb.Flags&win.LLKHF_INJECTED == 0 { // let teacher remote-control input through
			return 1
		}
	}
	return win.CallNextHookEx(int(int32(code)), wp, lp)
}

// ------------------------------------------------------------ overlay (lock / fullscreen demo)

type overlay struct {
	hwnd        uintptr
	kind        string // lock | input | demo
	title, text string
	canvas      *proto.Canvas
	bg          []byte
	bgW, bgH    int
	minute      int
}

func virtualRect() win.RECT {
	x := win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)
	y := win.GetSystemMetrics(win.SM_YVIRTUALSCREEN)
	w := win.GetSystemMetrics(win.SM_CXVIRTUALSCREEN)
	h := win.GetSystemMetrics(win.SM_CYVIRTUALSCREEN)
	return win.RECT{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}
}

// primaryInClient returns the primary monitor rectangle in overlay client coordinates.
func primaryInClient() win.RECT {
	v := virtualRect()
	for _, m := range win.Monitors() {
		if m.Primary {
			return win.RECT{Left: m.R.Left - v.Left, Top: m.R.Top - v.Top, Right: m.R.Right - v.Left, Bottom: m.R.Bottom - v.Top}
		}
	}
	return win.RECT{Right: v.Right - v.Left, Bottom: v.Bottom - v.Top}
}

func newOverlay(kind string) *overlay {
	o := &overlay{kind: kind, minute: -1}
	v := virtualRect()
	ex := uint32(win.WS_EX_TOPMOST | win.WS_EX_TOOLWINDOW)
	if kind == "input" {
		ex |= win.WS_EX_LAYERED
	}
	o.hwnd = win.CreateWindow(ex, "OrbitOverlay", "Orbit", win.WS_POPUP, int(v.Left), int(v.Top), int(v.Right-v.Left), int(v.Bottom-v.Top), 0)
	windows[o.hwnd] = o
	if kind == "input" {
		win.SetLayeredAlpha(o.hwnd, 1)
	}
	win.ShowWindow(o.hwnd, win.SW_SHOW)
	win.SetForegroundWindow(o.hwnd)
	win.SetTimer(o.hwnd, 1, 400)
	return o
}

func (o *overlay) destroy() {
	if o.hwnd != 0 {
		win.KillTimer(o.hwnd, 1)
		win.DestroyWindow(o.hwnd)
		o.hwnd = 0
	}
}

func (o *overlay) proc(hwnd uintptr, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case win.WM_TIMER:
		v := virtualRect()
		after := win.HWND_TOPMOST
		if o.kind == "demo" && lockOv != nil {
			after = lockOv.hwnd // stay below the lock screen
		}
		win.SetWindowPos(hwnd, after, int(v.Left), int(v.Top), int(v.Right-v.Left), int(v.Bottom-v.Top), win.SWP_NOACTIVATE|win.SWP_SHOWWINDOW)
		if (o.kind != "demo" || lockOv == nil) && win.GetForegroundWindow() != hwnd {
			win.SetForegroundWindow(hwnd)
		}
		if o.kind == "lock" && time.Now().Minute() != o.minute {
			win.InvalidateRect(hwnd)
		}
		return 0, true
	case win.WM_ERASEBKGND:
		return 1, true
	case win.WM_PAINT:
		o.paint(hwnd)
		return 0, true
	case win.WM_CLOSE:
		return 0, true // students cannot close it
	case win.WM_SYSCOMMAND:
		if wp&0xFFF0 == 0xF060 { // SC_CLOSE
			return 0, true
		}
	case win.WM_SETCURSOR:
		win.SetCursor(cursorArrow)
		return 1, true
	case win.WM_DESTROY:
		delete(windows, hwnd)
		return 0, true
	}
	return 0, false
}

func (o *overlay) paint(hwnd uintptr) {
	var ps win.PAINTSTRUCT
	hdc := win.BeginPaint(hwnd, &ps)
	defer win.EndPaint(hwnd, &ps)
	rc := win.GetClientRect(hwnd)
	w, h := int(rc.Right), int(rc.Bottom)
	switch o.kind {
	case "demo":
		paintCanvas(hdc, rc, primaryInClient(), o.canvas, true)
	case "input":
		win.FillColor(hdc, rc, 0)
	default:
		mdc := win.CreateCompatibleDC(hdc)
		bmp := win.CreateCompatibleBitmap(hdc, w, h)
		old := win.SelectObject(mdc, bmp)
		o.paintLock(mdc, w, h)
		win.BitBlt(hdc, 0, 0, w, h, mdc, 0, 0)
		win.SelectObject(mdc, old)
		win.DeleteObject(bmp)
		win.DeleteDC(mdc)
	}
}

func lockBackground(w, h int) []byte {
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		br, bg, bb := 14+14*t, 17+8*t, 33+26*t
		for x := 0; x < w; x++ {
			dx := (float64(x) - float64(w)/2) / float64(w)
			dy := (float64(y) - float64(h)*0.38) / float64(h)
			d := math.Sqrt(dx*dx + dy*dy*2.2)
			g := math.Max(0, 1-d*1.9)
			g = g * g * 0.5
			r := br + (79-br)*g
			gg := bg + (110-bg)*g
			b := bb + (255-bb)*g
			i := (y*w + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(b), byte(gg), byte(r), 255
		}
	}
	return pix
}

func (o *overlay) paintLock(hdc uintptr, w, h int) {
	if o.bgW != w || o.bgH != h {
		o.bg, o.bgW, o.bgH = lockBackground(w, h), w, h
	}
	win.StretchBGRA(hdc, 0, 0, w, h, o.bg, w, h)
	p := primaryInClient()
	cx := int(p.Left+p.Right) / 2
	cy := int(p.Top+p.Bottom) / 2
	win.SetBkTransparent(hdc)

	fTitle := win.CreateFont(px(40), win.FW_SEMIBOLD, "Segoe UI")
	fText := win.CreateFont(px(20), win.FW_NORMAL, "Segoe UI")
	fSmall := win.CreateFont(px(17), win.FW_NORMAL, "Segoe UI")
	defer func() {
		for _, f := range []uintptr{fTitle, fText, fSmall} {
			win.DeleteObject(f)
		}
	}()
	tw := min(px(900), int(p.Right-p.Left)-px(80))
	measure := func(f uintptr, s string) int {
		if s == "" {
			return 0
		}
		r := win.RECT{Right: int32(tw)}
		win.SelectObject(hdc, f)
		win.DrawText(hdc, s, &r, win.DT_WORDBREAK|win.DT_CENTER|win.DT_NOPREFIX|win.DT_CALCRECT)
		return int(r.Bottom)
	}
	gh := px(110)
	th := measure(fTitle, o.title)
	xh := measure(fText, o.text)
	total := gh + px(24) + th + px(14) + xh
	y := cy - total/2

	// glyph inside a soft circle
	circle := win.RECT{Left: int32(cx - px(64)), Top: int32(y - px(10)), Right: int32(cx + px(64)), Bottom: int32(y + px(118))}
	win.EllipseColor(hdc, circle, win.RGB(40, 46, 92))
	drawLockIcon(hdc, cx, int(circle.Top+circle.Bottom)/2, win.RGB(180, 192, 255), win.RGB(40, 46, 92))
	y += gh + px(24)

	draw := func(f uintptr, s string, col uint32, height int) {
		if s == "" {
			return
		}
		r := win.RECT{Left: int32(cx - tw/2), Top: int32(y), Right: int32(cx + tw/2), Bottom: int32(y + height + 4)}
		win.SelectObject(hdc, f)
		win.SetTextColor(hdc, col)
		win.DrawText(hdc, s, &r, win.DT_WORDBREAK|win.DT_CENTER|win.DT_NOPREFIX)
		y += height
	}
	draw(fTitle, o.title, win.RGB(255, 255, 255), th)
	y += px(14)
	draw(fText, o.text, win.RGB(199, 204, 228), xh)

	now := time.Now()
	o.minute = now.Minute()
	fr := win.RECT{Left: p.Left, Top: p.Bottom - int32(px(70)), Right: p.Right, Bottom: p.Bottom - int32(px(30))}
	win.SelectObject(hdc, fSmall)
	win.SetTextColor(hdc, win.RGB(138, 144, 173))
	win.DrawText(hdc, now.Format("15:04")+"   ·   Orbit Classroom", &fr, win.DT_CENTER|win.DT_SINGLELINE|win.DT_VCENTER)
}

// drawLockIcon draws a padlock with GDI shapes (no icon font needed).
func drawLockIcon(hdc uintptr, cx, cy int, fg, bg uint32) {
	sw := px(9)
	shackle := win.RECT{Left: int32(cx - px(21)), Top: int32(cy - px(40)), Right: int32(cx + px(21)), Bottom: int32(cy + px(6))}
	win.RoundRectOutline(hdc, shackle, px(42), fg, sw)
	body := win.RECT{Left: int32(cx - px(34)), Top: int32(cy - px(10)), Right: int32(cx + px(34)), Bottom: int32(cy + px(42))}
	win.RoundRectColor(hdc, body, px(14), fg)
	hole := win.RECT{Left: int32(cx - px(6)), Top: int32(cy + px(4)), Right: int32(cx + px(6)), Bottom: int32(cy + px(16))}
	win.EllipseColor(hdc, hole, bg)
	slot := win.RECT{Left: int32(cx - px(2.5)), Top: int32(cy + px(12)), Right: int32(cx + px(2.5)), Bottom: int32(cy + px(28))}
	win.RoundRectColor(hdc, slot, px(3), bg)
}

// paintCanvas draws the teacher's screen letterboxed into area (client coordinates).
func paintCanvas(hdc uintptr, client, area win.RECT, c *proto.Canvas, black bool) {
	if c == nil {
		win.FillColor(hdc, client, 0)
		return
	}
	c.Lock()
	defer c.Unlock()
	if !c.Ready || c.W == 0 {
		win.FillColor(hdc, client, win.RGB(14, 17, 33))
		f := win.CreateFont(px(22), win.FW_NORMAL, "Segoe UI")
		win.SelectObject(hdc, f)
		win.SetBkTransparent(hdc)
		win.SetTextColor(hdc, win.RGB(199, 204, 228))
		r := area
		win.DrawText(hdc, waitingText, &r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE)
		win.DeleteObject(f)
		return
	}
	aw, ah := float64(area.Right-area.Left), float64(area.Bottom-area.Top)
	s := math.Min(aw/float64(c.W), ah/float64(c.H))
	iw, ih := int32(float64(c.W)*s), int32(float64(c.H)*s)
	ix := area.Left + (int32(aw)-iw)/2
	iy := area.Top + (int32(ah)-ih)/2
	img := win.RECT{Left: ix, Top: iy, Right: ix + iw, Bottom: iy + ih}
	bg := uint32(0)
	if !black {
		bg = win.RGB(24, 26, 34)
	}
	// letterbox bars only (painting the whole client would flicker)
	win.FillColor(hdc, win.RECT{Left: client.Left, Top: client.Top, Right: client.Right, Bottom: img.Top}, bg)
	win.FillColor(hdc, win.RECT{Left: client.Left, Top: img.Bottom, Right: client.Right, Bottom: client.Bottom}, bg)
	win.FillColor(hdc, win.RECT{Left: client.Left, Top: img.Top, Right: img.Left, Bottom: img.Bottom}, bg)
	win.FillColor(hdc, win.RECT{Left: img.Right, Top: img.Top, Right: client.Right, Bottom: img.Bottom}, bg)
	if int(iw) == c.W && int(ih) == c.H {
		win.SetStretchBltMode(hdc, win.COLORONCOLOR)
	} else {
		win.SetStretchBltMode(hdc, win.HALFTONE)
	}
	win.StretchBGRA(hdc, int(ix), int(iy), int(iw), int(ih), c.Pix, c.W, c.H)
}

var waitingText = "…"

// SetWaitingText sets the localised "waiting for the teacher's screen" text.
func SetWaitingText(s string) { waitingText = s }

// ShowLock locks the screen (mode "screen") or only mouse+keyboard (mode "input").
func ShowLock(mode, title, text string) {
	invoke(func() {
		kind := "lock"
		if mode == "input" {
			kind = "input"
		}
		if lockOv != nil && lockOv.kind == kind {
			lockOv.title, lockOv.text = title, text
			win.InvalidateRect(lockOv.hwnd)
			return
		}
		if lockOv != nil {
			lockOv.destroy()
		}
		lockOv = newOverlay(kind)
		lockOv.title, lockOv.text = title, text
		win.InvalidateRect(lockOv.hwnd)
		updateHook()
	})
}

// HideLock removes the lock screen.
func HideLock() {
	invoke(func() {
		if lockOv != nil {
			lockOv.destroy()
			lockOv = nil
		}
		updateHook()
	})
}

// ------------------------------------------------------------ demo

type demoWindow struct {
	hwnd   uintptr
	canvas *proto.Canvas
}

func (d *demoWindow) proc(hwnd uintptr, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case win.WM_ERASEBKGND:
		return 1, true
	case win.WM_PAINT:
		var ps win.PAINTSTRUCT
		hdc := win.BeginPaint(hwnd, &ps)
		rc := win.GetClientRect(hwnd)
		paintCanvas(hdc, rc, rc, d.canvas, false)
		win.EndPaint(hwnd, &ps)
		return 0, true
	case win.WM_CLOSE:
		win.ShowWindow(hwnd, win.SW_MINIMIZE) // the teacher ends the demo
		return 0, true
	case win.WM_DESTROY:
		delete(windows, hwnd)
		return 0, true
	}
	return 0, false
}

// ShowDemo displays the teacher's screen; mode is "fullscreen" or "window".
func ShowDemo(mode, title string, c *proto.Canvas) {
	invoke(func() {
		hideDemoNow()
		if mode == "window" {
			wa := win.GetWorkArea()
			ww, wh := int(wa.Right-wa.Left)*3/4, int(wa.Bottom-wa.Top)*3/4
			x, y := int(wa.Left)+(int(wa.Right-wa.Left)-ww)/2, int(wa.Top)+(int(wa.Bottom-wa.Top)-wh)/2
			d := &demoWindow{canvas: c}
			d.hwnd = win.CreateWindow(win.WS_EX_APPWINDOW, "OrbitDemo", title, win.WS_OVERLAPPEDWINDOW|win.WS_VISIBLE, x, y, ww, wh, 0)
			windows[d.hwnd] = d
			win.ShowWindow(d.hwnd, win.SW_SHOW)
			win.SetForegroundWindow(d.hwnd)
			demoWin = d
			demoHwnd.Store(d.hwnd)
		} else {
			demoOv = newOverlay("demo")
			demoOv.canvas = c
			demoHwnd.Store(demoOv.hwnd)
			if lockOv != nil {
				win.SetForegroundWindow(lockOv.hwnd)
			}
		}
		updateHook()
	})
}

// DemoUpdated repaints the demo window (callable from any goroutine).
func DemoUpdated() {
	if h := demoHwnd.Load(); h != 0 {
		win.InvalidateRect(h)
	}
}

func hideDemoNow() {
	demoHwnd.Store(0)
	if demoOv != nil {
		demoOv.destroy()
		demoOv = nil
	}
	if demoWin != nil {
		win.DestroyWindow(demoWin.hwnd)
		demoWin = nil
	}
	updateHook()
}

// HideDemo closes the demo window.
func HideDemo() { invoke(hideDemoNow) }

// ------------------------------------------------------------ cards

type card struct {
	hwnd    uintptr
	c       Card
	cb      func(int, string)
	w, h    int
	glyphRc win.RECT
	titleRc win.RECT
	bodyRc  win.RECT
	inputRc win.RECT
	btns    []win.RECT
	hover   int
	pressed int
	edit    uintptr
	fonts   []uintptr
	fTitle  uintptr
	fBody   uintptr
	fBtn    uintptr
	fGlyph  uintptr
	white   uintptr
	done    bool
}

// ShowCard opens a dialog card. cb receives the clicked button index (or the
// Cancel index) and the text box content. The returned function closes the
// card programmatically without calling cb.
func ShowCard(c Card, cb func(int, string)) func() {
	var cd *card
	invoke(func() { cd = newCard(c, cb) })
	return func() {
		invoke(func() {
			if cd != nil && !cd.done {
				cd.done = true
				win.DestroyWindow(cd.hwnd)
			}
		})
	}
}

func newCard(c Card, cb func(int, string)) *card {
	cd := &card{c: c, cb: cb, hover: -1, pressed: -1}
	if c.Accent == 0 {
		cd.c.Accent = accent
	}
	cd.fTitle = win.CreateFont(px(20), win.FW_SEMIBOLD, "Segoe UI")
	cd.fBody = win.CreateFont(px(15), win.FW_NORMAL, "Segoe UI")
	cd.fBtn = win.CreateFont(px(15), win.FW_SEMIBOLD, "Segoe UI")
	if hasGlyphFont {
		cd.fGlyph = win.CreateFont(px(22), win.FW_NORMAL, glyphFont)
	} else {
		cd.fGlyph = win.CreateFont(px(22), win.FW_BOLD, "Segoe UI")
	}
	cd.fonts = []uintptr{cd.fTitle, cd.fBody, cd.fBtn, cd.fGlyph}
	cd.white = win.CreateSolidBrush(win.RGB(255, 255, 255))

	W := px(float64(max(440, c.MinWidthDIP)))
	pad := px(28)
	hdc := win.GetDC(0)
	measure := func(f uintptr, s string, width int, flags uint32) (int, int) {
		r := win.RECT{Right: int32(width)}
		win.SelectObject(hdc, f)
		win.DrawText(hdc, s, &r, flags|win.DT_CALCRECT|win.DT_NOPREFIX)
		return int(r.Right), int(r.Bottom)
	}
	g := px(44)
	cd.glyphRc = win.RECT{Left: int32(pad), Top: int32(pad), Right: int32(pad + g), Bottom: int32(pad + g)}
	tx := pad + g + px(14)
	_, th := measure(cd.fTitle, c.Title, W-pad-tx, win.DT_WORDBREAK)
	ty := pad + max(0, (g-th)/2)
	cd.titleRc = win.RECT{Left: int32(tx), Top: int32(ty), Right: int32(W - pad), Bottom: int32(ty + th)}
	y := pad + max(g, th) + px(18)
	if c.Body != "" {
		_, bh := measure(cd.fBody, c.Body, W-2*pad, win.DT_WORDBREAK)
		cd.bodyRc = win.RECT{Left: int32(pad), Top: int32(y), Right: int32(W - pad), Bottom: int32(y + bh)}
		y += bh + px(20)
	}
	if c.Input {
		ih := px(42)
		if c.Multiline {
			ih = px(110)
		}
		cd.inputRc = win.RECT{Left: int32(pad), Top: int32(y), Right: int32(W - pad), Bottom: int32(y + ih)}
		y += ih + px(20)
	}
	bh := px(40)
	widths := make([]int, len(c.Buttons))
	total := 0
	for i, b := range c.Buttons {
		tw, _ := measure(cd.fBtn, b, 2000, win.DT_SINGLELINE)
		widths[i] = max(px(96), tw+px(36))
		total += widths[i] + px(10)
	}
	if c.Vertical || total-px(10) > W-2*pad {
		for range c.Buttons {
			cd.btns = append(cd.btns, win.RECT{Left: int32(pad), Top: int32(y), Right: int32(W - pad), Bottom: int32(y + bh)})
			y += bh + px(10)
		}
		y -= px(10) + bh // the common "y += bh" below adds the last row back
	} else {
		x := W - pad
		cd.btns = make([]win.RECT, len(c.Buttons))
		for i := len(c.Buttons) - 1; i >= 0; i-- {
			cd.btns[i] = win.RECT{Left: int32(x - widths[i]), Top: int32(y), Right: int32(x), Bottom: int32(y + bh)}
			x -= widths[i] + px(10)
		}
	}
	if len(c.Buttons) > 0 {
		y += bh
	}
	H := y + pad
	win.ReleaseDC(0, hdc)
	cd.w, cd.h = W, H

	wa := win.GetWorkArea()
	x0 := int(wa.Left) + (int(wa.Right-wa.Left)-W)/2
	y0 := int(wa.Top) + (int(wa.Bottom-wa.Top)-H)/2
	ex := uint32(win.WS_EX_APPWINDOW)
	if c.Topmost {
		ex |= win.WS_EX_TOPMOST
	}
	cd.hwnd = win.CreateWindow(ex, "OrbitCard", c.Title, win.WS_POPUP|win.WS_CLIPCHILDREN, x0, y0, W, H, 0)
	windows[cd.hwnd] = cd
	win.SetWindowRounded(cd.hwnd)
	if c.Input {
		style := uint32(win.WS_CHILD | win.WS_VISIBLE | win.WS_TABSTOP)
		if c.Multiline {
			style |= win.ES_MULTILINE | win.ES_AUTOVSCROLL | win.ES_WANTRETURN | win.WS_VSCROLL
		} else {
			style |= win.ES_AUTOHSCROLL
		}
		r := cd.inputRc
		in := int32(px(10))
		top := r.Top + in
		eh := r.Bottom - r.Top - 2*in
		if !c.Multiline {
			eh = int32(px(22))
			top = r.Top + (r.Bottom-r.Top-eh)/2
		}
		cd.edit = win.CreateWindow(0, "EDIT", c.InputText, style, int(r.Left+in), int(top), int(r.Right-r.Left-2*in), int(eh), cd.hwnd)
		win.SendMessage(cd.edit, win.WM_SETFONT, cd.fBody, 1)
	}
	win.ShowWindow(cd.hwnd, win.SW_SHOW)
	win.SetForegroundWindow(cd.hwnd)
	win.FlashWindow(cd.hwnd)
	if cd.edit != 0 {
		win.SetFocus(cd.edit)
	}
	if c.Beep {
		win.MessageBeep(0x40)
	}
	return cd
}

func (cd *card) hit(x, y int) int {
	for i, r := range cd.btns {
		if x >= int(r.Left) && x < int(r.Right) && y >= int(r.Top) && y < int(r.Bottom) {
			return i
		}
	}
	return -1
}

func (cd *card) finish(idx int) {
	if cd.done {
		return
	}
	cd.done = true
	text := ""
	if cd.edit != 0 {
		text = win.GetWindowText(cd.edit)
	}
	win.DestroyWindow(cd.hwnd)
	if cd.cb != nil {
		cb := cd.cb
		go cb(idx, strings.TrimSpace(text))
	}
}

func (cd *card) proc(hwnd uintptr, msg uint32, wp, lp uintptr) (uintptr, bool) {
	switch msg {
	case win.WM_ERASEBKGND:
		return 1, true
	case win.WM_PAINT:
		cd.paint(hwnd)
		return 0, true
	case win.WM_NCHITTEST:
		p := win.ScreenToClient(hwnd, win.POINT{X: int32(win.LoWord(lp)), Y: int32(win.HiWord(lp))})
		if cd.hit(int(p.X), int(p.Y)) >= 0 {
			return win.HTCLIENT, true
		}
		r := cd.inputRc
		if cd.edit != 0 && p.X >= r.Left && p.X < r.Right && p.Y >= r.Top && p.Y < r.Bottom {
			return win.HTCLIENT, true
		}
		return win.HTCAPTION, true // drag the card anywhere else
	case win.WM_MOUSEMOVE:
		i := cd.hit(win.LoWord(lp), win.HiWord(lp))
		if i != cd.hover {
			cd.hover = i
			win.InvalidateRect(hwnd)
		}
		win.TrackMouseLeave(hwnd)
		return 0, true
	case win.WM_MOUSELEAVE:
		cd.hover = -1
		win.InvalidateRect(hwnd)
		return 0, true
	case win.WM_SETCURSOR:
		if cd.hover >= 0 {
			win.SetCursor(cursorHand)
			return 1, true
		}
	case win.WM_LBUTTONDOWN:
		cd.pressed = cd.hit(win.LoWord(lp), win.HiWord(lp))
		return 0, true
	case win.WM_LBUTTONUP:
		if i := cd.hit(win.LoWord(lp), win.HiWord(lp)); i >= 0 && i == cd.pressed {
			cd.finish(i)
		}
		cd.pressed = -1
		return 0, true
	case win.WM_KEYDOWN:
		if wp == win.VK_RETURN && len(cd.c.Buttons) > 0 && !cd.c.Vertical {
			cd.finish(cd.c.Primary)
		} else if wp == win.VK_ESCAPE && cd.c.Cancel >= 0 {
			cd.finish(cd.c.Cancel)
		}
		return 0, true
	case win.WM_CTLCOLOREDIT:
		win.SetBkColor(wp, win.RGB(255, 255, 255))
		win.SetTextColor(wp, win.RGB(20, 24, 39))
		return cd.white, true
	case win.WM_CLOSE:
		if cd.c.Cancel >= 0 {
			cd.finish(cd.c.Cancel)
		}
		return 0, true
	case win.WM_DESTROY:
		delete(windows, hwnd)
		for _, f := range cd.fonts {
			win.DeleteObject(f)
		}
		win.DeleteObject(cd.white)
		return 0, true
	}
	return 0, false
}

func (cd *card) paint(hwnd uintptr) {
	var ps win.PAINTSTRUCT
	hdc := win.BeginPaint(hwnd, &ps)
	defer win.EndPaint(hwnd, &ps)
	w, h := cd.w, cd.h
	mdc := win.CreateCompatibleDC(hdc)
	bmp := win.CreateCompatibleBitmap(hdc, w, h)
	old := win.SelectObject(mdc, bmp)
	full := win.RECT{Right: int32(w), Bottom: int32(h)}
	win.FillColor(mdc, full, win.RGB(255, 255, 255))
	win.RoundRectOutline(mdc, full, 0, win.RGB(222, 226, 236), 1)
	win.SetBkTransparent(mdc)

	acc := cd.c.Accent
	ar, ag, ab := byte(acc), byte(acc>>8), byte(acc>>16)
	win.EllipseColor(mdc, cd.glyphRc, win.RGB(byte(int(ar)+(255-int(ar))*85/100), byte(int(ag)+(255-int(ag))*85/100), byte(int(ab)+(255-int(ab))*85/100)))
	glyph := cd.c.Glyph
	if glyph == 0 {
		glyph = GlyphInfo
	}
	text := string(glyph)
	if !hasGlyphFont { // icon font missing: use a plain character instead
		text = map[rune]string{GlyphMessage: "!", GlyphPoll: "?", GlyphKey: "#", GlyphSend: "»", GlyphLock: "!"}[glyph]
		if text == "" {
			text = "i"
		}
	}
	win.SelectObject(mdc, cd.fGlyph)
	win.SetTextColor(mdc, acc)
	gr := cd.glyphRc
	win.DrawText(mdc, text, &gr, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE)

	win.SelectObject(mdc, cd.fTitle)
	win.SetTextColor(mdc, win.RGB(20, 24, 39))
	tr := cd.titleRc
	win.DrawText(mdc, cd.c.Title, &tr, win.DT_WORDBREAK|win.DT_NOPREFIX)

	if cd.c.Body != "" {
		win.SelectObject(mdc, cd.fBody)
		win.SetTextColor(mdc, win.RGB(59, 65, 87))
		br := cd.bodyRc
		win.DrawText(mdc, cd.c.Body, &br, win.DT_WORDBREAK|win.DT_NOPREFIX)
	}
	if cd.edit != 0 {
		win.RoundRectOutline(mdc, cd.inputRc, px(10), win.RGB(196, 202, 218), 1)
	}
	win.SelectObject(mdc, cd.fBtn)
	for i, r := range cd.btns {
		var fill, text uint32
		if i == cd.c.Primary && !cd.c.Vertical {
			fill, text = acc, win.RGB(255, 255, 255)
			if i == cd.hover {
				fill = win.RGB(byte(int(ar)*85/100), byte(int(ag)*85/100), byte(int(ab)*85/100))
			}
		} else {
			fill, text = win.RGB(238, 240, 246), win.RGB(28, 33, 51)
			if i == cd.hover {
				fill = win.RGB(224, 228, 240)
				if cd.c.Vertical {
					fill, text = acc, win.RGB(255, 255, 255)
				}
			}
		}
		win.RoundRectColor(mdc, r, px(10), fill)
		win.SetTextColor(mdc, text)
		rr := r
		win.DrawText(mdc, cd.c.Buttons[i], &rr, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_NOPREFIX)
	}
	win.BitBlt(hdc, 0, 0, w, h, mdc, 0, 0)
	win.SelectObject(mdc, old)
	win.DeleteObject(bmp)
	win.DeleteDC(mdc)
}
