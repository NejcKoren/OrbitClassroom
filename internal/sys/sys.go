// Package sys contains the operating-system specific parts of the agent:
// input injection, power actions, process control, system information and
// the native user interface (lock screen, demo window, message cards, tray).
//
// sys_windows.go / ui_windows.go hold the real implementation. sys_other.go
// is a headless stand-in used for development and automated tests on Linux.
package sys

import (
	"path/filepath"
	"strings"
)

// Card describes a small native dialog shown to the student
// (teacher message, poll, setup prompt, "message to teacher").
type Card struct {
	Title       string
	Body        string
	Buttons     []string
	Primary     int  // index of the highlighted button
	Cancel      int  // button index returned on Esc / close (-1 = none)
	Vertical    bool // stack buttons (polls)
	Input       bool // show a text box
	Multiline   bool
	InputText   string
	Glyph       rune // Segoe MDL2 Assets glyph shown in the header
	Accent      uint32
	Topmost     bool
	Beep        bool
	MinWidthDIP int
}

// Glyphs from the "Segoe MDL2 Assets" font (available on Windows 10/11).
const (
	GlyphMessage = ''
	GlyphPoll    = ''
	GlyphLock    = ''
	GlyphInfo    = ''
	GlyphPerson  = ''
	GlyphShield  = ''
	GlyphKey     = ''
	GlyphSend    = ''
)

// MenuItem is one entry of the tray menu.
type MenuItem struct {
	ID        string
	Label     string
	Checked   bool
	Disabled  bool
	Separator bool
}

// TrayState controls the tray icon badge and tooltip.
type TrayState struct {
	Hand    bool
	Live    bool // screen is being viewed or controlled
	Locked  bool
	Tooltip string
}

// UIHandler receives events from the native UI.
type UIHandler interface {
	TrayMenu() []MenuItem
	TrayCommand(id string)
	Hotkey(id int)
}

// ExeBase returns the lower-case executable name of a path ("chrome.exe").
func ExeBase(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	return strings.ToLower(filepath.Base(p))
}

// MatchProcess reports whether exe (e.g. "Chrome.exe") matches a block list
// entry ("chrome" or "chrome.exe", case-insensitive).
func MatchProcess(exe string, list []string) bool {
	e := strings.ToLower(exe)
	eb := strings.TrimSuffix(e, ".exe")
	for _, l := range list {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		if l == e || l == eb || strings.TrimSuffix(l, ".exe") == eb {
			return true
		}
	}
	return false
}
