//go:build !windows

package master

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OpenWindow prints the address; with ORBIT_OPEN=1 it also opens a browser.
func OpenWindow(url, profileDir string) (*os.Process, error) {
	fmt.Println("Orbit Master UI:", url)
	if os.Getenv("ORBIT_OPEN") == "1" {
		c := exec.Command("xdg-open", url)
		if err := c.Start(); err == nil {
			return c.Process, nil
		}
	}
	return nil, nil
}

func OpenFolder(dir string) error { return exec.Command("xdg-open", dir).Start() }

func PicturesDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Pictures")
}

func systemLanguage() string {
	l := os.Getenv("ORBIT_LANG")
	if l == "" {
		l = strings.ToLower(os.Getenv("LANG"))
	}
	if strings.HasPrefix(l, "sl") {
		return "sl"
	}
	return "en"
}

func ShowError(title, text string) { fmt.Fprintln(os.Stderr, title+": "+text) }
