// Package agent implements Orbit Agent, the program that runs on every
// student computer. It serves a small HTTPS API (port 21400) that Orbit
// Master uses for thumbnails, live view, remote control and commands.
package agent

import (
	"bufio"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"orbit/internal/proto"
	"orbit/internal/screen"
	"orbit/internal/sys"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type session struct {
	exp    time.Time
	master string
}

// Agent is one running student agent.
type Agent struct {
	cfg      *Config
	tr       *Tr
	trusted  map[string]ed25519.PublicKey
	cert     tls.Certificate
	certFP   string
	started  time.Time
	host     string
	Portable bool
	Quit     chan struct{}

	mu         sync.Mutex
	nonces     map[string]time.Time
	tokens     map[string]session
	locked     bool
	lockMode   string
	demoActive bool
	demoMode   string
	demoSeq    int
	hand       bool
	handAt     int64
	focus      bool
	focusList  []string
	poll       *proto.PollState
	pollClose  func()
	msgClose   func()
	inbox      []proto.InboxMsg
	inboxSeq   int64
	viewers    int
	lastInput  time.Time
	notified   map[string]time.Time

	thumbMu   sync.Mutex
	thumbImg  *image.RGBA
	thumbHash uint64
	thumbJPEG []byte
	thumbAt   time.Time
	thumbW    int
}

// New creates an agent from a config and TLS certificate.
func New(cfg *Config, cert tls.Certificate) (*Agent, error) {
	a := &Agent{
		cfg: cfg, cert: cert, started: time.Now(), Quit: make(chan struct{}),
		trusted: map[string]ed25519.PublicKey{}, nonces: map[string]time.Time{},
		tokens: map[string]session{}, notified: map[string]time.Time{},
	}
	for _, k := range cfg.TrustedKeys {
		pub, err := proto.ParsePublicKey(k)
		if err != nil {
			log.Printf("ignoring invalid trusted key %q: %v", k, err)
			continue
		}
		a.trusted[proto.Fingerprint(pub)] = pub
	}
	if len(a.trusted) == 0 {
		return nil, errors.New("no valid classroom key configured")
	}
	if len(cert.Certificate) == 0 {
		return nil, errors.New("no TLS certificate")
	}
	a.certFP = proto.CertFingerprint(cert.Certificate[0])
	a.host, _ = os.Hostname()
	lang := cfg.Language
	if lang == "" || lang == "auto" {
		lang = sys.UILanguage()
	}
	a.tr = NewTr(lang, ExeDir())
	sys.SetWaitingText(a.tr.T("demo.waiting"))
	return a, nil
}

// T translates a text.
func (a *Agent) T(key string, args ...any) string { return a.tr.T(key, args...) }

// Listen opens the TCP port (separately so port conflicts can be reported).
func (a *Agent) Listen() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", a.cfg.Port))
}

// Serve runs the HTTPS API until the listener is closed.
func (a *Agent) Serve(ln net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/hello", a.handleHello)
	mux.HandleFunc("POST /api/v1/auth", a.handleAuth)
	mux.HandleFunc("GET /api/v1/status", a.authed(a.handleStatus))
	mux.HandleFunc("GET /api/v1/thumb", a.authed(a.handleThumb))
	mux.HandleFunc("GET /api/v1/stream", a.authed(a.handleStream))
	mux.HandleFunc("GET /api/v1/screenshot", a.authed(a.handleScreenshot))
	mux.HandleFunc("POST /api/v1/input", a.authed(a.handleInput))
	mux.HandleFunc("POST /api/v1/cmd", a.authed(a.handleCmd))
	mux.HandleFunc("POST /api/v1/file", a.authed(a.handleFile))
	mux.HandleFunc("POST /api/v1/demo", a.authed(a.handleDemo))
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{a.cert},
			MinVersion:   tls.VersionTLS12,
			NextProtos:   []string{"http/1.1"},
		},
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){}, // HTTP/1.1 only
		ErrorLog:     log.New(io.Discard, "", 0),
	}
	return srv.ServeTLS(ln, "", "")
}

// Run starts the background loops (discovery, focus mode).
func (a *Agent) Run() {
	if a.cfg.Discovery {
		go a.discoveryLoop()
	}
	go a.focusLoop()
	go a.housekeeping()
}

// ------------------------------------------------------------ auth

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (a *Agent) handleHello(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if len(a.nonces) > 2000 {
		a.mu.Unlock()
		http.Error(w, "busy", http.StatusTooManyRequests)
		return
	}
	n := proto.RandomToken(24)
	a.nonces[n] = time.Now().Add(60 * time.Second)
	a.mu.Unlock()
	writeJSON(w, 200, proto.Hello{Product: proto.Product, Proto: proto.ProtoVersion, Version: proto.Version, Name: a.host, Nonce: n})
}

func (a *Agent) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req proto.AuthRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	a.mu.Lock()
	exp, ok := a.nonces[req.Nonce]
	delete(a.nonces, req.Nonce)
	a.mu.Unlock()
	fail := func(msg string) {
		log.Printf("auth from %s rejected: %s", r.RemoteAddr, msg)
		time.Sleep(300 * time.Millisecond)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
	}
	if !ok || time.Now().After(exp) {
		fail("unknown or expired nonce")
		return
	}
	pub, err := proto.ParsePublicKey(req.Pub)
	if err != nil {
		fail("invalid key")
		return
	}
	if _, ok := a.trusted[proto.Fingerprint(pub)]; !ok {
		fail("key not trusted")
		return
	}
	if !proto.VerifyAuth(pub, req.Nonce, a.certFP, req.Sig) {
		fail("bad signature")
		return
	}
	tok := proto.RandomToken(32)
	a.mu.Lock()
	a.tokens[tok] = session{exp: time.Now().Add(12 * time.Hour), master: req.Master}
	a.mu.Unlock()
	log.Printf("teacher %q (%s) authenticated from %s", req.Master, proto.Fingerprint(pub), r.RemoteAddr)
	writeJSON(w, 200, proto.AuthResponse{Token: tok, TTL: 12 * 3600})
}

func (a *Agent) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		a.mu.Lock()
		s, ok := a.tokens[tok]
		a.mu.Unlock()
		if !ok || time.Now().After(s.exp) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (a *Agent) housekeeping() {
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		now := time.Now()
		a.mu.Lock()
		for k, v := range a.nonces {
			if now.After(v) {
				delete(a.nonces, k)
			}
		}
		for k, v := range a.tokens {
			if now.After(v.exp) {
				delete(a.tokens, k)
			}
		}
		a.mu.Unlock()
	}
}

// ------------------------------------------------------------ status

func (a *Agent) handleStatus(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, a.Status()) }

// Status builds the current status report.
func (a *Agent) Status() proto.Status {
	ips, macs := proto.LocalNetInfo()
	vs := screen.VirtualScreen()
	st := proto.Status{
		Host: a.host, User: sys.UserName(), OS: sys.OSName(), Version: proto.Version, Proto: proto.ProtoVersion,
		IPs: ips, MACs: macs, ScreenW: vs.Dx(), ScreenH: vs.Dy(), Monitors: screen.Monitors(),
		Features: proto.Features, Uptime: int64(time.Since(a.started).Seconds()), Lang: a.tr.lang,
		AllowCtl: a.cfg.AllowControl,
	}
	if a.cfg.ShareActivity {
		st.ActiveApp, st.ActiveTitle = sys.ActiveWindow()
		st.IdleSec = sys.IdleSeconds()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	st.Locked, st.LockMode = a.locked, a.lockMode
	st.Demo, st.DemoMode = a.demoActive, a.demoMode
	st.Hand, st.HandAt = a.hand, a.handAt
	st.Focus = a.focus
	st.Viewers = a.viewers
	st.Controlled = time.Since(a.lastInput) < 5*time.Second
	if a.poll != nil {
		p := *a.poll
		st.Poll = &p
	}
	st.Inbox = append([]proto.InboxMsg(nil), a.inbox...)
	return st
}

func (a *Agent) updateTray() {
	a.mu.Lock()
	ts := sys.TrayState{Hand: a.hand, Live: a.viewers > 0 || time.Since(a.lastInput) < 5*time.Second, Locked: a.locked}
	a.mu.Unlock()
	ts.Tooltip = a.T("tray.tip")
	if ts.Live {
		ts.Tooltip = a.T("tray.tipLive")
	} else if ts.Hand {
		ts.Tooltip = a.T("tray.tipHand")
	}
	sys.SetTray(ts)
}

// notifyOnce shows a notification at most once per interval for a given key.
func (a *Agent) notifyOnce(key string, every time.Duration, text string) {
	a.mu.Lock()
	last := a.notified[key]
	if time.Since(last) < every {
		a.mu.Unlock()
		return
	}
	a.notified[key] = time.Now()
	a.mu.Unlock()
	sys.Notify(a.T("product"), text)
}

// ------------------------------------------------------------ screen

func clampInt(s string, def, lo, hi int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return min(max(v, lo), hi)
}

func (a *Agent) handleThumb(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tw := clampInt(q.Get("w"), 320, 64, 1280)
	quality := clampInt(q.Get("q"), 60, 10, 95)
	a.thumbMu.Lock()
	if a.thumbJPEG == nil || a.thumbW != tw || time.Since(a.thumbAt) > 250*time.Millisecond {
		vs := screen.VirtualScreen()
		th := max(2, tw*vs.Dy()/max(1, vs.Dx()))
		img, err := screen.Capture(a.thumbImg, vs, tw, th, false)
		if err != nil {
			a.thumbMu.Unlock()
			http.Error(w, "screen unavailable", http.StatusServiceUnavailable)
			return
		}
		a.thumbImg = img
		h := fnv.New64a()
		h.Write(img.Pix)
		sum := h.Sum64()
		if sum != a.thumbHash || a.thumbW != tw || a.thumbJPEG == nil {
			buf := &byteBuf{}
			jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
			a.thumbJPEG = buf.b
			a.thumbHash = sum
		}
		a.thumbW = tw
		a.thumbAt = time.Now()
	}
	data, etag := a.thumbJPEG, fmt.Sprintf(`"%x"`, a.thumbHash)
	a.thumbMu.Unlock()
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(data)
}

type byteBuf struct{ b []byte }

func (b *byteBuf) Write(p []byte) (int, error) { b.b = append(b.b, p...); return len(p), nil }

func (a *Agent) viewerStart(purpose string) {
	a.mu.Lock()
	a.viewers++
	a.mu.Unlock()
	a.updateTray()
	if purpose == "demo" {
		a.notifyOnce("shared", 30*time.Second, a.T("notify.shared"))
	} else if a.cfg.NotifyOnView {
		a.notifyOnce("viewed", 60*time.Second, a.T("notify.viewed"))
	}
}

func (a *Agent) viewerEnd() {
	a.mu.Lock()
	a.viewers--
	a.mu.Unlock()
	a.updateTray()
}

func (a *Agent) handleStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	quality := clampInt(q.Get("q"), 70, 10, 95)
	maxW := clampInt(q.Get("maxw"), 1920, 160, 7680)
	fps := clampInt(q.Get("fps"), 20, 1, 30)
	kf := clampInt(q.Get("kf"), 0, 0, 3600)
	mon := clampInt(q.Get("mon"), -1, -1, 16)
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "application/x-orbit-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	fl.Flush()
	a.viewerStart(q.Get("purpose"))
	defer a.viewerEnd()

	enc := &proto.Encoder{Quality: quality}
	interval := time.Second / time.Duration(fps)
	var img *image.RGBA
	area := screen.Area(mon)
	areaAt := time.Now()
	lastKey := time.Now()
	ctx := r.Context()
	for {
		t0 := time.Now()
		if time.Since(areaAt) > 2*time.Second {
			area, areaAt = screen.Area(mon), time.Now()
		}
		ow, oh := screen.FitSize(area.Dx(), area.Dy(), maxW)
		var err error
		img, err = screen.Capture(img, area, ow, oh, true)
		if err == nil {
			force := kf > 0 && time.Since(lastKey) > time.Duration(kf)*time.Second
			data, key, err := enc.Encode(img, force)
			if err != nil {
				return
			}
			if key {
				lastKey = time.Now()
			}
			if data != nil {
				if _, err := w.Write(data); err != nil {
					return
				}
				fl.Flush()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(max(time.Millisecond, interval-time.Since(t0))):
		}
	}
}

func (a *Agent) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	mon := clampInt(r.URL.Query().Get("mon"), -1, -1, 16)
	img, err := screen.Capture(nil, screen.Area(mon), 0, 0, true)
	if err != nil {
		http.Error(w, "screen unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	(&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(w, img)
}

func (a *Agent) handleInput(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.AllowControl {
		http.Error(w, "remote control is disabled on this computer", http.StatusForbidden)
		return
	}
	var evs []proto.InputEvent
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&evs); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	mon := clampInt(r.URL.Query().Get("mon"), -1, -1, 16)
	a.mu.Lock()
	wasIdle := time.Since(a.lastInput) > 5*time.Second
	a.lastInput = time.Now()
	a.mu.Unlock()
	if wasIdle {
		a.updateTray()
		if a.cfg.NotifyOnView {
			a.notifyOnce("controlled", 60*time.Second, a.T("notify.controlled"))
		}
		go func() { time.Sleep(6 * time.Second); a.updateTray() }()
	}
	if err := sys.SendInput(evs, screen.Area(mon)); err != nil {
		writeJSON(w, 200, proto.CommandResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, 200, proto.CommandResult{OK: true})
}

// ------------------------------------------------------------ commands

func (a *Agent) handleCmd(w http.ResponseWriter, r *http.Request) {
	var c proto.Command
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&c); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	if err := a.Exec(c); err != nil {
		writeJSON(w, 200, proto.CommandResult{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, 200, proto.CommandResult{OK: true})
}

// Exec executes one command from the teacher.
func (a *Agent) Exec(c proto.Command) error {
	log.Printf("command: %s", c.Action)
	switch c.Action {
	case "lock":
		mode := c.Mode
		if mode != "input" {
			mode = "screen"
		}
		title, text := c.Title, c.Text
		if title == "" {
			title = a.T("lock.title")
		}
		if text == "" {
			text = a.T("lock.text")
		}
		a.mu.Lock()
		a.locked, a.lockMode = true, mode
		a.mu.Unlock()
		sys.ShowLock(mode, title, text)
		if mode == "input" {
			sys.Notify(a.T("product"), a.T("lock.input"))
		}
		a.updateTray()
	case "unlock":
		a.mu.Lock()
		a.locked, a.lockMode = false, ""
		a.mu.Unlock()
		sys.HideLock()
		a.updateTray()
	case "message":
		title := c.Title
		if title == "" {
			title = a.T("msg.title")
		}
		closeFn := sys.ShowCard(sys.Card{Title: title, Body: c.Text, Buttons: []string{a.T("ok")}, Primary: 0, Cancel: 0,
			Glyph: sys.GlyphMessage, Topmost: true, Beep: true, MinWidthDIP: 460}, nil)
		a.mu.Lock()
		a.msgClose = closeFn
		a.mu.Unlock()
	case "open_url":
		u, err := url.Parse(strings.TrimSpace(c.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("only http:// and https:// addresses are allowed")
		}
		return sys.ShellOpen(u.String(), "")
	case "run":
		if strings.TrimSpace(c.Program) == "" {
			return errors.New("no program given")
		}
		return sys.ShellOpen(strings.TrimSpace(c.Program), c.Args)
	case "power":
		switch c.Mode {
		case "shutdown", "reboot", "logoff", "abort":
		default:
			return errors.New("unknown power mode")
		}
		go func() { time.Sleep(300 * time.Millisecond); sys.Power(c.Mode, max(0, c.Delay), c.Text) }()
	case "focus":
		a.mu.Lock()
		was := a.focus
		a.focus = c.Enabled && len(c.List) > 0
		a.focusList = c.List
		now := a.focus
		a.mu.Unlock()
		if now && !was {
			sys.Notify(a.T("product"), a.T("focus.on"))
		} else if !now && was {
			sys.Notify(a.T("product"), a.T("focus.off"))
		}
	case "poll":
		if len(c.Options) == 0 || len(c.Options) > 12 {
			return errors.New("a poll needs 1-12 options")
		}
		a.mu.Lock()
		if a.pollClose != nil {
			a.pollClose()
		}
		id := c.ID
		a.poll = &proto.PollState{ID: id, Answer: -1}
		a.mu.Unlock()
		closeFn := sys.ShowCard(sys.Card{Title: a.T("poll.title"), Body: c.Text, Buttons: c.Options, Primary: -1, Cancel: -1,
			Vertical: true, Glyph: sys.GlyphPoll, Topmost: true, Beep: true, MinWidthDIP: 480}, func(btn int, _ string) {
			a.mu.Lock()
			if a.poll != nil && a.poll.ID == id && btn >= 0 {
				a.poll.Answer = btn
				a.poll.At = time.Now().Unix()
			}
			a.mu.Unlock()
		})
		a.mu.Lock()
		a.pollClose = closeFn
		a.mu.Unlock()
	case "poll_close":
		a.mu.Lock()
		if a.pollClose != nil {
			a.pollClose()
			a.pollClose = nil
		}
		if a.poll != nil && (c.ID == "" || a.poll.ID == c.ID) {
			a.poll = nil
		}
		a.mu.Unlock()
	case "hand_lower":
		a.mu.Lock()
		was := a.hand
		a.hand = false
		a.mu.Unlock()
		if was {
			sys.Notify(a.T("product"), a.T("hand.byTeacher"))
		}
		a.updateTray()
	case "inbox_ack":
		a.mu.Lock()
		keep := a.inbox[:0]
		for _, m := range a.inbox {
			if m.ID > c.Upto {
				keep = append(keep, m)
			}
		}
		a.inbox = keep
		a.mu.Unlock()
	case "demo_stop":
		a.mu.Lock()
		a.demoSeq++
		a.demoActive = false
		a.demoMode = ""
		a.mu.Unlock()
		sys.HideDemo()
	default:
		return fmt.Errorf("unknown action %q (agent version %s)", c.Action, proto.Version)
	}
	return nil
}

// ------------------------------------------------------------ files

func safeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, `\`, "/"))
	n = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, n)
	n = strings.Trim(n, ". ")
	if n == "" {
		n = "file"
	}
	return n
}

func uniquePath(dir, name string) string {
	p := filepath.Join(dir, name)
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
	}
}

func (a *Agent) handleFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := safeName(q.Get("name"))
	dir := a.cfg.ReceivedFolder
	if dir == "" {
		dir = filepath.Join(sys.DesktopDir(), "Orbit")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, 200, proto.CommandResult{Error: err.Error()})
		return
	}
	p := uniquePath(dir, name)
	f, err := os.Create(p)
	if err != nil {
		writeJSON(w, 200, proto.CommandResult{Error: err.Error()})
		return
	}
	_, err = io.Copy(f, http.MaxBytesReader(w, r.Body, 8<<30))
	f.Close()
	if err != nil {
		os.Remove(p)
		writeJSON(w, 200, proto.CommandResult{Error: err.Error()})
		return
	}
	log.Printf("received file %s", p)
	if q.Get("open") == "1" {
		sys.ShellOpen(p, "")
	} else {
		sys.Notify(a.T("product"), a.T("file.received", filepath.Base(p)))
	}
	writeJSON(w, 200, proto.CommandResult{OK: true})
}

// ------------------------------------------------------------ demo

func (a *Agent) handleDemo(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode != "window" {
		mode = "fullscreen"
	}
	canvas := &proto.Canvas{}
	a.mu.Lock()
	a.demoSeq++
	seq := a.demoSeq
	a.demoActive, a.demoMode = true, mode
	a.mu.Unlock()
	sys.ShowDemo(mode, a.T("demo.title"), canvas)
	log.Printf("demo started (%s)", mode)

	br := bufio.NewReaderSize(r.Body, 512<<10)
	for {
		f, err := proto.ReadFrame(br)
		if err != nil {
			break
		}
		a.mu.Lock()
		cur := a.demoSeq == seq
		a.mu.Unlock()
		if !cur {
			break
		}
		if err := canvas.Apply(f); err == nil && len(f.Rects) > 0 {
			sys.DemoUpdated()
		}
	}
	// connection ended: give the master a moment to reconnect before closing
	go func() {
		time.Sleep(3 * time.Second)
		a.mu.Lock()
		cur := a.demoSeq == seq
		if cur {
			a.demoActive, a.demoMode = false, ""
		}
		a.mu.Unlock()
		if cur {
			sys.HideDemo()
			log.Printf("demo ended")
		}
	}()
}

// ------------------------------------------------------------ discovery

func (a *Agent) discoveryLoop() {
	pc, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", proto.DiscoveryPort))
	if err != nil {
		log.Printf("discovery disabled: %v", err)
		return
	}
	defer pc.Close()
	buf := make([]byte, 2048)
	var window time.Time
	count := 0
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			select {
			case <-a.Quit:
				return
			default:
			}
			time.Sleep(time.Second)
			continue
		}
		var req proto.DiscoverRequest
		if json.Unmarshal(buf[:n], &req) != nil || req.Orbit != "discover" {
			continue
		}
		if time.Since(window) > time.Second {
			window, count = time.Now(), 0
		}
		if count++; count > 20 {
			continue // rate limit
		}
		ips, macs := proto.LocalNetInfo()
		_, trusted := a.trusted[req.KeyFP]
		rep := proto.DiscoverReply{Orbit: "here", V: 1, Nonce: req.Nonce, Name: a.host, User: sys.UserName(),
			Port: a.cfg.Port, Version: proto.Version, OS: sys.OSName(), MACs: macs, IPs: ips, Trusted: trusted}
		b, _ := json.Marshal(rep)
		pc.WriteTo(b, addr)
	}
}

// ------------------------------------------------------------ focus mode

func (a *Agent) focusLoop() {
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-a.Quit:
			return
		case <-t.C:
		}
		a.mu.Lock()
		on, list := a.focus, a.focusList
		a.mu.Unlock()
		if !on {
			continue
		}
		if killed := sys.KillProcesses(list); len(killed) > 0 {
			log.Printf("focus mode closed: %v", killed)
			a.notifyOnce("focus", 8*time.Second, a.T("focus.closed", strings.Join(uniq(killed), ", ")))
		}
	}
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[strings.ToLower(v)] {
			seen[strings.ToLower(v)] = true
			out = append(out, v)
		}
	}
	return out
}

// ------------------------------------------------------------ tray

// TrayMenu implements sys.UIHandler.
func (a *Agent) TrayMenu() []sys.MenuItem {
	a.mu.Lock()
	hand := a.hand
	a.mu.Unlock()
	var items []sys.MenuItem
	if a.cfg.AllowHand {
		label := a.T("tray.raise")
		if hand {
			label = a.T("tray.lower")
		}
		items = append(items, sys.MenuItem{ID: "hand", Label: label, Checked: hand})
	}
	if a.cfg.AllowMessages {
		items = append(items, sys.MenuItem{ID: "msg", Label: a.T("tray.message")})
	}
	items = append(items, sys.MenuItem{Separator: true}, sys.MenuItem{ID: "about", Label: a.T("tray.about")})
	if a.Portable {
		items = append(items, sys.MenuItem{ID: "exit", Label: a.T("tray.exit")})
	}
	return items
}

// TrayCommand implements sys.UIHandler.
func (a *Agent) TrayCommand(id string) {
	switch id {
	case "hand":
		a.ToggleHand()
	case "msg":
		sys.ShowCard(sys.Card{Title: a.T("msg.toTeacher"), Body: a.T("msg.toTeacherBody"), Input: true, Multiline: true,
			Buttons: []string{a.T("send"), a.T("cancel")}, Primary: 0, Cancel: 1, Glyph: sys.GlyphSend, MinWidthDIP: 460},
			func(btn int, text string) {
				if btn != 0 || text == "" {
					return
				}
				if len(text) > 2000 {
					text = text[:2000]
				}
				a.mu.Lock()
				a.inboxSeq++
				a.inbox = append(a.inbox, proto.InboxMsg{ID: a.inboxSeq, Text: text, At: time.Now().Unix()})
				if len(a.inbox) > 50 {
					a.inbox = a.inbox[len(a.inbox)-50:]
				}
				a.mu.Unlock()
				sys.Notify(a.T("product"), a.T("msg.sent"))
			})
	case "about":
		a.mu.Lock()
		n := len(a.tokens)
		a.mu.Unlock()
		status := a.T("status.none")
		if n > 0 {
			status = a.T("status.some", n)
		}
		sys.ShowCard(sys.Card{Title: a.T("about.title", proto.Version), Body: a.T("about.body", status),
			Buttons: []string{a.T("ok")}, Primary: 0, Cancel: 0, Glyph: sys.GlyphShield, MinWidthDIP: 480}, nil)
	case "exit":
		select {
		case <-a.Quit:
		default:
			close(a.Quit)
		}
	}
}

// Hotkey implements sys.UIHandler (Ctrl+Alt+H = raise/lower hand).
func (a *Agent) Hotkey(id int) {
	if id == 1 && a.cfg.AllowHand {
		a.ToggleHand()
	}
}

// ToggleHand raises or lowers the student's hand.
func (a *Agent) ToggleHand() {
	a.mu.Lock()
	a.hand = !a.hand
	if a.hand {
		a.handAt = time.Now().Unix()
	}
	up := a.hand
	a.mu.Unlock()
	if up {
		sys.Notify(a.T("product"), a.T("hand.raised"))
	} else {
		sys.Notify(a.T("product"), a.T("hand.lowered"))
	}
	a.updateTray()
}

// RaiseHandForTest is used by automated tests.
func (a *Agent) RaiseHandForTest() { a.ToggleHand() }

// AddInboxForTest is used by automated tests.
func (a *Agent) AddInboxForTest(text string) {
	a.mu.Lock()
	a.inboxSeq++
	a.inbox = append(a.inbox, proto.InboxMsg{ID: a.inboxSeq, Text: text, At: time.Now().Unix()})
	a.mu.Unlock()
}
