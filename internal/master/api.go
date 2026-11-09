package master

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"orbit/internal/proto"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v)
}

// Handler returns the HTTP handler for the local UI server.
func (m *Master) Handler(port int) http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(static))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if t := r.URL.Query().Get("t"); t != "" {
			if t == m.SessionToken {
				http.SetCookie(w, &http.Cookie{Name: "orbit_session", Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			}
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
	api := http.NewServeMux()
	api.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, map[string]any{"ok": true, "version": proto.Version})
	})
	api.HandleFunc("GET /api/events", m.handleEvents)
	api.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(m.snapshot())
	})
	api.HandleFunc("GET /api/settings", m.handleGetSettings)
	api.HandleFunc("PUT /api/settings", m.handlePutSettings)
	api.HandleFunc("GET /api/langs", m.handleLangs)
	api.HandleFunc("GET /api/i18n/{lang}", m.handleI18n)
	api.HandleFunc("GET /api/docs/{lang}", m.handleDocs)
	api.HandleFunc("POST /api/view", m.handleView)
	api.HandleFunc("POST /api/rooms", m.handleAddRoom)
	api.HandleFunc("PUT /api/rooms/{id}", m.handleEditRoom)
	api.HandleFunc("DELETE /api/rooms/{id}", m.handleDeleteRoom)
	api.HandleFunc("POST /api/computers", m.handleAddComputer)
	api.HandleFunc("POST /api/computers/bulk", m.handleBulkAdd)
	api.HandleFunc("PUT /api/computers/{id}", m.handleEditComputer)
	api.HandleFunc("DELETE /api/computers/{id}", m.handleDeleteComputer)
	api.HandleFunc("POST /api/computers/move", m.handleMove)
	api.HandleFunc("POST /api/discover", m.handleDiscover)
	api.HandleFunc("POST /api/action", m.handleAction)
	api.HandleFunc("POST /api/wake", m.handleWake)
	api.HandleFunc("GET /api/thumb/{id}", m.handleThumb)
	api.HandleFunc("GET /api/stream/{id}", m.handleStream)
	api.HandleFunc("POST /api/input/{id}", m.handleInput)
	api.HandleFunc("POST /api/file", m.handleFile)
	api.HandleFunc("POST /api/screenshot", m.handleScreenshot)
	api.HandleFunc("GET /api/screenshots", m.handleListShots)
	api.HandleFunc("GET /api/screenshots/file/{name}", m.handleGetShot)
	api.HandleFunc("DELETE /api/screenshots/file/{name}", m.handleDeleteShot)
	api.HandleFunc("POST /api/demo/start", m.handleDemoStart)
	api.HandleFunc("POST /api/demo/stop", func(w http.ResponseWriter, r *http.Request) { m.stopDemo(); jsonOut(w, map[string]bool{"ok": true}) })
	api.HandleFunc("POST /api/poll", m.handlePoll)
	api.HandleFunc("POST /api/poll/close", m.handlePollClose)
	api.HandleFunc("POST /api/poll/clear", func(w http.ResponseWriter, r *http.Request) {
		m.closePoll()
		m.mu.Lock()
		m.poll = nil
		m.mu.Unlock()
		m.markDirty()
		jsonOut(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/inbox/read", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		for i := range m.inbox {
			m.inbox[i].Read = true
		}
		m.mu.Unlock()
		m.markDirty()
		jsonOut(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/inbox/clear", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.inbox = nil
		m.mu.Unlock()
		m.markDirty()
		jsonOut(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/key/regenerate", m.handleKeyRegenerate)
	api.HandleFunc("GET /api/key/secret", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, map[string]string{"secret": proto.SecretKeyString(m.key())})
	})
	api.HandleFunc("POST /api/key/import", m.handleKeyImport)
	api.HandleFunc("GET /api/agent-config", m.handleAgentConfig)
	api.HandleFunc("GET /api/agent-package", m.handleAgentPackage)
	api.HandleFunc("POST /api/open-window", func(w http.ResponseWriter, r *http.Request) {
		go OpenWindow(fmt.Sprintf("http://127.0.0.1:%d/?t=%s", port, m.SessionToken), filepath.Join(m.dir, "ui-profile"))
		jsonOut(w, map[string]bool{"ok": true})
	})
	api.HandleFunc("POST /api/open-folder", m.handleOpenFolder)
	api.HandleFunc("POST /api/quit", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, map[string]bool{"ok": true})
		go func() { time.Sleep(200 * time.Millisecond); m.RequestQuit() }()
	})

	allowedHosts := map[string]bool{fmt.Sprintf("127.0.0.1:%d", port): true, fmt.Sprintf("localhost:%d", port): true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[r.Host] { // DNS rebinding protection
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !m.authorized(r) {
				jsonErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			api.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (m *Master) authorized(r *http.Request) bool {
	if c, err := r.Cookie("orbit_session"); err == nil && c.Value == m.SessionToken {
		return true
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if auth == "" {
		return false
	}
	if auth == m.SessionToken {
		return true
	}
	s := m.settings()
	return s.APIToken != "" && auth == s.APIToken
}

func (m *Master) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := m.subscribe()
	defer m.unsubscribe(ch)
	fmt.Fprintf(w, "event: state\ndata: %s\n\n", m.snapshot())
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-m.Quit:
			fmt.Fprintf(w, "event: quit\ndata: {}\n\n")
			fl.Flush()
			return
		case msg := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.event, msg.data)
			fl.Flush()
		case <-ping.C:
			fmt.Fprintf(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// ------------------------------------------------------------ settings, languages, docs

func (m *Master) shotsDir() string {
	s := m.settings()
	if s.ScreenshotDir != "" {
		return s.ScreenshotDir
	}
	return filepath.Join(PicturesDir(), "Orbit Screenshots")
}

func (m *Master) agentAvailable() bool {
	if len(m.AgentExe) > 0 {
		return true
	}
	_, err := os.Stat(filepath.Join(m.ExeDir, "OrbitAgent.exe"))
	return err == nil
}

func (m *Master) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	s := m.cfg.Settings
	pub := m.priv.Public().(ed25519.PublicKey)
	m.mu.RUnlock()
	host, _ := os.Hostname()
	jsonOut(w, map[string]any{
		"settings": s, "publicKey": proto.EncodeKey(proto.PublicKeyPrefix, pub), "fingerprint": proto.Fingerprint(pub),
		"version": proto.Version, "configDir": m.dir, "screenshotsDir": m.shotsDir(), "os": runtime.GOOS,
		"hostname": host, "agentAvailable": m.agentAvailable(), "systemLanguage": systemLanguage(),
		"agentPort": proto.AgentPort, "discoveryPort": proto.DiscoveryPort, "masterName": m.name,
	})
}

func (m *Master) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	s := m.cfg.Settings
	if err := readJSON(r, &s); err != nil {
		m.mu.Unlock()
		jsonErr(w, 400, err.Error())
		return
	}
	s.ThumbInterval = min(max(s.ThumbInterval, 250), 10000)
	s.ViewQuality = min(max(s.ViewQuality, 20), 95)
	s.DemoQuality = min(max(s.DemoQuality, 20), 95)
	s.ViewFPS = min(max(s.ViewFPS, 2), 30)
	s.DemoFPS = min(max(s.DemoFPS, 2), 30)
	s.ViewMaxW = min(max(s.ViewMaxW, 640), 7680)
	s.DemoMaxW = min(max(s.DemoMaxW, 640), 7680)
	for i := range s.CustomActions {
		if s.CustomActions[i].ID == "" {
			s.CustomActions[i].ID = proto.RandomID()
		}
	}
	m.cfg.Settings = s
	err := m.saveLocked()
	m.mu.Unlock()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	m.handleGetSettings(w, r)
}

// langDir is the folder for additional language packs (next to the exe).
func (m *Master) langDir() string { return filepath.Join(m.ExeDir, "lang") }

func (m *Master) handleLangs(w http.ResponseWriter, r *http.Request) {
	type lang struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	seen := map[string]string{}
	read := func(b []byte, code string) {
		var mm map[string]any
		if json.Unmarshal(b, &mm) == nil {
			if n, ok := mm["_name"].(string); ok {
				seen[code] = n
				return
			}
		}
		seen[code] = code
	}
	ents, _ := fs.ReadDir(webFS, "web/i18n")
	for _, e := range ents {
		code := strings.TrimSuffix(e.Name(), ".json")
		b, _ := webFS.ReadFile("web/i18n/" + e.Name())
		read(b, code)
	}
	ext, _ := os.ReadDir(m.langDir())
	for _, e := range ext {
		if strings.HasSuffix(e.Name(), ".json") {
			b, _ := os.ReadFile(filepath.Join(m.langDir(), e.Name()))
			read(b, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	var out []lang
	for c, n := range seen {
		out = append(out, lang{c, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	jsonOut(w, out)
}

func validLang(s string) bool {
	if s == "" || len(s) > 12 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (m *Master) handleI18n(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if !validLang(lang) {
		jsonErr(w, 400, "bad language")
		return
	}
	merged := map[string]any{}
	load := func(b []byte) {
		var mm map[string]any
		if json.Unmarshal(b, &mm) == nil {
			for k, v := range mm {
				merged[k] = v
			}
		}
	}
	if b, err := webFS.ReadFile("web/i18n/en.json"); err == nil {
		load(b)
	}
	if b, err := webFS.ReadFile("web/i18n/" + lang + ".json"); err == nil {
		load(b)
	}
	if b, err := os.ReadFile(filepath.Join(m.langDir(), lang+".json")); err == nil {
		load(b)
	}
	jsonOut(w, merged)
}

func (m *Master) handleDocs(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if !validLang(lang) {
		lang = "en"
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	if b, err := os.ReadFile(filepath.Join(m.langDir(), lang+".md")); err == nil {
		w.Write(b)
		return
	}
	if b, err := webFS.ReadFile("web/docs/" + lang + ".md"); err == nil {
		w.Write(b)
		return
	}
	b, _ := webFS.ReadFile("web/docs/en.md")
	w.Write(b)
}

func (m *Master) handleView(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs   []string `json:"ids"`
		Fast  []string `json:"fast"`
		Width int      `json:"width"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	m.mu.Lock()
	m.view = map[string]bool{}
	m.fast = map[string]bool{}
	for _, id := range req.IDs {
		m.view[id] = true
	}
	for _, id := range req.Fast {
		m.fast[id] = true
	}
	m.viewW = min(max(req.Width, 160), 1280)
	conns := make([]*Conn, 0, len(req.IDs))
	for _, id := range req.IDs {
		if c := m.conns[id]; c != nil {
			conns = append(conns, c)
		}
	}
	m.mu.Unlock()
	for _, c := range conns {
		c.poke()
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// ------------------------------------------------------------ rooms & computers

func (m *Master) handleAddRoom(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name string }
	readJSON(r, &req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		jsonErr(w, 400, "name required")
		return
	}
	m.mu.Lock()
	room := &Room{ID: proto.RandomID(), Name: name, Order: len(m.cfg.Rooms)}
	m.cfg.Rooms = append(m.cfg.Rooms, room)
	m.saveLocked()
	m.mu.Unlock()
	m.markDirty()
	jsonOut(w, room)
}

func (m *Master) handleEditRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Order *int   `json:"order"`
	}
	readJSON(r, &req)
	m.mu.Lock()
	defer m.mu.Unlock()
	room := m.cfg.room(r.PathValue("id"))
	if room == nil {
		jsonErr(w, 404, "no such room")
		return
	}
	if n := strings.TrimSpace(req.Name); n != "" {
		room.Name = n
	}
	if req.Order != nil {
		room.Order = *req.Order
	}
	m.saveLocked()
	m.markDirty()
	jsonOut(w, room)
}

func (m *Master) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m.mu.Lock()
	var rooms []*Room
	for _, rm := range m.cfg.Rooms {
		if rm.ID != id {
			rooms = append(rooms, rm)
		}
	}
	m.cfg.Rooms = rooms
	var keep []*Computer
	for _, pc := range m.cfg.Computers {
		if pc.Room == id {
			if c := m.conns[pc.ID]; c != nil {
				close(c.stop)
				delete(m.conns, pc.ID)
			}
			continue
		}
		keep = append(keep, pc)
	}
	m.cfg.Computers = keep
	m.saveLocked()
	m.mu.Unlock()
	m.markDirty()
	jsonOut(w, map[string]bool{"ok": true})
}

type computerReq struct {
	Name string   `json:"name"`
	Host string   `json:"host"`
	Port int      `json:"port"`
	Room string   `json:"room"`
	MACs []string `json:"macs"`
	MAC  string   `json:"mac"`
}

func (q *computerReq) normalize() error {
	q.Host = strings.TrimSpace(q.Host)
	q.Name = strings.TrimSpace(q.Name)
	if h, p, err := net.SplitHostPort(q.Host); err == nil {
		q.Host = h
		if n, err := strconv.Atoi(p); err == nil && q.Port == 0 {
			q.Port = n
		}
	}
	if q.Host == "" {
		return errors.New("host name or IP address required")
	}
	if q.Name == "" {
		q.Name = q.Host
	}
	if q.Port <= 0 || q.Port > 65535 {
		q.Port = proto.AgentPort
	}
	if q.MAC = strings.TrimSpace(q.MAC); q.MAC != "" {
		if _, err := net.ParseMAC(q.MAC); err != nil {
			return errors.New("invalid MAC address")
		}
		q.MACs = []string{q.MAC}
	}
	return nil
}

func (m *Master) addComputerLocked(q computerReq) *Computer {
	pc := &Computer{ID: proto.RandomID(), Name: q.Name, Host: q.Host, Port: q.Port, Room: q.Room, MACs: q.MACs, Order: 0}
	if m.cfg.room(pc.Room) == nil && len(m.cfg.Rooms) > 0 {
		pc.Room = m.cfg.Rooms[0].ID
	}
	m.cfg.Computers = append(m.cfg.Computers, pc)
	m.startConn(pc)
	return pc
}

func (m *Master) handleAddComputer(w http.ResponseWriter, r *http.Request) {
	var q computerReq
	readJSON(r, &q)
	if err := q.normalize(); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	m.mu.Lock()
	pc := m.addComputerLocked(q)
	m.saveLocked()
	m.mu.Unlock()
	m.markDirty()
	jsonOut(w, pc)
}

func (m *Master) handleBulkAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Room  string        `json:"room"`
		Items []computerReq `json:"items"`
	}
	readJSON(r, &req)
	added := 0
	m.mu.Lock()
	for _, q := range req.Items {
		q.Room = req.Room
		if q.normalize() != nil {
			continue
		}
		exists := false
		for _, pc := range m.cfg.Computers {
			if strings.EqualFold(pc.Host, q.Host) && pc.Port == q.Port {
				exists = true
				if pc.Room != req.Room && req.Room != "" {
					pc.Room = req.Room
				}
			}
		}
		if !exists {
			m.addComputerLocked(q)
			added++
		}
	}
	m.saveLocked()
	m.mu.Unlock()
	m.markDirty()
	jsonOut(w, map[string]int{"added": added})
}

func (m *Master) handleEditComputer(w http.ResponseWriter, r *http.Request) {
	var q computerReq
	readJSON(r, &q)
	if err := q.normalize(); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	m.mu.Lock()
	pc := m.cfg.computer(r.PathValue("id"))
	if pc == nil {
		m.mu.Unlock()
		jsonErr(w, 404, "no such computer")
		return
	}
	pc.Name, pc.Host, pc.Port = q.Name, q.Host, q.Port
	if q.Room != "" && m.cfg.room(q.Room) != nil {
		pc.Room = q.Room
	}
	if len(q.MACs) > 0 {
		pc.MACs = q.MACs
	}
	c := m.conns[pc.ID]
	m.saveLocked()
	m.mu.Unlock()
	if c != nil {
		c.setAddr(q.Host, q.Port)
	}
	m.markDirty()
	jsonOut(w, pc)
}

func (m *Master) handleDeleteComputer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m.mu.Lock()
	var keep []*Computer
	for _, pc := range m.cfg.Computers {
		if pc.ID != id {
			keep = append(keep, pc)
		}
	}
	m.cfg.Computers = keep
	if c := m.conns[id]; c != nil {
		close(c.stop)
		delete(m.conns, id)
	}
	m.saveLocked()
	m.mu.Unlock()
	m.markDirty()
	jsonOut(w, map[string]bool{"ok": true})
}

func (m *Master) handleMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs  []string `json:"ids"`
		Room string   `json:"room"`
		// Order, when given, sets the display order of ids (drag & drop).
		Order bool `json:"order"`
	}
	readJSON(r, &req)
	m.mu.Lock()
	for i, id := range req.IDs {
		if pc := m.cfg.computer(id); pc != nil {
			if req.Room != "" && m.cfg.room(req.Room) != nil {
				pc.Room = req.Room
			}
			if req.Order {
				pc.Order = i + 1
			}
		}
	}
	m.saveLocked()
	m.mu.Unlock()
	m.markDirty()
	jsonOut(w, map[string]bool{"ok": true})
}

func (m *Master) handleDiscover(w http.ResponseWriter, r *http.Request) {
	var req struct{ Scan bool }
	readJSON(r, &req)
	jsonOut(w, map[string]any{"found": m.Discover(req.Scan)})
}

// ------------------------------------------------------------ actions

// fanout runs fn for every target in parallel and collects errors ("" = ok).
func (m *Master) fanout(targets []string, fn func(c *Conn) error) map[string]string {
	res := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 48)
	for _, id := range targets {
		c := m.conn(id)
		if c == nil {
			res[id] = "unknown computer"
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			err := fn(c)
			mu.Lock()
			if err != nil {
				res[id] = shortErr(err)
			} else {
				res[id] = ""
			}
			mu.Unlock()
			c.poke()
		}()
	}
	wg.Wait()
	return res
}

func (m *Master) handleAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Targets []string      `json:"targets"`
		Action  string        `json:"action"`
		Params  proto.Command `json:"params"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	cmd := req.Params
	cmd.Action = req.Action
	if cmd.Action == "lock" {
		s := m.settings()
		if cmd.Title == "" {
			cmd.Title = s.LockTitle
		}
		if cmd.Text == "" {
			cmd.Text = s.LockText
		}
	}
	if cmd.Action == "" {
		jsonErr(w, 400, "action required")
		return
	}
	res := m.fanout(req.Targets, func(c *Conn) error { return c.Cmd(cmd) })
	jsonOut(w, map[string]any{"results": res})
}

func (m *Master) handleWake(w http.ResponseWriter, r *http.Request) {
	var req struct{ Targets []string }
	readJSON(r, &req)
	jsonOut(w, map[string]any{"results": m.WakeOnLAN(req.Targets)})
}

func (m *Master) handleThumb(w http.ResponseWriter, r *http.Request) {
	c := m.conn(r.PathValue("id"))
	if c == nil {
		http.NotFound(w, r)
		return
	}
	c.mu.Lock()
	data := c.thumb
	c.mu.Unlock()
	if data == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Write(data)
}

func (m *Master) handleStream(w http.ResponseWriter, r *http.Request) {
	c := m.conn(r.PathValue("id"))
	if c == nil {
		http.NotFound(w, r)
		return
	}
	s := m.settings()
	in := r.URL.Query()
	q := url.Values{}
	q.Set("q", fmt.Sprint(s.ViewQuality))
	q.Set("maxw", fmt.Sprint(s.ViewMaxW))
	q.Set("fps", fmt.Sprint(s.ViewFPS))
	for _, k := range []string{"q", "maxw", "fps", "mon", "purpose", "kf"} {
		if v := in.Get(k); v != "" {
			q.Set(k, v)
		}
	}
	resp, err := c.do(r.Context(), c.streamClient, "GET", "/api/v1/stream?"+q.Encode(), nil, "", nil)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, shortErr(err))
		return
	}
	defer resp.Body.Close()
	fl, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/x-orbit-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	buf := make([]byte, 128<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func (m *Master) handleInput(w http.ResponseWriter, r *http.Request) {
	c := m.conn(r.PathValue("id"))
	if c == nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	path := "/api/v1/input"
	if mon := r.URL.Query().Get("mon"); mon != "" {
		path += "?mon=" + url.QueryEscape(mon)
	}
	resp, err := c.do(r.Context(), c.client, "POST", path, bytes.NewReader(body), "application/json", nil)
	if err != nil {
		jsonErr(w, http.StatusBadGateway, shortErr(err))
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (m *Master) handleFile(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	var targets []string
	open := false
	var tmp *os.File
	name := ""
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		switch part.FormName() {
		case "targets":
			json.NewDecoder(io.LimitReader(part, 1<<20)).Decode(&targets)
		case "open":
			b, _ := io.ReadAll(io.LimitReader(part, 16))
			open = string(b) == "1"
		case "file":
			name = part.FileName()
			tmp, err = os.CreateTemp("", "orbit-upload-*")
			if err != nil {
				jsonErr(w, 500, err.Error())
				return
			}
			if _, err := io.Copy(tmp, part); err != nil {
				jsonErr(w, 500, err.Error())
				return
			}
		}
		part.Close()
	}
	if tmp == nil || name == "" {
		jsonErr(w, 400, "no file")
		return
	}
	st, _ := tmp.Stat()
	size := st.Size()
	tmpName := tmp.Name()
	q := url.Values{"name": {name}}
	if open {
		q.Set("open", "1")
	}
	res := m.fanout(targets, func(c *Conn) error {
		f, err := os.Open(tmpName)
		if err != nil {
			return err
		}
		defer f.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		req := io.NopCloser(f)
		resp, err := c.doSized(ctx, "/api/v1/file?"+q.Encode(), req, size)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var cr proto.CommandResult
		json.NewDecoder(resp.Body).Decode(&cr)
		if !cr.OK {
			return errors.New(cr.Error)
		}
		return nil
	})
	jsonOut(w, map[string]any{"results": res})
}

// doSized uploads a body of known size (no chunked encoding).
func (c *Conn) doSized(ctx context.Context, path string, body io.ReadCloser, size int64) (*http.Response, error) {
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok == "" {
		return nil, errors.New("not connected")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base()+path, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/octet-stream")
	return c.streamClient.Do(req)
}

func fileSafe(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?* `, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func (m *Master) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	var req struct{ Targets []string }
	readJSON(r, &req)
	dir := m.shotsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	res := m.fanout(req.Targets, func(c *Conn) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := c.do(ctx, c.streamClient, "GET", "/api/v1/screenshot", nil, "", nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		m.mu.RLock()
		name := c.id
		if pc := m.cfg.computer(c.id); pc != nil {
			name = pc.Name
		}
		m.mu.RUnlock()
		c.mu.Lock()
		user := ""
		if c.status != nil {
			user = c.status.User
			if i := strings.Index(user, " ("); i > 0 {
				user = user[:i]
			}
		}
		c.mu.Unlock()
		fn := fileSafe(name)
		if user != "" {
			fn += "_" + fileSafe(user)
		}
		fn += "_" + time.Now().Format("2006-01-02_15-04-05") + ".png"
		f, err := os.Create(filepath.Join(dir, fn))
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(resp.Body, 256<<20))
		f.Close()
		return err
	})
	jsonOut(w, map[string]any{"results": res, "dir": dir})
}

func (m *Master) handleListShots(w http.ResponseWriter, r *http.Request) {
	type shot struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Time int64  `json:"time"`
	}
	ents, _ := os.ReadDir(m.shotsDir())
	out := []shot{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, shot{e.Name(), info.Size(), info.ModTime().Unix()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	jsonOut(w, map[string]any{"dir": m.shotsDir(), "items": out})
}

func (m *Master) shotPath(name string) (string, bool) {
	if name != filepath.Base(name) || strings.Contains(name, "..") || !strings.HasSuffix(strings.ToLower(name), ".png") {
		return "", false
	}
	return filepath.Join(m.shotsDir(), name), true
}

func (m *Master) handleGetShot(w http.ResponseWriter, r *http.Request) {
	p, ok := m.shotPath(r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, p)
}

func (m *Master) handleDeleteShot(w http.ResponseWriter, r *http.Request) {
	p, ok := m.shotPath(r.PathValue("name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	os.Remove(p)
	jsonOut(w, map[string]bool{"ok": true})
}

func (m *Master) handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	var req struct{ Which string }
	readJSON(r, &req)
	dir := m.dir
	if req.Which == "screenshots" {
		dir = m.shotsDir()
		os.MkdirAll(dir, 0o755)
	}
	if err := OpenFolder(dir); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOut(w, map[string]string{"dir": dir})
}

// ------------------------------------------------------------ demo & polls

func (m *Master) handleDemoStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Source  string   `json:"source"`
		Mode    string   `json:"mode"`
		Targets []string `json:"targets"`
	}
	readJSON(r, &req)
	if req.Source == "" {
		req.Source = "teacher"
	}
	if err := m.startDemo(req.Source, req.Mode, req.Targets); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (m *Master) handlePoll(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Targets  []string `json:"targets"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	readJSON(r, &req)
	var opts []string
	for _, o := range req.Options {
		if o = strings.TrimSpace(o); o != "" {
			opts = append(opts, o)
		}
	}
	if len(opts) < 1 || len(opts) > 12 {
		jsonErr(w, 400, "1-12 options required")
		return
	}
	m.closePoll()
	p := &Poll{ID: proto.RandomID(), Question: strings.TrimSpace(req.Question), Options: opts, Targets: req.Targets,
		Started: time.Now().Unix(), Active: true, Answers: map[string]int{}}
	m.mu.Lock()
	m.poll = p
	m.mu.Unlock()
	res := m.fanout(req.Targets, func(c *Conn) error {
		return c.Cmd(proto.Command{Action: "poll", ID: p.ID, Text: p.Question, Options: p.Options})
	})
	m.markDirty()
	jsonOut(w, map[string]any{"results": res, "id": p.ID})
}

func (m *Master) closePoll() {
	m.mu.Lock()
	p := m.poll
	if p != nil {
		p.Active = false
	}
	m.mu.Unlock()
	if p == nil {
		return
	}
	m.fanout(p.Targets, func(c *Conn) error { return c.Cmd(proto.Command{Action: "poll_close", ID: p.ID}) })
	m.markDirty()
}

func (m *Master) handlePollClose(w http.ResponseWriter, r *http.Request) {
	m.closePoll()
	jsonOut(w, map[string]bool{"ok": true})
}

// ------------------------------------------------------------ keys & deployment

func (m *Master) reconnectAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.conns {
		c.mu.Lock()
		c.token = ""
		c.state = "connecting"
		c.mu.Unlock()
		c.poke()
	}
}

func (m *Master) handleKeyRegenerate(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.priv = proto.NewTeacherKey()
	m.cfg.Secret = proto.SecretKeyString(m.priv)
	m.saveLocked()
	m.mu.Unlock()
	m.reconnectAll()
	m.handleGetSettings(w, r)
}

func (m *Master) handleKeyImport(w http.ResponseWriter, r *http.Request) {
	var req struct{ Key string }
	readJSON(r, &req)
	k, err := proto.ParseSecretKey(req.Key)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	m.mu.Lock()
	m.priv = k
	m.cfg.Secret = proto.SecretKeyString(k)
	m.saveLocked()
	m.mu.Unlock()
	m.reconnectAll()
	m.handleGetSettings(w, r)
}

func (m *Master) agentConfigJSON() []byte {
	pub := proto.PublicKeyString(m.key())
	cfg := map[string]any{
		"trusted_keys": []string{pub}, "port": proto.AgentPort, "discovery": true, "language": "auto",
		"notify_on_view": true, "allow_control": true, "show_tray": true, "allow_hand": true,
		"allow_messages": true, "share_activity": true, "received_folder": "",
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return b
}

func (m *Master) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="orbit-agent.json"`)
	w.Write(m.agentConfigJSON())
}

const packageReadme = `ORBIT AGENT - STUDENT COMPUTER SETUP
====================================

EN  Install on every student computer:
    1. Copy this whole folder to the student computer (USB stick or network share).
    2. Double-click "Install-Orbit-Agent.cmd" (or OrbitAgent.exe) and confirm the
       Windows administrator prompt.
    3. Done. The agent starts automatically for every user at sign-in.
    Mass deployment (GPO / Intune / script, run as administrator or SYSTEM):
       OrbitAgent.exe --install --silent
    Remove:  Settings > Apps > "Orbit Agent" > Uninstall   or   OrbitAgent.exe --uninstall

SL  Namestite na vsak učenčev računalnik:
    1. Kopirajte celotno mapo na učenčev računalnik (USB ključ ali omrežna mapa).
    2. Dvokliknite "Install-Orbit-Agent.cmd" (ali OrbitAgent.exe) in potrdite
       skrbniško okno sistema Windows.
    3. Končano. Agent se samodejno zažene za vsakega uporabnika ob prijavi.
    Množična namestitev (GPO / Intune / skripta, kot skrbnik ali SYSTEM):
       OrbitAgent.exe --install --silent
    Odstranitev:  Nastavitve > Aplikacije > "Orbit Agent" > Odstrani   ali   OrbitAgent.exe --uninstall

The file orbit-agent.json contains only the PUBLIC teacher key (safe to share).
Datoteka orbit-agent.json vsebuje le JAVNI učiteljev ključ (varno za deljenje).
`

func (m *Master) handleAgentPackage(w http.ResponseWriter, r *http.Request) {
	exe := m.AgentExe
	if len(exe) == 0 {
		exe, _ = os.ReadFile(filepath.Join(m.ExeDir, "OrbitAgent.exe"))
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte, method uint16) {
		h := &zip.FileHeader{Name: "Orbit Agent Setup/" + name, Method: method, Modified: time.Now()}
		f, err := zw.CreateHeader(h)
		if err == nil {
			f.Write(data)
		}
	}
	if len(exe) > 0 {
		add("OrbitAgent.exe", exe, zip.Deflate)
	}
	add("orbit-agent.json", m.agentConfigJSON(), zip.Deflate)
	add("Install-Orbit-Agent.cmd", []byte("@echo off\r\nrem Installs Orbit Agent for all users (asks for administrator rights)\r\n\"%~dp0OrbitAgent.exe\" --install\r\n"), zip.Deflate)
	add("README.txt", []byte(strings.ReplaceAll(packageReadme, "\n", "\r\n")), zip.Deflate)
	if err := zw.Close(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="Orbit-Agent-Setup.zip"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.Write(buf.Bytes())
	if len(exe) == 0 {
		log.Printf("agent package created without OrbitAgent.exe (not embedded / not found next to master)")
	}
}
