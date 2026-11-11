// Orbit Agent - runs on every student computer.
//
//	OrbitAgent.exe                         start (first start shows the setup card)
//	OrbitAgent.exe --install [--key K]     install for all users (asks for admin rights)
//	OrbitAgent.exe --install --key K --silent   unattended install (GPO / Intune / scripts)
//	OrbitAgent.exe --uninstall [--silent]  remove from this computer
//	OrbitAgent.exe --portable              run without installing
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"orbit/internal/agent"
	"orbit/internal/proto"
	"orbit/internal/screen"
	"orbit/internal/sys"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	fInstall   = flag.Bool("install", false, "install for all users of this computer")
	fUninstall = flag.Bool("uninstall", false, "remove Orbit Agent from this computer")
	fSilent    = flag.Bool("silent", false, "no dialogs (for scripts and mass deployment)")
	fKey       = flag.String("key", "", "classroom key(s) OPK1-..., comma separated")
	fPort      = flag.Int("port", 0, "TCP port (default 21400)")
	fConfig    = flag.String("config", "", "use this config file")
	fPortable  = flag.Bool("portable", false, "run without installing")
	fAutostart = flag.Bool("autostart", false, "started automatically at sign-in")
	fElevated  = flag.Bool("elevated", false, "internal: elevated helper process")
	fLang      = flag.String("lang", "", "language: en, sl (default: Windows language)")
	fParent    = flag.Int("parent-pid", 0, "internal: process that must not be stopped")
	fService   = flag.Bool("service", false, "internal: run as the Windows service")
	fNoService = flag.Bool("no-service", false, "install without the protected service (autostart only)")
)

func setupLog() {
	dir := filepath.Join(agent.UserDir(), "logs")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "agent.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
	log.SetFlags(log.LstdFlags)
}

func keysFromFlag() []string {
	var out []string
	for _, k := range strings.Split(*fKey, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

func main() {
	flag.Parse()
	screen.Init()
	if *fService {
		// Launched by the Service Control Manager: supervise user sessions.
		os.Exit(agent.RunService())
	}
	setupLog()
	log.Printf("Orbit Agent %s starting (%s/%s)", proto.Version, runtime.GOOS, runtime.GOARCH)

	lang := *fLang
	if lang == "" {
		lang = sys.UILanguage()
	}
	tr := agent.NewTr(lang, agent.ExeDir())

	switch {
	case *fUninstall:
		os.Exit(doUninstall(tr))
	case *fInstall:
		os.Exit(doInstall(tr, nil))
	}

	cfg, err := agent.LoadConfig(*fConfig)
	portable := *fPortable
	interactive := !*fAutostart && !*fSilent && !*fPortable
	fromPreset := err == nil && filepath.Clean(cfg.Path()) == filepath.Clean(filepath.Join(agent.ExeDir(), "orbit-agent.json"))
	if err != nil || (fromPreset && interactive && !agent.IsInstalled() && runtime.GOOS == "windows") {
		if !interactive {
			log.Printf("no configuration: %v", err)
			os.Exit(2)
		}
		// first start: interactive setup
		if err := sys.StartUI(nopUI{}); err != nil {
			log.Fatal(err)
		}
		cfg, portable = firstRunSetup(tr)
		if cfg == nil {
			return
		}
	}
	if *fPort > 0 {
		cfg.Port = *fPort
	}
	if *fLang != "" {
		cfg.Language = *fLang
	}
	if !strings.HasPrefix(cfg.Path(), agent.ProgramDataDir()) && !agent.IsInstalled() {
		portable = true
	}
	runAgent(cfg, portable, tr)
}

type nopUI struct{}

func (nopUI) TrayMenu() []sys.MenuItem { return nil }
func (nopUI) TrayCommand(string)       {}
func (nopUI) Hotkey(int)               {}

func runAgent(cfg *agent.Config, portable bool, tr *agent.Tr) {
	if !sys.SingleInstance("OrbitAgent") {
		log.Printf("already running")
		if !*fAutostart && !*fSilent {
			sys.MessageBox("Orbit Agent", tr.T("already"), "info")
		}
		return
	}
	cert, err := agent.LoadOrCreateCert(filepath.Dir(cfg.Path()), agent.ProgramDataDir(), agent.UserDir())
	if err != nil {
		fatal(tr, "certificate: "+err.Error())
	}
	a, err := agent.New(cfg, cert)
	if err != nil {
		fatal(tr, err.Error())
	}
	a.Portable = portable
	ln, err := a.Listen()
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			fatal(tr, tr.T("error.port", cfg.Port))
		}
		fatal(tr, err.Error())
	}
	if err := sys.StartUI(a); err != nil {
		log.Printf("UI: %v", err)
	}
	if cfg.ShowTray {
		sys.EnableTray()
	}
	if cfg.AllowHand {
		sys.RegisterHandHotkey()
	}
	a.Run()
	go func() {
		if err := a.Serve(ln); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()
	log.Printf("listening on port %d (config %s, portable=%v)", cfg.Port, cfg.Path(), portable)
	if portable && !*fAutostart && !*fSilent {
		go func() {
			time.Sleep(800 * time.Millisecond)
			sys.Notify("Orbit Classroom", tr.T("portable.running"))
		}()
	}
	if os.Getenv("ORBIT_TEST_HAND") == "1" { // automated tests
		go func() {
			time.Sleep(3 * time.Second)
			a.RaiseHandForTest()
			a.AddInboxForTest("Can you help me with exercise 3?")
		}()
	}
	<-a.Quit
	sys.RemoveTray()
}

func fatal(tr *agent.Tr, msg string) {
	log.Printf("fatal: %s", msg)
	if !*fSilent && !*fAutostart {
		sys.MessageBox("Orbit Agent", msg, "error")
	}
	os.Exit(1)
}

// firstRunSetup shows the welcome card: install or run once.
func firstRunSetup(tr *agent.Tr) (*agent.Config, bool) {
	// orbit-agent.json exported from Orbit Master lying next to the exe?
	var preset *agent.Config
	if c, err := agent.ReadConfig(filepath.Join(agent.ExeDir(), "orbit-agent.json")); err == nil && len(c.TrustedKeys) > 0 {
		preset = c
	}
	type answer struct {
		btn  int
		text string
	}
	ask := func(card sys.Card) answer {
		ch := make(chan answer, 1)
		sys.ShowCard(card, func(b int, t string) { ch <- answer{b, t} })
		return <-ch
	}
	cfg := agent.DefaultConfig()
	if preset != nil {
		cfg = preset
		fp := "?"
		if pub, err := proto.ParsePublicKey(preset.TrustedKeys[0]); err == nil {
			fp = proto.Fingerprint(pub)
		}
		a := ask(sys.Card{Title: tr.T("setup.title"), Body: tr.T("setup.bodyFound", fp), Glyph: sys.GlyphShield,
			Buttons: []string{tr.T("setup.install"), tr.T("setup.runOnce"), tr.T("cancel")}, Primary: 0, Cancel: 2, Topmost: true, MinWidthDIP: 500})
		switch a.btn {
		case 0:
			return installFromSetup(tr, cfg)
		case 1:
			return cfg, true
		}
		return nil, false
	}
	errText := ""
	for {
		body := tr.T("setup.body")
		if errText != "" {
			body += "\n\n⚠ " + errText
		}
		a := ask(sys.Card{Title: tr.T("setup.title"), Body: body, Glyph: sys.GlyphKey, Input: true,
			Buttons: []string{tr.T("setup.install"), tr.T("setup.runOnce"), tr.T("cancel")}, Primary: 0, Cancel: 2, Topmost: true, MinWidthDIP: 520})
		if a.btn == 2 || a.btn < 0 {
			return nil, false
		}
		if _, err := proto.ParsePublicKey(a.text); err != nil {
			errText = tr.T("setup.badKey", err.Error())
			continue
		}
		cfg.TrustedKeys = []string{strings.ToUpper(strings.TrimSpace(a.text))}
		if a.btn == 0 {
			return installFromSetup(tr, cfg)
		}
		cfg.Save(filepath.Join(agent.UserDir(), "agent.json"))
		return cfg, true
	}
}

func installFromSetup(tr *agent.Tr, cfg *agent.Config) (*agent.Config, bool) {
	code := doInstall(tr, cfg)
	if code != 0 {
		return nil, false
	}
	return nil, false // the installed copy was started by doInstall
}

// doInstall installs (elevating via UAC if needed) and starts the installed agent.
func doInstall(tr *agent.Tr, cfg *agent.Config) int {
	if cfg == nil {
		cfg = agent.DefaultConfig()
		if c, err := agent.ReadConfig(filepath.Join(agent.ExeDir(), "orbit-agent.json")); err == nil {
			cfg = c
		}
		if ks := keysFromFlag(); len(ks) > 0 {
			cfg.TrustedKeys = ks
		}
		if *fPort > 0 {
			cfg.Port = *fPort
		}
		if *fLang != "" {
			cfg.Language = *fLang
		}
	}
	if len(cfg.TrustedKeys) == 0 {
		msg := "No classroom key. Use --key OPK1-... or put orbit-agent.json next to OrbitAgent.exe."
		log.Print(msg)
		if !*fSilent {
			sys.MessageBox("Orbit Agent", msg, "error")
		}
		return 2
	}
	if !sys.IsAdmin() {
		if *fElevated {
			return 5
		}
		// write the config to a temp file and re-run elevated
		tmp := filepath.Join(os.TempDir(), fmt.Sprintf("orbit-agent-setup-%d.json", os.Getpid()))
		if err := cfg.Save(tmp); err != nil {
			return 1
		}
		defer os.Remove(tmp)
		self, _ := os.Executable()
		args := fmt.Sprintf(`--install --elevated --silent --parent-pid %d --config "%s"`, os.Getpid(), tmp)
		if *fNoService {
			args += " --no-service"
		}
		code, err := sys.RunElevated(self, args)
		if err != nil || code != 0 {
			msg := fmt.Sprintf("exit code %d", code)
			if err != nil {
				msg = err.Error()
			}
			if !*fSilent {
				sys.MessageBox("Orbit Agent", tr.T("setup.failed", msg), "error")
			}
			return 1
		}
	} else {
		if *fConfig != "" {
			if c, err := agent.ReadConfig(*fConfig); err == nil {
				cfg = c
			}
		}
		if err := agent.Install(cfg, *fParent, !*fNoService); err != nil {
			log.Printf("install failed: %v", err)
			if !*fSilent {
				sys.MessageBox("Orbit Agent", tr.T("setup.failed", err.Error()), "error")
			}
			return 1
		}
		if *fElevated {
			return 0
		}
	}
	// start the installed agent in the user's session (not elevated)
	if !*fSilent || !sys.IsAdmin() {
		exec.Command(agent.InstalledExe(), "--autostart").Start()
	}
	if !*fSilent {
		sys.MessageBox("Orbit Agent", tr.T("setup.done"), "info")
	}
	return 0
}

func doUninstall(tr *agent.Tr) int {
	if !sys.IsAdmin() {
		if *fElevated {
			return 5
		}
		self, _ := os.Executable()
		code, err := sys.RunElevated(self, fmt.Sprintf("--uninstall --elevated --silent --parent-pid %d", os.Getpid()))
		if err != nil || code != 0 {
			return 1
		}
	} else if err := agent.Uninstall(*fParent); err != nil {
		log.Printf("uninstall failed: %v", err)
		if !*fSilent {
			sys.MessageBox("Orbit Agent", err.Error(), "error")
		}
		return 1
	}
	if !*fSilent {
		sys.MessageBox("Orbit Agent", tr.T("setup.uninstalled"), "info")
	}
	return 0
}
