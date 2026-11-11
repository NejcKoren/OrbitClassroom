//go:build windows

package agent

// Orbit Agent service mode.
//
// When installed in protected mode the agent is registered as a SYSTEM
// Windows service (OrbitAgentSvc, auto-start, restart-on-crash). The service
// itself shows no UI (Windows session 0 cannot). Its only job is to make sure
// a normal UI agent runs in every signed-in user's session, and to relaunch it
// within a couple of seconds if it stops. A standard user (a student) cannot
// stop or delete a service, so the monitoring cannot be turned off without
// administrator rights. The UI agent still shows the tray icon and all the
// usual "you are being viewed" notifications, so nothing is hidden.

import (
	"fmt"
	"io"
	"log"
	"orbit/internal/win"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// ServiceName is the Windows service name (visible in services.msc).
const ServiceName = "OrbitAgentSvc"

// ServiceDisplayName is shown in the Services list.
const ServiceDisplayName = "Orbit Agent (Orbit Classroom)"

var serviceStop atomic.Bool

// serviceLog points logging at ProgramData (SYSTEM can always write there).
func serviceLog() {
	dir := filepath.Join(ProgramDataDir(), "logs")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "service.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		os.Rename(p, p+".1")
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
	}
	log.SetFlags(log.LstdFlags)
}

// RunService runs the agent as a Windows service. It returns an exit code.
// If the process was not launched by the Service Control Manager (for example
// someone ran "OrbitAgent.exe --service" by hand) it returns a non-zero code
// so the caller can fall back to a normal run.
func RunService() int {
	serviceLog()
	log.Printf("Orbit Agent service starting")
	connected := win.StartServiceDispatcher(ServiceName, serviceMain)
	if !connected {
		log.Printf("not started by the Service Control Manager")
		return 3
	}
	return 0
}

var serviceHandle win.ServiceStatusHandle

func serviceMain() {
	serviceHandle = win.RegisterServiceHandler(ServiceName, func(control uint32) {
		switch control {
		case win.SERVICE_CONTROL_STOP, win.SERVICE_CONTROL_SHUTDOWN:
			win.SetServiceState(serviceHandle, win.SERVICE_STOP_PENDING, 0, 4000)
			serviceStop.Store(true)
		}
	})
	if serviceHandle == 0 {
		log.Printf("could not register service control handler")
		return
	}
	win.SetServiceState(serviceHandle, win.SERVICE_RUNNING, win.SERVICE_ACCEPT_STOP|win.SERVICE_ACCEPT_SHUTDOWN, 0)
	log.Printf("service running; supervising user sessions")
	supervise()
	win.SetServiceState(serviceHandle, win.SERVICE_STOPPED, 0, 0)
	log.Printf("service stopped")
}

type sessState struct {
	handle    uintptr
	pid       uint32
	startedAt time.Time
	backoff   time.Duration
	next      time.Time
}

// supervise keeps one UI agent running in every active user session until the
// service is asked to stop.
func supervise() {
	exe := InstalledExe()
	if _, err := os.Stat(exe); err != nil {
		if self, e := os.Executable(); e == nil {
			exe = self
		}
	}
	workdir := filepath.Dir(exe)
	sessions := map[uint32]*sessState{}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		if serviceStop.Load() {
			break
		}
		active := map[uint32]bool{}
		for _, s := range win.ActiveSessions() {
			if !s.Active || s.SessionID == 0 { // skip session 0 (no interactive user)
				continue
			}
			active[s.SessionID] = true
			st := sessions[s.SessionID]
			if st == nil {
				st = &sessState{backoff: 2 * time.Second}
				sessions[s.SessionID] = st
			}
			if win.ProcessAlive(st.handle) {
				continue
			}
			// the agent in this session is gone (never started, logged off, or
			// a student closed it): relaunch it, with a little backoff if it
			// keeps dying straight away.
			if st.handle != 0 {
				if time.Since(st.startedAt) < 4*time.Second {
					st.backoff = min(st.backoff*2, 60*time.Second)
				} else {
					st.backoff = 2 * time.Second
				}
				win.CloseHandleSafe(st.handle)
				st.handle = 0
				st.next = time.Now().Add(st.backoff)
			}
			if time.Now().Before(st.next) {
				continue
			}
			h, pid, err := win.LaunchInSession(s.SessionID, exe, "--autostart", workdir)
			if err != nil {
				log.Printf("session %d: launch failed: %v", s.SessionID, err)
				st.next = time.Now().Add(5 * time.Second)
				continue
			}
			st.handle, st.pid, st.startedAt = h, pid, time.Now()
			log.Printf("session %d: started agent pid %d", s.SessionID, pid)
		}
		// forget sessions that ended
		for id, st := range sessions {
			if !active[id] {
				win.CloseHandleSafe(st.handle)
				delete(sessions, id)
			}
		}
		select {
		case <-tick.C:
		}
	}
	for _, st := range sessions {
		win.CloseHandleSafe(st.handle)
	}
}

// InstallService registers the Windows service for exe and starts it.
func InstallService(exe string) error {
	bin := fmt.Sprintf(`"%s" --service`, exe)
	// remove any previous definition, then create fresh
	run("sc", "stop", ServiceName)
	run("sc", "delete", ServiceName)
	if err := run("sc", "create", ServiceName, "binPath=", bin, "start=", "auto",
		"obj=", "LocalSystem", "DisplayName=", ServiceDisplayName); err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	run("sc", "description", ServiceName, "Keeps the Orbit Classroom student agent running so the teacher can manage this computer.")
	// restart automatically if it ever crashes
	run("sc", "failure", ServiceName, "reset=", "86400", "actions=", "restart/5000/restart/5000/restart/5000")
	if err := run("sc", "start", ServiceName); err != nil {
		log.Printf("warning: could not start service immediately: %v", err)
	}
	return nil
}

// RemoveService stops and deletes the Windows service.
func RemoveService() {
	run("sc", "stop", ServiceName)
	run("sc", "delete", ServiceName)
}
