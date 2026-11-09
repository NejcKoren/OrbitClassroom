package agent

import (
	"errors"
	"fmt"
	"io"
	"log"
	"orbit/internal/proto"
	"orbit/internal/sys"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Installation layout on Windows:
//
//	C:\Program Files\Orbit Classroom\OrbitAgent.exe     program
//	C:\ProgramData\Orbit Classroom\agent.json           config (read-only for students)
//	C:\ProgramData\Orbit Classroom\agent-cert.pem/key   TLS certificate
//	HKLM\...\CurrentVersion\Run\OrbitAgent              autostart for every user
//	Firewall rule "Orbit Agent"                         inbound for the program
//	HKLM\...\Uninstall\OrbitAgent                       "Apps & features" entry

const runKey = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
const uninstKey = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\OrbitAgent`

// InstallDir is where the agent program is installed.
func InstallDir() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "Orbit Classroom")
}

// InstalledExe is the path of the installed agent.
func InstalledExe() string { return filepath.Join(InstallDir(), "OrbitAgent.exe") }

// IsInstalled reports whether an installed agent exists.
func IsInstalled() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	if _, err := os.Stat(InstalledExe()); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(ProgramDataDir(), "agent.json"))
	return err == nil
}

func run(name string, args ...string) error {
	out, err := sys.HiddenCmd(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	out.Close()
	os.Remove(dst + ".old")
	os.Rename(dst, dst+".old") // a running exe can be renamed but not overwritten
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	os.Remove(dst + ".old")
	return nil
}

// stopOthers stops running agents except this process and keepPID.
func stopOthers(keepPID int) {
	args := []string{"/F", "/FI", fmt.Sprintf("PID ne %d", os.Getpid())}
	if keepPID > 0 {
		args = append(args, "/FI", fmt.Sprintf("PID ne %d", keepPID))
	}
	run("taskkill", append(args, "/IM", "OrbitAgent.exe")...)
}

// Install installs the agent machine-wide. Must run as administrator.
func Install(cfg *Config, keepPID int) error {
	if runtime.GOOS != "windows" {
		return errors.New("installation is only supported on Windows")
	}
	if !sys.IsAdmin() {
		return errors.New("administrator rights required")
	}
	if len(cfg.TrustedKeys) == 0 {
		return errors.New("no classroom key")
	}
	for _, k := range cfg.TrustedKeys {
		if _, err := proto.ParsePublicKey(k); err != nil {
			return fmt.Errorf("invalid key: %v", err)
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	// stop other running agents so the exe can be replaced
	stopOthers(keepPID)

	if err := os.MkdirAll(InstallDir(), 0o755); err != nil {
		return err
	}
	exe := InstalledExe()
	if !strings.EqualFold(filepath.Clean(self), filepath.Clean(exe)) {
		if err := copyFile(self, exe); err != nil {
			return fmt.Errorf("copy program: %w", err)
		}
	}
	data := ProgramDataDir()
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	// merge with an existing configuration (keep previously trusted teachers)
	p := filepath.Join(data, "agent.json")
	if old, err := ReadConfig(p); err == nil {
		seen := map[string]bool{}
		for _, k := range cfg.TrustedKeys {
			seen[k] = true
		}
		for _, k := range old.TrustedKeys {
			if !seen[k] {
				cfg.TrustedKeys = append(cfg.TrustedKeys, k)
			}
		}
	}
	if err := cfg.Save(p); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if _, err := os.Stat(filepath.Join(data, "agent-cert.pem")); err != nil {
		if err := CreateCert(data); err != nil {
			return fmt.Errorf("create certificate: %w", err)
		}
	}
	// Administrators + SYSTEM full control, Users read only.
	if err := run("icacls", data, "/inheritance:r", "/grant:r", "*S-1-5-32-544:(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-545:(OI)(CI)RX"); err != nil {
		log.Printf("warning: %v", err)
	}
	if err := run("reg", "add", runKey, "/v", "OrbitAgent", "/t", "REG_SZ", "/d", `"`+exe+`" --autostart`, "/f"); err != nil {
		return err
	}
	run("netsh", "advfirewall", "firewall", "delete", "rule", "name=Orbit Agent")
	if err := run("netsh", "advfirewall", "firewall", "add", "rule", "name=Orbit Agent", "dir=in", "action=allow",
		"program="+exe, "enable=yes", "profile=any", "description=Orbit Classroom student agent"); err != nil {
		log.Printf("warning: firewall rule: %v", err)
	}
	for _, kv := range [][3]string{
		{"DisplayName", "REG_SZ", "Orbit Agent (Orbit Classroom)"},
		{"DisplayVersion", "REG_SZ", proto.Version},
		{"Publisher", "REG_SZ", "Orbit Classroom"},
		{"DisplayIcon", "REG_SZ", exe},
		{"InstallLocation", "REG_SZ", InstallDir()},
		{"UninstallString", "REG_SZ", `"` + exe + `" --uninstall`},
		{"QuietUninstallString", "REG_SZ", `"` + exe + `" --uninstall --silent`},
		{"NoModify", "REG_DWORD", "1"},
		{"NoRepair", "REG_DWORD", "1"},
	} {
		run("reg", "add", uninstKey, "/v", kv[0], "/t", kv[1], "/d", kv[2], "/f")
	}
	log.Printf("installed to %s", exe)
	return nil
}

// Uninstall removes the agent. Must run as administrator.
func Uninstall(keepPID int) error {
	if runtime.GOOS != "windows" {
		return errors.New("only supported on Windows")
	}
	if !sys.IsAdmin() {
		return errors.New("administrator rights required")
	}
	stopOthers(keepPID)
	run("reg", "delete", runKey, "/v", "OrbitAgent", "/f")
	run("reg", "delete", uninstKey, "/f")
	run("netsh", "advfirewall", "firewall", "delete", "rule", "name=Orbit Agent")
	os.RemoveAll(ProgramDataDir())
	// the running exe cannot delete itself: remove the folder shortly after we exit
	dir := InstallDir()
	c := sys.HiddenCmd("cmd.exe", "/C", "ping -n 3 127.0.0.1 >NUL & rmdir /S /Q \""+dir+"\"")
	c.Dir = os.TempDir()
	return c.Start()
}
