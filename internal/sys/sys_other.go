//go:build !windows

package sys

// Headless development implementation (Linux/macOS). It logs what a real
// student computer would do, so master+agent can be tested end-to-end
// without Windows. Set ORBIT_DEBUG_DUMP=dir to save demo frames as PNG.

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"orbit/internal/proto"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

func HiddenCmd(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) }

func UserName() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return os.Getenv("USER")
}

func OSName() string { return runtime.GOOS + " (dev)" }

func IdleSeconds() int               { return int(time.Now().Unix() % 97) }
func ActiveWindow() (string, string) { return "dev-shell", "Orbit development agent" }
func UILanguage() string {
	if l := os.Getenv("ORBIT_LANG"); l != "" {
		return l
	}
	return "en"
}

func Power(mode string, delay int, msg string) error {
	log.Printf("[sys] power %s delay=%d msg=%q (simulated)", mode, delay, msg)
	return nil
}

func ShellOpen(target, args string) error {
	log.Printf("[sys] open %q %q (simulated)", target, args)
	return nil
}

func KillProcesses(list []string) []string { return nil }

func DesktopDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Desktop")
}

func PicturesDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Pictures")
}

func IsAdmin() bool                             { return os.Geteuid() == 0 }
func RunElevated(exe, args string) (int, error) { return 1, fmt.Errorf("elevation only on Windows") }
func SingleInstance(name string) bool           { return true }
func MessageBox(title, text, kind string) bool  { log.Printf("[ui] %s: %s", title, text); return false }

func SendInput(evs []proto.InputEvent, area image.Rectangle) error {
	if len(evs) > 0 {
		log.Printf("[sys] input: %d events (first %s)", len(evs), evs[0].T)
	}
	return nil
}

// ---- UI stand-ins ----

var (
	uiMu      sync.Mutex
	demoStop  chan struct{}
	handlerUI UIHandler
)

func StartUI(h UIHandler) error { handlerUI = h; return nil }
func EnableTray()               {}
func RemoveTray()               {}
func SetTray(s TrayState)       {}
func Notify(title, text string) { log.Printf("[ui] notify: %s - %s", title, text) }
func RegisterHandHotkey()       {}
func Beep()                     {}
func SetWaitingText(s string)   {}
func ShowLock(mode, title, text string) {
	log.Printf("[ui] LOCK mode=%s title=%q text=%q", mode, title, text)
}
func HideLock() { log.Printf("[ui] UNLOCK") }

func ShowDemo(mode, title string, c *proto.Canvas) {
	log.Printf("[ui] DEMO show mode=%s", mode)
	dir := os.Getenv("ORBIT_DEBUG_DUMP")
	if dir == "" {
		return
	}
	uiMu.Lock()
	if demoStop != nil {
		close(demoStop)
	}
	stop := make(chan struct{})
	demoStop = stop
	uiMu.Unlock()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				c.Lock()
				ready := c.Ready
				c.Unlock()
				if !ready {
					continue
				}
				img := c.RGBA()
				tmp := filepath.Join(dir, "demo-frame.tmp")
				f, err := os.Create(tmp)
				if err == nil {
					if err := png.Encode(f, img); err != nil {
						log.Printf("[ui] dump failed: %v (%dx%d)", err, img.Rect.Dx(), img.Rect.Dy())
					}
					f.Close()
					os.Rename(tmp, filepath.Join(dir, "demo-frame.png"))
				}
			}
		}
	}()
}
func DemoUpdated() {}
func HideDemo() {
	log.Printf("[ui] DEMO hide")
	uiMu.Lock()
	if demoStop != nil {
		close(demoStop)
		demoStop = nil
	}
	uiMu.Unlock()
}

// ShowCard logs the card. With ORBIT_AUTO_ANSWER=n set, it answers with button n after 1s (tests).
func ShowCard(c Card, cb func(int, string)) func() {
	log.Printf("[ui] CARD %q: %q buttons=%v", c.Title, c.Body, c.Buttons)
	var once sync.Once
	cancel := func() { once.Do(func() {}) }
	if v := os.Getenv("ORBIT_AUTO_ANSWER"); v != "" && cb != nil {
		var n int
		fmt.Sscan(v, &n)
		go func() {
			time.Sleep(time.Second)
			once.Do(func() {
				if n >= len(c.Buttons) {
					n = len(c.Buttons) - 1
				}
				cb(n, "auto answer from dev agent")
			})
		}()
	}
	return cancel
}
