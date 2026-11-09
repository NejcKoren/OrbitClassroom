//go:build windows

package sys

import (
	"errors"
	"fmt"
	"image"
	"orbit/internal/proto"
	"orbit/internal/win"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// HiddenCmd builds an exec.Cmd that never flashes a console window.
func HiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return c
}

// UserName returns the logged on user (DOMAIN\user -> user).
func UserName() string {
	u, err := user.Current()
	if err != nil {
		return os.Getenv("USERNAME")
	}
	name := u.Username
	if i := strings.LastIndex(name, `\`); i >= 0 {
		name = name[i+1:]
	}
	full := u.Name // display name, e.g. "Janez Novak"
	if i := strings.LastIndex(full, `\`); i >= 0 {
		full = full[i+1:]
	}
	if full != "" && !strings.EqualFold(full, name) {
		return name + " (" + full + ")"
	}
	return name
}

// OSName returns e.g. "Windows 11 (22631)".
func OSName() string {
	maj, min, build := win.OSVersion()
	name := fmt.Sprintf("Windows %d.%d", maj, min)
	if maj == 10 {
		name = "Windows 10"
		if build >= 22000 {
			name = "Windows 11"
		}
	}
	return fmt.Sprintf("%s (%d)", name, build)
}

// IdleSeconds since the last keyboard/mouse input.
func IdleSeconds() int { return int(win.IdleMillis() / 1000) }

// ActiveWindow returns the exe name and title of the foreground window.
func ActiveWindow() (string, string) {
	h := win.GetForegroundWindow()
	if h == 0 {
		return "", ""
	}
	title := win.GetWindowText(h)
	exe := win.ProcessImage(win.GetWindowPID(h))
	return ExeBase(exe), title
}

// UILanguage maps the Windows display language to a language code.
func UILanguage() string {
	switch win.UILanguage() & 0x3FF {
	case 0x24:
		return "sl"
	case 0x07:
		return "de"
	case 0x1a:
		return "hr"
	case 0x10:
		return "it"
	case 0x0c:
		return "fr"
	case 0x0a:
		return "es"
	}
	return "en"
}

// Power performs shutdown / reboot / logoff / abort.
func Power(mode string, delay int, msg string) error {
	var args []string
	switch mode {
	case "shutdown":
		args = []string{"/s", "/t", fmt.Sprint(delay)}
	case "reboot":
		args = []string{"/r", "/t", fmt.Sprint(delay)}
	case "logoff":
		args = []string{"/l"}
	case "abort":
		args = []string{"/a"}
	default:
		return errors.New("unknown power mode")
	}
	if (mode == "shutdown" || mode == "reboot") && delay == 0 {
		args = append(args, "/f")
	}
	if msg != "" && mode != "logoff" && mode != "abort" {
		if len(msg) > 500 {
			msg = msg[:500]
		}
		args = append(args, "/c", msg)
	}
	return HiddenCmd("shutdown.exe", args...).Run()
}

// ShellOpen opens a URL, document or program like a double click.
func ShellOpen(target, args string) error {
	return win.ShellExecute("open", target, args, "")
}

// KillProcesses terminates processes whose exe name matches list.
// Returns the names that were closed.
func KillProcesses(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snap)
	self := uint32(os.Getpid())
	var killed []string
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		name := syscall.UTF16ToString(pe.ExeFile[:])
		if pe.ProcessID == self || !MatchProcess(name, list) {
			continue
		}
		h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, pe.ProcessID)
		if err != nil {
			continue
		}
		if syscall.TerminateProcess(h, 1) == nil {
			killed = append(killed, name)
		}
		syscall.CloseHandle(h)
	}
	return killed
}

// DesktopDir returns the user's real desktop folder (handles OneDrive redirection).
func DesktopDir() string {
	if p := win.KnownFolder(&win.FOLDERID_Desktop); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return h + `\Desktop`
}

// PicturesDir returns the user's Pictures folder.
func PicturesDir() string {
	if p := win.KnownFolder(&win.FOLDERID_Pictures); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return h + `\Pictures`
}

func IsAdmin() bool { return win.IsAdmin() }

// RunElevated restarts exe with admin rights (UAC prompt) and waits.
func RunElevated(exe, args string) (int, error) {
	code, err := win.RunElevated(exe, args, true)
	return int(code), err
}

// SingleInstance returns false if another instance with that name runs in this session.
func SingleInstance(name string) bool { return win.CreateMutex(`Local\` + name) }

// MessageBox shows a simple native message box. kind: info, warn, error, yesno.
func MessageBox(title, text, kind string) bool {
	flags := uint32(win.MB_TOPMOST | win.MB_SETFOREGROUND)
	switch kind {
	case "warn":
		flags |= win.MB_ICONWARNING
	case "error":
		flags |= win.MB_ICONERROR
	case "yesno":
		flags |= win.MB_YESNO | win.MB_ICONINFORMATION
	default:
		flags |= win.MB_ICONINFORMATION
	}
	return win.MessageBox(title, text, flags) == win.IDYES
}

// ---------------------------------------------------------------- input

type sc struct {
	code uint16
	ext  bool
}

// scancodes maps KeyboardEvent.code to PC set-1 scan codes. Using scan codes
// (physical keys) means the student's own keyboard layout is applied.
var scancodes = map[string]sc{
	"Escape": {0x01, false}, "Digit1": {0x02, false}, "Digit2": {0x03, false}, "Digit3": {0x04, false},
	"Digit4": {0x05, false}, "Digit5": {0x06, false}, "Digit6": {0x07, false}, "Digit7": {0x08, false},
	"Digit8": {0x09, false}, "Digit9": {0x0A, false}, "Digit0": {0x0B, false}, "Minus": {0x0C, false},
	"Equal": {0x0D, false}, "Backspace": {0x0E, false}, "Tab": {0x0F, false}, "KeyQ": {0x10, false},
	"KeyW": {0x11, false}, "KeyE": {0x12, false}, "KeyR": {0x13, false}, "KeyT": {0x14, false},
	"KeyY": {0x15, false}, "KeyU": {0x16, false}, "KeyI": {0x17, false}, "KeyO": {0x18, false},
	"KeyP": {0x19, false}, "BracketLeft": {0x1A, false}, "BracketRight": {0x1B, false}, "Enter": {0x1C, false},
	"ControlLeft": {0x1D, false}, "KeyA": {0x1E, false}, "KeyS": {0x1F, false}, "KeyD": {0x20, false},
	"KeyF": {0x21, false}, "KeyG": {0x22, false}, "KeyH": {0x23, false}, "KeyJ": {0x24, false},
	"KeyK": {0x25, false}, "KeyL": {0x26, false}, "Semicolon": {0x27, false}, "Quote": {0x28, false},
	"Backquote": {0x29, false}, "ShiftLeft": {0x2A, false}, "Backslash": {0x2B, false}, "KeyZ": {0x2C, false},
	"KeyX": {0x2D, false}, "KeyC": {0x2E, false}, "KeyV": {0x2F, false}, "KeyB": {0x30, false},
	"KeyN": {0x31, false}, "KeyM": {0x32, false}, "Comma": {0x33, false}, "Period": {0x34, false},
	"Slash": {0x35, false}, "ShiftRight": {0x36, false}, "NumpadMultiply": {0x37, false}, "AltLeft": {0x38, false},
	"Space": {0x39, false}, "CapsLock": {0x3A, false}, "F1": {0x3B, false}, "F2": {0x3C, false},
	"F3": {0x3D, false}, "F4": {0x3E, false}, "F5": {0x3F, false}, "F6": {0x40, false},
	"F7": {0x41, false}, "F8": {0x42, false}, "F9": {0x43, false}, "F10": {0x44, false},
	"NumLock": {0x45, true}, "ScrollLock": {0x46, false}, "Numpad7": {0x47, false}, "Numpad8": {0x48, false},
	"Numpad9": {0x49, false}, "NumpadSubtract": {0x4A, false}, "Numpad4": {0x4B, false}, "Numpad5": {0x4C, false},
	"Numpad6": {0x4D, false}, "NumpadAdd": {0x4E, false}, "Numpad1": {0x4F, false}, "Numpad2": {0x50, false},
	"Numpad3": {0x51, false}, "Numpad0": {0x52, false}, "NumpadDecimal": {0x53, false}, "IntlBackslash": {0x56, false},
	"F11": {0x57, false}, "F12": {0x58, false}, "IntlRo": {0x73, false}, "IntlYen": {0x7D, false},
	"NumpadEnter": {0x1C, true}, "ControlRight": {0x1D, true}, "NumpadDivide": {0x35, true},
	"PrintScreen": {0x37, true}, "AltRight": {0x38, true}, "Home": {0x47, true}, "ArrowUp": {0x48, true},
	"PageUp": {0x49, true}, "ArrowLeft": {0x4B, true}, "ArrowRight": {0x4D, true}, "End": {0x4F, true},
	"ArrowDown": {0x50, true}, "PageDown": {0x51, true}, "Insert": {0x52, true}, "Delete": {0x53, true},
	"MetaLeft": {0x5B, true}, "MetaRight": {0x5C, true}, "ContextMenu": {0x5D, true},
}

func keyInput(code string, up bool) (win.INPUT, bool) {
	s, ok := scancodes[code]
	if !ok {
		return win.INPUT{}, false
	}
	flags := uint32(win.KEYEVENTF_SCANCODE)
	if s.ext {
		flags |= win.KEYEVENTF_EXTENDEDKEY
	}
	if up {
		flags |= win.KEYEVENTF_KEYUP
	}
	// Windows uses the scan code (layout independent); the virtual-key code is
	// filled in as well for applications/compat layers that look at it.
	return win.KeyInput(win.KEYBDINPUT{WVk: win.ScanToVK(s.code, s.ext), WScan: s.code, DwFlags: flags}), true
}

func textInputs(s string) []win.INPUT {
	var out []win.INPUT
	for _, u := range utf16.Encode([]rune(s)) {
		if u == '\n' {
			if in, ok := keyInput("Enter", false); ok {
				out = append(out, in)
			}
			if in, ok := keyInput("Enter", true); ok {
				out = append(out, in)
			}
			continue
		}
		out = append(out, win.KeyInput(win.KEYBDINPUT{WScan: u, DwFlags: win.KEYEVENTF_UNICODE}))
		out = append(out, win.KeyInput(win.KEYBDINPUT{WScan: u, DwFlags: win.KEYEVENTF_UNICODE | win.KEYEVENTF_KEYUP}))
	}
	return out
}

func combo(keys ...string) []win.INPUT {
	var out []win.INPUT
	for _, k := range keys {
		if in, ok := keyInput(k, false); ok {
			out = append(out, in)
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if in, ok := keyInput(keys[i], true); ok {
			out = append(out, in)
		}
	}
	return out
}

// SendInput injects remote-control events. Coordinates are normalised to area.
func SendInput(evs []proto.InputEvent, area image.Rectangle) error {
	vx := win.GetSystemMetrics(win.SM_XVIRTUALSCREEN)
	vy := win.GetSystemMetrics(win.SM_YVIRTUALSCREEN)
	vw := max(2, win.GetSystemMetrics(win.SM_CXVIRTUALSCREEN))
	vh := max(2, win.GetSystemMetrics(win.SM_CYVIRTUALSCREEN))
	abs := func(x, y float64) (int32, int32) {
		x = min(max(x, 0), 1)
		y = min(max(y, 0), 1)
		px := float64(area.Min.X) + x*float64(area.Dx()-1)
		py := float64(area.Min.Y) + y*float64(area.Dy()-1)
		return int32((px - float64(vx)) * 65535 / float64(vw-1)), int32((py - float64(vy)) * 65535 / float64(vh-1))
	}
	var ins []win.INPUT
	mouse := func(e proto.InputEvent, flags uint32, data int32) {
		ax, ay := abs(e.X, e.Y)
		ins = append(ins, win.INPUT{Type: win.INPUT_MOUSE, Mi: win.MOUSEINPUT{Dx: ax, Dy: ay, MouseData: uint32(data),
			DwFlags: flags | win.MOUSEEVENTF_MOVE | win.MOUSEEVENTF_ABSOLUTE | win.MOUSEEVENTF_VIRTUALDSK}})
	}
	for _, e := range evs {
		switch e.T {
		case "mm":
			mouse(e, 0, 0)
		case "md", "mu":
			var f uint32
			switch e.B {
			case 0:
				f = win.MOUSEEVENTF_LEFTDOWN
			case 1:
				f = win.MOUSEEVENTF_MIDDLEDOWN
			case 2:
				f = win.MOUSEEVENTF_RIGHTDOWN
			}
			if e.T == "mu" {
				f <<= 1 // the *UP flag is always the next bit
			}
			mouse(e, f, 0)
		case "wh":
			mouse(e, win.MOUSEEVENTF_WHEEL, int32(e.D))
		case "hw":
			mouse(e, win.MOUSEEVENTF_HWHEEL, int32(e.D))
		case "kd", "ku":
			if in, ok := keyInput(e.C, e.T == "ku"); ok {
				ins = append(ins, in)
			}
		case "tx":
			ins = append(ins, textInputs(e.S)...)
		case "combo":
			switch e.S {
			case "win":
				ins = append(ins, combo("MetaLeft")...)
			case "alttab":
				ins = append(ins, combo("AltLeft", "Tab")...)
			case "ctrlesc":
				ins = append(ins, combo("ControlLeft", "Escape")...)
			case "ctrlshiftesc":
				ins = append(ins, combo("ControlLeft", "ShiftLeft", "Escape")...)
			case "winr":
				ins = append(ins, combo("MetaLeft", "KeyR")...)
			case "wind":
				ins = append(ins, combo("MetaLeft", "KeyD")...)
			}
		}
	}
	if len(ins) > 0 && win.SendInput(ins) == 0 {
		return errors.New("input was blocked by Windows (UAC prompt or elevated window?)")
	}
	return nil
}
