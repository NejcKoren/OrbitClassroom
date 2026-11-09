//go:build windows

package master

import (
	"orbit/internal/sys"
	"orbit/internal/win"
	"os"
	"os/exec"
	"path/filepath"
)

// browsers in order of preference (Edge is part of every Windows 10/11).
func browserCandidates() []string {
	pf86 := os.Getenv("ProgramFiles(x86)")
	pf := os.Getenv("ProgramFiles")
	la := os.Getenv("LOCALAPPDATA")
	return []string{
		filepath.Join(pf86, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(la, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pf86, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(la, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pf, `BraveSoftware\Brave-Browser\Application\brave.exe`),
	}
}

// OpenWindow opens the user interface as a standalone app window
// (Edge/Chrome "app mode" with a private profile). Falls back to the default browser.
func OpenWindow(url, profileDir string) (*os.Process, error) {
	for _, b := range browserCandidates() {
		if _, err := os.Stat(b); err != nil {
			continue
		}
		cmd := exec.Command(b, "--app="+url, "--user-data-dir="+profileDir, "--no-first-run", "--no-default-browser-check",
			"--disable-background-mode", "--disable-sync", "--hide-crash-restore-bubble", "--window-size=1480,920",
			"--disable-features=Translate,msEdgeSidebarV2,msHubApps")
		if err := cmd.Start(); err == nil {
			return cmd.Process, nil
		}
	}
	return nil, win.ShellExecute("open", url, "", "")
}

// OpenFolder shows a folder in Explorer.
func OpenFolder(dir string) error { return win.ShellExecute("open", dir, "", "") }

// PicturesDir is the user's Pictures folder.
func PicturesDir() string { return sys.PicturesDir() }

func systemLanguage() string { return sys.UILanguage() }

// ShowError shows a native error dialog.
func ShowError(title, text string) { sys.MessageBox(title, text, "error") }
