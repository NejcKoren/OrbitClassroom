// Orbit Master - the teacher console.
//
//	OrbitMaster.exe              start (opens the window)
//	OrbitMaster.exe --no-window  run headless, print the address (automation)
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"orbit/internal/master"
	"orbit/internal/proto"
	"orbit/internal/screen"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"
)

//go:embed all:embedded
var embedded embed.FS

var (
	fNoWindow = flag.Bool("no-window", false, "do not open the user interface window")
	fPort     = flag.Int("port", 21480, "local UI/API port (127.0.0.1 only; 0 = random)")
	fDir      = flag.String("dir", "", "configuration folder")
	fGrace    = flag.Duration("grace", 20*time.Second, "exit this long after the window was closed")
)

type instance struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func configDir() string {
	if *fDir != "" {
		return *fDir
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "Orbit Classroom")
}

func setupLog(dir string) {
	ld := filepath.Join(dir, "logs")
	os.MkdirAll(ld, 0o755)
	p := filepath.Join(ld, "master.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 4<<20 {
		os.Rename(p, p+".1")
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
}

// existing returns a running instance (single instance per user).
func existing(dir string) *instance {
	b, err := os.ReadFile(filepath.Join(dir, "instance.json"))
	if err != nil {
		return nil
	}
	var in instance
	if json.Unmarshal(b, &in) != nil || in.Port == 0 {
		return nil
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/ping", in.Port), nil)
	req.Host = fmt.Sprintf("127.0.0.1:%d", in.Port)
	req.Header.Set("Authorization", "Bearer "+in.Token)
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := c.Do(req)
	if err != nil {
		return nil
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	return &in
}

func main() {
	flag.Parse()
	screen.Init()
	dir := configDir()
	os.MkdirAll(dir, 0o700)
	setupLog(dir)
	log.Printf("Orbit Master %s starting (%s/%s)", proto.Version, runtime.GOOS, runtime.GOARCH)

	if in := existing(dir); in != nil {
		// already running: just open another window of the running instance
		req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/open-window", in.Port), nil)
		req.Header.Set("Authorization", "Bearer "+in.Token)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
		log.Printf("already running on port %d - opened a new window", in.Port)
		return
	}

	m, err := master.New(dir)
	if err != nil {
		master.ShowError("Orbit Master", "Cannot start: "+err.Error())
		os.Exit(1)
	}
	if b, err := embedded.ReadFile("embedded/OrbitAgent.exe"); err == nil && len(b) > 0 {
		m.AgentExe = b
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *fPort))
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		master.ShowError("Orbit Master", "Cannot open local port: "+err.Error())
		os.Exit(1)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ib, _ := json.Marshal(instance{Port: port, Token: m.SessionToken, PID: os.Getpid()})
	instPath := filepath.Join(dir, "instance.json")
	os.WriteFile(instPath, ib, 0o600)
	defer os.Remove(instPath)

	srv := &http.Server{Handler: m.Handler(port), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	m.Start()
	url := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, m.SessionToken)
	log.Printf("UI listening on 127.0.0.1:%d - window address: %s", port, url)
	if *fNoWindow {
		fmt.Println("Orbit Master running:", url)
	} else {
		if _, err := master.OpenWindow(url, filepath.Join(dir, "ui-profile")); err != nil {
			log.Printf("could not open window: %v", err)
		}
		go m.WatchClients(*fGrace)
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; m.RequestQuit() }()
	<-m.Quit
	log.Printf("shutting down")
	m.Stop()
	srv.Close()
}
