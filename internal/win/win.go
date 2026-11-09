//go:build windows

// Package win contains the small subset of the Win32 API used by Orbit,
// called directly through syscalls (no cgo, no external dependencies).
package win

import (
	"os"
	"sync"
	"syscall"
	"unsafe"
)

func sysDLL(name string) *syscall.LazyDLL {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	// load from System32 explicitly to avoid DLL search-order hijacking
	return syscall.NewLazyDLL(root + `\System32\` + name)
}

var (
	user32   = sysDLL("user32.dll")
	gdi32    = sysDLL("gdi32.dll")
	kernel32 = sysDLL("kernel32.dll")
	shell32  = sysDLL("shell32.dll")
	ole32    = sysDLL("ole32.dll")
	dwmapi   = sysDLL("dwmapi.dll")
	ntdll    = sysDLL("ntdll.dll")
	shcore   = sysDLL("shcore.dll")
)

var (
	pGetDC                         = user32.NewProc("GetDC")
	pReleaseDC                     = user32.NewProc("ReleaseDC")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pGetCursorInfo                 = user32.NewProc("GetCursorInfo")
	pGetIconInfo                   = user32.NewProc("GetIconInfo")
	pDrawIconEx                    = user32.NewProc("DrawIconEx")
	pSendInput                     = user32.NewProc("SendInput")
	pGetLastInputInfo              = user32.NewProc("GetLastInputInfo")
	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	pGetWindowTextW                = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	pGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                 = user32.NewProc("DestroyWindow")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pUpdateWindow                  = user32.NewProc("UpdateWindow")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pSendMessageW                  = user32.NewProc("SendMessageW")
	pInvalidateRect                = user32.NewProc("InvalidateRect")
	pBeginPaint                    = user32.NewProc("BeginPaint")
	pEndPaint                      = user32.NewProc("EndPaint")
	pFillRect                      = user32.NewProc("FillRect")
	pDrawTextW                     = user32.NewProc("DrawTextW")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pSetWindowsHookExW             = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHookEx           = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx                = user32.NewProc("CallNextHookEx")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pSetCursor                     = user32.NewProc("SetCursor")
	pCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                   = user32.NewProc("AppendMenuW")
	pTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                   = user32.NewProc("DestroyMenu")
	pCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	pDestroyIcon                   = user32.NewProc("DestroyIcon")
	pMessageBoxW                   = user32.NewProc("MessageBoxW")
	pSetTimer                      = user32.NewProc("SetTimer")
	pKillTimer                     = user32.NewProc("KillTimer")
	pGetClientRect                 = user32.NewProc("GetClientRect")
	pSetLayeredWindowAttributes    = user32.NewProc("SetLayeredWindowAttributes")
	pRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	pGetDpiForSystem               = user32.NewProc("GetDpiForSystem")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	pTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	pSetFocus                      = user32.NewProc("SetFocus")
	pEnumDisplayMonitors           = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	pRegisterHotKey                = user32.NewProc("RegisterHotKey")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pMessageBeep                   = user32.NewProc("MessageBeep")
	pIsWindow                      = user32.NewProc("IsWindow")
	pBringWindowToTop              = user32.NewProc("BringWindowToTop")
	pScreenToClient                = user32.NewProc("ScreenToClient")
	pFlashWindow                   = user32.NewProc("FlashWindow")
	pSystemParametersInfoW         = user32.NewProc("SystemParametersInfoW")
	pMapVirtualKeyW                = user32.NewProc("MapVirtualKeyW")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pStretchBlt             = gdi32.NewProc("StretchBlt")
	pSetStretchBltMode      = gdi32.NewProc("SetStretchBltMode")
	pSetBrushOrgEx          = gdi32.NewProc("SetBrushOrgEx")
	pStretchDIBits          = gdi32.NewProc("StretchDIBits")
	pSetDIBitsToDevice      = gdi32.NewProc("SetDIBitsToDevice")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreatePen              = gdi32.NewProc("CreatePen")
	pCreateFontW            = gdi32.NewProc("CreateFontW")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pRoundRect              = gdi32.NewProc("RoundRect")
	pEllipse                = gdi32.NewProc("Ellipse")
	pGetStockObject         = gdi32.NewProc("GetStockObject")
	pCreateBitmap           = gdi32.NewProc("CreateBitmap")
	pGdiFlush               = gdi32.NewProc("GdiFlush")
	pGetDeviceCaps          = gdi32.NewProc("GetDeviceCaps")
	pSetBkColor             = gdi32.NewProc("SetBkColor")
	pGetTextFaceW           = gdi32.NewProc("GetTextFaceW")

	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pGetTickCount               = kernel32.NewProc("GetTickCount")
	pCreateMutexW               = kernel32.NewProc("CreateMutexW")
	pQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	pGetUserDefaultUILanguage   = kernel32.NewProc("GetUserDefaultUILanguage")

	pShellNotifyIconW     = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW        = shell32.NewProc("ShellExecuteW")
	pShellExecuteExW      = shell32.NewProc("ShellExecuteExW")
	pSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	pIsUserAnAdmin        = shell32.NewProc("IsUserAnAdmin")

	pCoTaskMemFree = ole32.NewProc("CoTaskMemFree")

	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	pRtlGetVersion = ntdll.NewProc("RtlGetVersion")

	pSetProcessDpiAwareness = shcore.NewProc("SetProcessDpiAwareness")
)

// ---- constants ----

const (
	SM_CXSCREEN        = 0
	SM_CYSCREEN        = 1
	SM_CXICON          = 11
	SM_CXCURSOR        = 13
	SM_CYCURSOR        = 14
	SM_CXSMICON        = 49
	SM_XVIRTUALSCREEN  = 76
	SM_YVIRTUALSCREEN  = 77
	SM_CXVIRTUALSCREEN = 78
	SM_CYVIRTUALSCREEN = 79

	SRCCOPY        = 0x00CC0020
	HALFTONE       = 4
	COLORONCOLOR   = 3
	DIB_RGB_COLORS = 0
	BI_RGB         = 0
	DI_NORMAL      = 3
	CURSOR_SHOWING = 1
	LOGPIXELSX     = 88

	WS_POPUP            = 0x80000000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_BORDER           = 0x00800000
	WS_VSCROLL          = 0x00200000
	WS_TABSTOP          = 0x00010000
	WS_CLIPCHILDREN     = 0x02000000
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_EX_TOPMOST       = 0x00000008
	WS_EX_TOOLWINDOW    = 0x00000080
	WS_EX_LAYERED       = 0x00080000
	WS_EX_APPWINDOW     = 0x00040000
	WS_EX_NOACTIVATE    = 0x08000000
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_AUTOHSCROLL      = 0x0080
	ES_WANTRETURN       = 0x1000
	CS_HREDRAW          = 0x0002
	CS_VREDRAW          = 0x0001
	CS_DROPSHADOW       = 0x00020000
	CW_USEDEFAULT       = 0x80000000

	WM_CREATE        = 0x0001
	WM_DESTROY       = 0x0002
	WM_SIZE          = 0x0005
	WM_ACTIVATE      = 0x0006
	WM_PAINT         = 0x000F
	WM_CLOSE         = 0x0010
	WM_ERASEBKGND    = 0x0014
	WM_SETCURSOR     = 0x0020
	WM_SETFONT       = 0x0030
	WM_DISPLAYCHANGE = 0x007E
	WM_CONTEXTMENU   = 0x007B
	WM_NCHITTEST     = 0x0084
	WM_KEYDOWN       = 0x0100
	WM_SYSKEYDOWN    = 0x0104
	WM_COMMAND       = 0x0111
	WM_SYSCOMMAND    = 0x0112
	WM_TIMER         = 0x0113
	WM_CTLCOLOREDIT  = 0x0133
	WM_MOUSEMOVE     = 0x0200
	WM_LBUTTONDOWN   = 0x0201
	WM_LBUTTONUP     = 0x0202
	WM_RBUTTONUP     = 0x0205
	WM_MOUSELEAVE    = 0x02A3
	WM_HOTKEY        = 0x0312
	WM_APP           = 0x8000
	WM_NULL          = 0x0000

	HTCLIENT  = 1
	HTCAPTION = 2

	SW_HIDE           = 0
	SW_SHOWNORMAL     = 1
	SW_SHOWNOACTIVATE = 4
	SW_SHOW           = 5
	SW_MINIMIZE       = 6

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_SHOWWINDOW   = 0x0040
	SWP_FRAMECHANGED = 0x0020

	WH_KEYBOARD_LL = 13
	LLKHF_INJECTED = 0x10

	NIM_ADD       = 0
	NIM_MODIFY    = 1
	NIM_DELETE    = 2
	NIF_MESSAGE   = 0x01
	NIF_ICON      = 0x02
	NIF_TIP       = 0x04
	NIF_INFO      = 0x10
	NIIF_INFO     = 0x01
	NIIF_USER     = 0x04
	NIIF_NOSOUND  = 0x10
	MF_STRING     = 0x0000
	MF_GRAYED     = 0x0001
	MF_CHECKED    = 0x0008
	MF_SEPARATOR  = 0x0800
	TPM_RETURNCMD = 0x0100
	TPM_RIGHTBTN  = 0x0002

	DT_CENTER     = 0x0001
	DT_VCENTER    = 0x0004
	DT_WORDBREAK  = 0x0010
	DT_SINGLELINE = 0x0020
	DT_CALCRECT   = 0x0400
	DT_NOPREFIX   = 0x0800
	DT_END_ELLIPS = 0x8000

	TRANSPARENT       = 1
	FW_NORMAL         = 400
	FW_SEMIBOLD       = 600
	FW_BOLD           = 700
	CLEARTYPE_QUALITY = 5
	DEFAULT_CHARSET   = 1
	PS_SOLID          = 0
	NULL_PEN          = 8
	NULL_BRUSH        = 5

	IDC_ARROW = 32512
	IDC_HAND  = 32649
	IDC_IBEAM = 32513

	LWA_ALPHA = 2

	MOD_ALT      = 0x0001
	MOD_CONTROL  = 0x0002
	MOD_NOREPEAT = 0x4000

	MB_OK              = 0x00000000
	MB_YESNO           = 0x00000004
	MB_ICONERROR       = 0x00000010
	MB_ICONWARNING     = 0x00000030
	MB_ICONINFORMATION = 0x00000040
	MB_SETFOREGROUND   = 0x00010000
	MB_TOPMOST         = 0x00040000
	IDYES              = 6

	TME_LEAVE = 0x00000002

	INPUT_MOUSE            = 0
	INPUT_KEYBOARD         = 1
	MOUSEEVENTF_MOVE       = 0x0001
	MOUSEEVENTF_LEFTDOWN   = 0x0002
	MOUSEEVENTF_LEFTUP     = 0x0004
	MOUSEEVENTF_RIGHTDOWN  = 0x0008
	MOUSEEVENTF_RIGHTUP    = 0x0010
	MOUSEEVENTF_MIDDLEDOWN = 0x0020
	MOUSEEVENTF_MIDDLEUP   = 0x0040
	MOUSEEVENTF_WHEEL      = 0x0800
	MOUSEEVENTF_HWHEEL     = 0x1000
	MOUSEEVENTF_VIRTUALDSK = 0x4000
	MOUSEEVENTF_ABSOLUTE   = 0x8000
	KEYEVENTF_EXTENDEDKEY  = 0x0001
	KEYEVENTF_KEYUP        = 0x0002
	KEYEVENTF_UNICODE      = 0x0004
	KEYEVENTF_SCANCODE     = 0x0008

	VK_RETURN = 0x0D
	VK_ESCAPE = 0x1B

	SEE_MASK_NOCLOSEPROCESS = 0x00000040
	SEE_MASK_NOASYNC        = 0x00000100

	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWCP_ROUND                   = 2
)

var (
	HWND_TOPMOST   = ^uintptr(0)     // -1
	HWND_NOTOPMOST = ^uintptr(0) - 1 // -2
)

// ---- structures ----

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	Private uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type PAINTSTRUCT struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     RECT
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	Header BITMAPINFOHEADER
	Colors [1]uint32
}

type CURSORINFO struct {
	CbSize  uint32
	Flags   uint32
	HCursor uintptr
	Pt      POINT
}

type ICONINFO struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type LASTINPUTINFO struct {
	CbSize uint32
	DwTime uint32
}

type MOUSEINPUT struct {
	Dx, Dy      int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT is the 40 byte SendInput structure (amd64 / arm64).
type INPUT struct {
	Type uint32
	_    uint32
	Mi   MOUSEINPUT
}

// KeyInput builds an INPUT holding a KEYBDINPUT.
func KeyInput(k KEYBDINPUT) INPUT {
	var in INPUT
	in.Type = INPUT_KEYBOARD
	*(*KEYBDINPUT)(unsafe.Pointer(&in.Mi)) = k
	return in
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type NOTIFYICONDATAW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type TRACKMOUSEEVENT struct {
	CbSize      uint32
	DwFlags     uint32
	HwndTrack   uintptr
	DwHoverTime uint32
}

type KBDLLHOOKSTRUCT struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type SHELLEXECUTEINFOW struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         uintptr
	LpVerb       *uint16
	LpFile       *uint16
	LpParameters *uint16
	LpDirectory  *uint16
	NShow        int32
	HInstApp     uintptr
	LpIDList     uintptr
	LpClass      *uint16
	HkeyClass    uintptr
	DwHotKey     uint32
	HIcon        uintptr
	HProcess     uintptr
}

type OSVERSIONINFOW struct {
	Size       uint32
	Major      uint32
	Minor      uint32
	Build      uint32
	PlatformID uint32
	CSDVersion [128]uint16
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	FOLDERID_Desktop  = GUID{0xB4BFCC3A, 0xDB2C, 0x424C, [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}
	FOLDERID_Pictures = GUID{0x33E28130, 0x4E1E, 0x4676, [8]byte{0x83, 0x5A, 0x98, 0x39, 0x5C, 0x3B, 0xC3, 0xBB}}
)

// ---- helpers ----

func U16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func ptr[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }

func RGB(r, g, b byte) uint32 { return uint32(r) | uint32(g)<<8 | uint32(b)<<16 }

func LoWord(v uintptr) int { return int(int16(v & 0xFFFF)) }
func HiWord(v uintptr) int { return int(int16((v >> 16) & 0xFFFF)) }

// ---- wrappers ----

func GetDC(hwnd uintptr) uintptr  { r, _, _ := pGetDC.Call(hwnd); return r }
func ReleaseDC(hwnd, hdc uintptr) { pReleaseDC.Call(hwnd, hdc) }
func GetSystemMetrics(i int) int  { r, _, _ := pGetSystemMetrics.Call(uintptr(i)); return int(int32(r)) }
func CreateCompatibleDC(hdc uintptr) uintptr {
	r, _, _ := pCreateCompatibleDC.Call(hdc)
	return r
}
func CreateCompatibleBitmap(hdc uintptr, w, h int) uintptr {
	r, _, _ := pCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	return r
}
func DeleteDC(hdc uintptr)                  { pDeleteDC.Call(hdc) }
func SelectObject(hdc, obj uintptr) uintptr { r, _, _ := pSelectObject.Call(hdc, obj); return r }
func DeleteObject(obj uintptr)              { pDeleteObject.Call(obj) }
func GdiFlush()                             { pGdiFlush.Call() }
func GetStockObject(i int) uintptr          { r, _, _ := pGetStockObject.Call(uintptr(i)); return r }
func SetStretchBltMode(hdc uintptr, mode int) {
	pSetStretchBltMode.Call(hdc, uintptr(mode))
	pSetBrushOrgEx.Call(hdc, 0, 0, 0)
}
func GetDeviceCaps(hdc uintptr, i int) int {
	r, _, _ := pGetDeviceCaps.Call(hdc, uintptr(i))
	return int(r)
}

// CreateDIB creates a 32bpp top-down DIB section and returns its pixel memory.
func CreateDIB(hdc uintptr, w, h int) (hbm uintptr, bits []byte) {
	bmi := BITMAPINFO{Header: BITMAPINFOHEADER{BiSize: 40, BiWidth: int32(w), BiHeight: int32(-h), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	var p uintptr
	hbm, _, _ = pCreateDIBSection.Call(hdc, ptr(&bmi), DIB_RGB_COLORS, ptr(&p), 0, 0)
	if hbm == 0 || p == 0 {
		return 0, nil
	}
	return hbm, unsafe.Slice((*byte)(unsafe.Pointer(p)), w*h*4)
}

func BitBlt(dst uintptr, x, y, w, h int, src uintptr, sx, sy int) bool {
	r, _, _ := pBitBlt.Call(dst, uintptr(x), uintptr(y), uintptr(w), uintptr(h), src, uintptr(sx), uintptr(sy), SRCCOPY)
	return r != 0
}

func StretchBlt(dst uintptr, x, y, w, h int, src uintptr, sx, sy, sw, sh int) bool {
	r, _, _ := pStretchBlt.Call(dst, uintptr(x), uintptr(y), uintptr(w), uintptr(h), src, uintptr(sx), uintptr(sy), uintptr(sw), uintptr(sh), SRCCOPY)
	return r != 0
}

// StretchBGRA paints a top-down BGRA buffer onto hdc.
func StretchBGRA(hdc uintptr, dx, dy, dw, dh int, pix []byte, sw, sh int) {
	if len(pix) < sw*sh*4 || sw == 0 || sh == 0 {
		return
	}
	bmi := BITMAPINFO{Header: BITMAPINFOHEADER{BiSize: 40, BiWidth: int32(sw), BiHeight: int32(-sh), BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB}}
	pStretchDIBits.Call(hdc, uintptr(dx), uintptr(dy), uintptr(dw), uintptr(dh), 0, 0, uintptr(sw), uintptr(sh),
		uintptr(unsafe.Pointer(&pix[0])), ptr(&bmi), DIB_RGB_COLORS, SRCCOPY)
}

func GetCursorInfo() (CURSORINFO, bool) {
	ci := CURSORINFO{CbSize: uint32(unsafe.Sizeof(CURSORINFO{}))}
	r, _, _ := pGetCursorInfo.Call(ptr(&ci))
	return ci, r != 0
}

// CursorHotspot returns the hotspot of a cursor handle.
func CursorHotspot(hc uintptr) (int, int) {
	var ii ICONINFO
	r, _, _ := pGetIconInfo.Call(hc, ptr(&ii))
	if r == 0 {
		return 0, 0
	}
	if ii.HbmMask != 0 {
		DeleteObject(ii.HbmMask)
	}
	if ii.HbmColor != 0 {
		DeleteObject(ii.HbmColor)
	}
	return int(ii.XHotspot), int(ii.YHotspot)
}

func DrawIcon(hdc uintptr, x, y int, icon uintptr, w, h int) {
	pDrawIconEx.Call(hdc, uintptr(x), uintptr(y), icon, uintptr(w), uintptr(h), 0, 0, DI_NORMAL)
}

func SendInput(in []INPUT) int {
	if len(in) == 0 {
		return 0
	}
	r, _, _ := pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(INPUT{}))
	return int(r)
}

func IdleMillis() uint32 {
	li := LASTINPUTINFO{CbSize: 8}
	pGetLastInputInfo.Call(ptr(&li))
	t, _, _ := pGetTickCount.Call()
	return uint32(t) - li.DwTime
}

// ScanToVK maps a scan code (with optional E0 prefix) to a virtual-key code.
func ScanToVK(scan uint16, ext bool) uint16 {
	code := uintptr(scan)
	if ext {
		code |= 0xE000
	}
	r, _, _ := pMapVirtualKeyW.Call(code, 3) // MAPVK_VSC_TO_VK_EX
	return uint16(r)
}

func GetForegroundWindow() uintptr { r, _, _ := pGetForegroundWindow.Call(); return r }

func GetWindowText(hwnd uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func GetWindowPID(hwnd uintptr) uint32 {
	var pid uint32
	pGetWindowThreadProcessId.Call(hwnd, ptr(&pid))
	return pid
}

// ProcessImage returns the full exe path of a process.
func ProcessImage(pid uint32) string {
	const PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	h, err := syscall.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := pQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), ptr(&n))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func GetModuleHandle() uintptr { r, _, _ := pGetModuleHandleW.Call(0); return r }

func RegisterClass(name string, proc uintptr, style uint32, cursor uintptr, icon uintptr) error {
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         style,
		LpfnWndProc:   proc,
		HInstance:     GetModuleHandle(),
		HCursor:       cursor,
		HIcon:         icon,
		HIconSm:       icon,
		LpszClassName: U16(name),
	}
	r, _, err := pRegisterClassExW.Call(ptr(&wc))
	if r == 0 {
		return err
	}
	return nil
}

func CreateWindow(exStyle uint32, class, title string, style uint32, x, y, w, h int, parent uintptr) uintptr {
	r, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(U16(class))), uintptr(unsafe.Pointer(U16(title))),
		uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, 0, GetModuleHandle(), 0)
	return r
}

func DefWindowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func DestroyWindow(hwnd uintptr)        { pDestroyWindow.Call(hwnd) }
func ShowWindow(hwnd uintptr, cmd int)  { pShowWindow.Call(hwnd, uintptr(cmd)) }
func UpdateWindow(hwnd uintptr)         { pUpdateWindow.Call(hwnd) }
func IsWindow(hwnd uintptr) bool        { r, _, _ := pIsWindow.Call(hwnd); return r != 0 }
func SetForegroundWindow(hwnd uintptr)  { pSetForegroundWindow.Call(hwnd); pBringWindowToTop.Call(hwnd) }
func SetFocus(hwnd uintptr)             { pSetFocus.Call(hwnd) }
func FlashWindow(hwnd uintptr)          { pFlashWindow.Call(hwnd, 1) }
func MessageBeep(t uint32)              { pMessageBeep.Call(uintptr(t)) }
func InvalidateRect(hwnd uintptr)       { pInvalidateRect.Call(hwnd, 0, 0) }
func SetTimer(hwnd, id uintptr, ms int) { pSetTimer.Call(hwnd, id, uintptr(ms), 0) }
func KillTimer(hwnd, id uintptr)        { pKillTimer.Call(hwnd, id) }
func PostMessage(hwnd uintptr, msg uint32, wp, lp uintptr) {
	pPostMessageW.Call(hwnd, uintptr(msg), wp, lp)
}
func SendMessage(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
func SetWindowPos(hwnd, after uintptr, x, y, w, h int, flags uint32) {
	pSetWindowPos.Call(hwnd, after, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(flags))
}
func GetClientRect(hwnd uintptr) RECT {
	var r RECT
	pGetClientRect.Call(hwnd, ptr(&r))
	return r
}
func SetLayeredAlpha(hwnd uintptr, alpha byte) {
	pSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), LWA_ALPHA)
}
func RegisterWindowMessage(name string) uint32 {
	r, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(U16(name))))
	return uint32(r)
}
func LoadCursor(id int) uintptr { r, _, _ := pLoadCursorW.Call(0, uintptr(id)); return r }
func SetCursor(c uintptr)       { pSetCursor.Call(c) }
func TrackMouseLeave(hwnd uintptr) {
	t := TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})), DwFlags: TME_LEAVE, HwndTrack: hwnd}
	pTrackMouseEvent.Call(ptr(&t))
}
func GetCursorPos() POINT {
	var p POINT
	pGetCursorPos.Call(ptr(&p))
	return p
}
func ScreenToClient(hwnd uintptr, p POINT) POINT {
	pScreenToClient.Call(hwnd, ptr(&p))
	return p
}

func GetMessage(m *MSG) int32 {
	r, _, _ := pGetMessageW.Call(ptr(m), 0, 0, 0)
	return int32(r)
}
func TranslateMessage(m *MSG) { pTranslateMessage.Call(ptr(m)) }
func DispatchMessage(m *MSG)  { pDispatchMessageW.Call(ptr(m)) }

func BeginPaint(hwnd uintptr, ps *PAINTSTRUCT) uintptr {
	r, _, _ := pBeginPaint.Call(hwnd, ptr(ps))
	return r
}
func EndPaint(hwnd uintptr, ps *PAINTSTRUCT) { pEndPaint.Call(hwnd, ptr(ps)) }

func CreateSolidBrush(c uint32) uintptr { r, _, _ := pCreateSolidBrush.Call(uintptr(c)); return r }
func CreatePen(width int, c uint32) uintptr {
	r, _, _ := pCreatePen.Call(PS_SOLID, uintptr(width), uintptr(c))
	return r
}
func FillRect(hdc uintptr, rc RECT, brush uintptr) { pFillRect.Call(hdc, ptr(&rc), brush) }

// FillColor fills a rectangle with a solid colour.
func FillColor(hdc uintptr, rc RECT, c uint32) {
	b := CreateSolidBrush(c)
	FillRect(hdc, rc, b)
	DeleteObject(b)
}

// RoundRectColor draws a filled rounded rectangle without outline.
func RoundRectColor(hdc uintptr, rc RECT, radius int, fill uint32) {
	b := CreateSolidBrush(fill)
	p := CreatePen(1, fill)
	ob := SelectObject(hdc, b)
	op := SelectObject(hdc, p)
	pRoundRect.Call(hdc, uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right), uintptr(rc.Bottom), uintptr(radius), uintptr(radius))
	SelectObject(hdc, ob)
	SelectObject(hdc, op)
	DeleteObject(b)
	DeleteObject(p)
}

// RoundRectOutline draws a rounded rectangle outline.
func RoundRectOutline(hdc uintptr, rc RECT, radius int, c uint32, width int) {
	p := CreatePen(width, c)
	op := SelectObject(hdc, p)
	ob := SelectObject(hdc, GetStockObject(NULL_BRUSH))
	pRoundRect.Call(hdc, uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right), uintptr(rc.Bottom), uintptr(radius), uintptr(radius))
	SelectObject(hdc, ob)
	SelectObject(hdc, op)
	DeleteObject(p)
}

// EllipseColor draws a filled ellipse without outline.
func EllipseColor(hdc uintptr, rc RECT, fill uint32) {
	b := CreateSolidBrush(fill)
	p := CreatePen(1, fill)
	ob := SelectObject(hdc, b)
	op := SelectObject(hdc, p)
	pEllipse.Call(hdc, uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right), uintptr(rc.Bottom))
	SelectObject(hdc, ob)
	SelectObject(hdc, op)
	DeleteObject(b)
	DeleteObject(p)
}

func CreateFont(px int, weight int, face string) uintptr {
	r, _, _ := pCreateFontW.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0, DEFAULT_CHARSET, 0, 0, CLEARTYPE_QUALITY, 0,
		uintptr(unsafe.Pointer(U16(face))))
	return r
}
func SetTextColor(hdc uintptr, c uint32) { pSetTextColor.Call(hdc, uintptr(c)) }
func SetBkTransparent(hdc uintptr)       { pSetBkMode.Call(hdc, TRANSPARENT) }
func SetBkColor(hdc uintptr, c uint32)   { pSetBkColor.Call(hdc, uintptr(c)) }

// FontAvailable reports whether Windows really provides the font face
// (CreateFont silently substitutes missing fonts).
func FontAvailable(face string) bool {
	hdc := GetDC(0)
	defer ReleaseDC(0, hdc)
	f := CreateFont(16, FW_NORMAL, face)
	old := SelectObject(hdc, f)
	buf := make([]uint16, 64)
	pGetTextFaceW.Call(hdc, 64, uintptr(unsafe.Pointer(&buf[0])))
	SelectObject(hdc, old)
	DeleteObject(f)
	return syscall.UTF16ToString(buf) == face
}

// DrawText draws (or with DT_CALCRECT measures) text; returns the height.
func DrawText(hdc uintptr, s string, rc *RECT, format uint32) int {
	u, _ := syscall.UTF16FromString(s)
	r, _, _ := pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), ptr(rc), uintptr(format))
	return int(int32(r))
}

func SetWindowsHookEx(id int, proc uintptr) uintptr {
	r, _, _ := pSetWindowsHookExW.Call(uintptr(id), proc, GetModuleHandle(), 0)
	return r
}
func UnhookWindowsHookEx(h uintptr) { pUnhookWindowsHookEx.Call(h) }
func CallNextHookEx(code int, wp, lp uintptr) uintptr {
	r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wp, lp)
	return r
}

func CreatePopupMenu() uintptr { r, _, _ := pCreatePopupMenu.Call(); return r }
func AppendMenu(m uintptr, flags uint32, id uintptr, text string) {
	var t uintptr
	if text != "" {
		t = uintptr(unsafe.Pointer(U16(text)))
	}
	pAppendMenuW.Call(m, uintptr(flags), id, t)
}
func TrackPopupMenu(m uintptr, x, y int, hwnd uintptr) int {
	r, _, _ := pTrackPopupMenu.Call(m, TPM_RETURNCMD|TPM_RIGHTBTN, uintptr(x), uintptr(y), 0, hwnd, 0)
	return int(r)
}
func DestroyMenu(m uintptr) { pDestroyMenu.Call(m) }

// CreateIconRGBA creates an HICON from straight-alpha RGBA pixels.
func CreateIconRGBA(w, h int, rgba []byte) uintptr {
	hdc := GetDC(0)
	hbm, bits := CreateDIB(hdc, w, h)
	ReleaseDC(0, hdc)
	if hbm == 0 {
		return 0
	}
	for i := 0; i+3 < len(rgba) && i+3 < len(bits); i += 4 {
		bits[i], bits[i+1], bits[i+2], bits[i+3] = rgba[i+2], rgba[i+1], rgba[i], rgba[i+3]
	}
	maskBits := make([]byte, ((w+15)/16)*2*h)
	mask, _, _ := pCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	ii := ICONINFO{FIcon: 1, HbmMask: mask, HbmColor: hbm}
	icon, _, _ := pCreateIconIndirect.Call(ptr(&ii))
	DeleteObject(hbm)
	DeleteObject(mask)
	return icon
}
func DestroyIcon(h uintptr) { pDestroyIcon.Call(h) }

func MessageBox(title, text string, flags uint32) int {
	r, _, _ := pMessageBoxW.Call(0, uintptr(unsafe.Pointer(U16(text))), uintptr(unsafe.Pointer(U16(title))), uintptr(flags))
	return int(r)
}

func ShellNotifyIcon(msg uint32, nid *NOTIFYICONDATAW) bool {
	r, _, _ := pShellNotifyIconW.Call(uintptr(msg), ptr(nid))
	return r != 0
}

// ShellExecute opens a file, folder, URL or program like a double click.
func ShellExecute(verb, file, args, dir string) error {
	var a, d, v uintptr
	if args != "" {
		a = uintptr(unsafe.Pointer(U16(args)))
	}
	if dir != "" {
		d = uintptr(unsafe.Pointer(U16(dir)))
	}
	if verb != "" {
		v = uintptr(unsafe.Pointer(U16(verb)))
	}
	r, _, _ := pShellExecuteW.Call(0, v, uintptr(unsafe.Pointer(U16(file))), a, d, SW_SHOWNORMAL)
	if r <= 32 {
		return syscall.Errno(r)
	}
	return nil
}

// RunElevated starts exe with a UAC prompt and optionally waits for it.
func RunElevated(exe, args string, wait bool) (uint32, error) {
	info := SHELLEXECUTEINFOW{
		FMask:        SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC,
		LpVerb:       U16("runas"),
		LpFile:       U16(exe),
		LpParameters: U16(args),
		NShow:        SW_SHOWNORMAL,
	}
	info.CbSize = uint32(unsafe.Sizeof(info))
	r, _, err := pShellExecuteExW.Call(ptr(&info))
	if r == 0 {
		return 1, err
	}
	if info.HProcess == 0 {
		return 0, nil
	}
	defer syscall.CloseHandle(syscall.Handle(info.HProcess))
	if !wait {
		return 0, nil
	}
	syscall.WaitForSingleObject(syscall.Handle(info.HProcess), syscall.INFINITE)
	var code uint32
	syscall.GetExitCodeProcess(syscall.Handle(info.HProcess), &code)
	return code, nil
}

func KnownFolder(id *GUID) string {
	var p uintptr
	r, _, _ := pSHGetKnownFolderPath.Call(uintptr(unsafe.Pointer(id)), 0, 0, ptr(&p))
	if r != 0 || p == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(p)
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), 32768))
}

func IsAdmin() bool {
	if pIsUserAnAdmin.Find() != nil {
		return false
	}
	r, _, _ := pIsUserAnAdmin.Call()
	return r != 0
}

func SetWindowRounded(hwnd uintptr) {
	if pDwmSetWindowAttribute.Find() != nil {
		return
	}
	v := uint32(DWMWCP_ROUND)
	pDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE, ptr(&v), 4)
}

// EnableDPIAwareness makes coordinates real pixels (important for capture).
func EnableDPIAwareness() {
	if pSetProcessDpiAwarenessContext.Find() == nil {
		// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4
		if r, _, _ := pSetProcessDpiAwarenessContext.Call(^uintptr(3)); r != 0 {
			return
		}
	}
	if pSetProcessDpiAwareness.Find() == nil {
		if r, _, _ := pSetProcessDpiAwareness.Call(2); r == 0 {
			return
		}
	}
	pSetProcessDPIAware.Call()
}

// DPIScale returns the system DPI scale factor (1.0 = 96 dpi).
func DPIScale() float64 {
	if pGetDpiForSystem.Find() == nil {
		if r, _, _ := pGetDpiForSystem.Call(); r > 0 {
			return float64(r) / 96
		}
	}
	hdc := GetDC(0)
	defer ReleaseDC(0, hdc)
	if d := GetDeviceCaps(hdc, LOGPIXELSX); d > 0 {
		return float64(d) / 96
	}
	return 1
}

// MonitorEntry describes one display.
type MonitorEntry struct {
	R       RECT
	Work    RECT
	Primary bool
}

var (
	monMu   sync.Mutex
	monList []MonitorEntry
	// Go limits how many callbacks may be created, so create it only once.
	monCB = syscall.NewCallback(func(hm, hdc, lprc, data uintptr) uintptr {
		mi := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
		pGetMonitorInfoW.Call(hm, ptr(&mi))
		monList = append(monList, MonitorEntry{mi.RcMonitor, mi.RcWork, mi.DwFlags&1 != 0})
		return 1
	})
)

// Monitors enumerates all displays.
func Monitors() []MonitorEntry {
	monMu.Lock()
	defer monMu.Unlock()
	monList = nil
	pEnumDisplayMonitors.Call(0, 0, monCB, 0)
	out := monList
	monList = nil
	return out
}

func UILanguage() uint16 {
	r, _, _ := pGetUserDefaultUILanguage.Call()
	return uint16(r)
}

func OSVersion() (major, minor, build uint32) {
	v := OSVERSIONINFOW{Size: uint32(unsafe.Sizeof(OSVERSIONINFOW{}))}
	pRtlGetVersion.Call(ptr(&v))
	return v.Major, v.Minor, v.Build
}

// CreateMutex returns false if a mutex with that name already exists.
func CreateMutex(name string) bool {
	_, _, err := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(U16(name))))
	return err != syscall.Errno(183) // ERROR_ALREADY_EXISTS
}

func RegisterHotKey(hwnd uintptr, id int, mods, vk uint32) bool {
	r, _, _ := pRegisterHotKey.Call(hwnd, uintptr(id), uintptr(mods), uintptr(vk))
	return r != 0
}

func GetWorkArea() RECT {
	var r RECT
	pSystemParametersInfoW.Call(0x0030, 0, ptr(&r), 0) // SPI_GETWORKAREA
	return r
}
