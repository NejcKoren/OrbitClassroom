// Package master implements Orbit Master, the teacher console. It keeps a
// connection to every student computer, serves the web based user interface
// on 127.0.0.1 and relays live screens, input and files.
package master

import (
	"crypto/ed25519"
	"encoding/json"
	"log"
	"orbit/internal/proto"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// InboxItem is a student message shown in the teacher's inbox.
type InboxItem struct {
	ID       int64  `json:"id"`
	Computer string `json:"computer"`
	Name     string `json:"name"`
	User     string `json:"user"`
	Text     string `json:"text"`
	At       int64  `json:"at"`
	Read     bool   `json:"read"`
}

// Poll is a running or finished poll.
type Poll struct {
	ID       string         `json:"id"`
	Question string         `json:"question"`
	Options  []string       `json:"options"`
	Targets  []string       `json:"targets"`
	Started  int64          `json:"started"`
	Active   bool           `json:"active"`
	Answers  map[string]int `json:"answers"`
}

// Master is the running teacher console.
type Master struct {
	dir          string
	cfgPath      string
	name         string
	SessionToken string
	Quit         chan struct{}
	quitOnce     sync.Once
	AgentExe     []byte // embedded agent (for the deployment package)
	ExeDir       string

	mu      sync.RWMutex
	cfg     *Config
	priv    ed25519.PrivateKey
	conns   map[string]*Conn
	view    map[string]bool
	fast    map[string]bool
	viewW   int
	inbox   []InboxItem
	inboxID int64
	poll    *Poll
	demo    *Demo

	subMu    sync.Mutex
	subs     map[chan sseMsg]bool
	everSub  bool
	noSubsAt time.Time
	dirty    chan struct{}
	lastJSON string
}

type sseMsg struct {
	event string
	data  []byte
}

// New loads (or creates) the configuration in dir.
func New(dir string) (*Master, error) {
	m := &Master{
		dir: dir, cfgPath: filepath.Join(dir, "master.json"), SessionToken: proto.RandomToken(24),
		Quit: make(chan struct{}), conns: map[string]*Conn{}, view: map[string]bool{}, fast: map[string]bool{},
		viewW: 320, subs: map[chan sseMsg]bool{}, dirty: make(chan struct{}, 1), noSubsAt: time.Now(),
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	cfg, err := loadConfig(m.cfgPath)
	if err != nil && !os.IsNotExist(err) {
		// keep a copy of a broken file instead of silently overwriting it
		os.Rename(m.cfgPath, m.cfgPath+".broken-"+time.Now().Format("20060102-150405"))
		log.Printf("config unreadable (%v), starting fresh", err)
	}
	m.cfg = cfg
	if cfg.Secret != "" {
		if k, err := proto.ParseSecretKey(cfg.Secret); err == nil {
			m.priv = k
		}
	}
	if m.priv == nil {
		m.priv = proto.NewTeacherKey()
		cfg.Secret = proto.SecretKeyString(m.priv)
	}
	if len(cfg.Rooms) == 0 {
		name := "Classroom 1"
		if strings.HasPrefix(systemLanguage(), "sl") {
			name = "Učilnica 1"
		}
		cfg.Rooms = append(cfg.Rooms, &Room{ID: proto.RandomID(), Name: name})
	}
	host, _ := os.Hostname()
	uname := ""
	if u, err := user.Current(); err == nil {
		uname = u.Username
		if i := strings.LastIndex(uname, `\`); i >= 0 {
			uname = uname[i+1:]
		}
	}
	m.name = strings.TrimSpace(host + " / " + uname)
	if exe, err := os.Executable(); err == nil {
		m.ExeDir = filepath.Dir(exe)
	}
	if err := m.saveLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

// Start connects to all computers and starts the state broadcaster.
func (m *Master) Start() {
	m.mu.Lock()
	for _, pc := range m.cfg.Computers {
		m.startConn(pc)
	}
	m.mu.Unlock()
	go m.broadcaster()
}

// Stop shuts everything down.
func (m *Master) Stop() {
	m.stopDemo()
	m.mu.Lock()
	for _, c := range m.conns {
		close(c.stop)
	}
	m.conns = map[string]*Conn{}
	m.mu.Unlock()
}

// RequestQuit asks the program to exit.
func (m *Master) RequestQuit() { m.quitOnce.Do(func() { close(m.Quit) }) }

func (m *Master) startConn(pc *Computer) {
	c := newConn(m, pc)
	m.conns[pc.ID] = c
	go c.run()
}

func (m *Master) saveLocked() error { return m.cfg.save(m.cfgPath) }

func (m *Master) save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

func (m *Master) key() ed25519.PrivateKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.priv
}

func (m *Master) conn(id string) *Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conns[id]
}

func (m *Master) settings() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.Settings
}

// viewParams tells a connection whether its thumbnail is visible in the UI.
func (m *Master) viewParams(id string) (bool, int, time.Duration) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	interval := time.Duration(m.cfg.Settings.ThumbInterval) * time.Millisecond
	w := m.viewW
	if m.fast[id] {
		interval = min(interval, 350*time.Millisecond)
		w = max(w, 960)
	}
	return m.view[id], w, interval
}

func (m *Master) markDirty() {
	select {
	case m.dirty <- struct{}{}:
	default:
	}
}

// ------------------------------------------------------------ status handling

func (m *Master) statusUpdate(c *Conn, st *proto.Status, raised bool, fresh []proto.InboxMsg) {
	m.mu.Lock()
	pc := m.cfg.computer(c.id)
	name := c.id
	if pc != nil {
		name = pc.Name
		if len(st.MACs) > 0 && strings.Join(pc.MACs, ",") != strings.Join(st.MACs, ",") {
			pc.MACs = append([]string(nil), st.MACs...) // remembered for Wake-on-LAN
			m.saveLocked()
		}
	}
	if m.poll != nil && m.poll.Active && st.Poll != nil && st.Poll.ID == m.poll.ID && st.Poll.Answer >= 0 {
		if old, ok := m.poll.Answers[c.id]; !ok || old != st.Poll.Answer {
			m.poll.Answers[c.id] = st.Poll.Answer
			m.markDirty()
		}
	}
	for _, msg := range fresh {
		m.inboxID++
		m.inbox = append(m.inbox, InboxItem{ID: m.inboxID, Computer: c.id, Name: name, User: st.User, Text: msg.Text, At: msg.At})
	}
	if len(m.inbox) > 500 {
		m.inbox = m.inbox[len(m.inbox)-500:]
	}
	m.mu.Unlock()
	if raised {
		m.toast("hand", c.id, name, st.User, "")
	}
	for _, msg := range fresh {
		m.toast("inbox", c.id, name, st.User, msg.Text)
	}
	if len(fresh) > 0 {
		m.markDirty()
	}
}

func (m *Master) toast(kind, id, name, user, text string) {
	b, _ := json.Marshal(map[string]string{"kind": kind, "id": id, "name": name, "user": user, "text": text})
	m.publish(sseMsg{"toast", b})
}

// ------------------------------------------------------------ state for the UI

type compView struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Host     string          `json:"host"`
	Port     int             `json:"port"`
	Room     string          `json:"room"`
	Order    int             `json:"order"`
	MACs     []string        `json:"macs"`
	State    string          `json:"state"`
	Err      string          `json:"err,omitempty"`
	ThumbVer int             `json:"thumbVer"`
	St       *proto.Status   `json:"st,omitempty"`
	Monitors []proto.Monitor `json:"-"`
}

type pollView struct {
	*Poll
	Counts   []int `json:"counts"`
	Answered int   `json:"answered"`
	Total    int   `json:"total"`
}

type stateView struct {
	Computers []compView  `json:"computers"`
	Rooms     []*Room     `json:"rooms"`
	Inbox     []InboxItem `json:"inbox"`
	Poll      *pollView   `json:"poll"`
	Demo      *demoView   `json:"demo"`
}

func (m *Master) snapshot() []byte {
	m.mu.RLock()
	sv := stateView{Rooms: m.cfg.Rooms, Inbox: m.inbox}
	if sv.Inbox == nil {
		sv.Inbox = []InboxItem{}
	}
	showAct := m.cfg.Settings.ShowActivity
	for _, pc := range m.cfg.Computers {
		cv := compView{ID: pc.ID, Name: pc.Name, Host: pc.Host, Port: pc.Port, Room: pc.Room, Order: pc.Order, MACs: pc.MACs, State: "offline"}
		if c := m.conns[pc.ID]; c != nil {
			c.mu.Lock()
			cv.State, cv.Err, cv.ThumbVer = c.state, c.errMsg, c.thumbVer
			if c.status != nil && c.state == "online" {
				st := *c.status
				st.Inbox = nil
				if !showAct {
					st.ActiveApp, st.ActiveTitle, st.IdleSec = "", "", 0
				}
				cv.St = &st
			}
			c.mu.Unlock()
		}
		sv.Computers = append(sv.Computers, cv)
	}
	if sv.Computers == nil {
		sv.Computers = []compView{}
	}
	if m.poll != nil {
		pv := &pollView{Poll: m.poll, Counts: make([]int, len(m.poll.Options)), Total: len(m.poll.Targets)}
		for _, a := range m.poll.Answers {
			if a >= 0 && a < len(pv.Counts) {
				pv.Counts[a]++
				pv.Answered++
			}
		}
		sv.Poll = pv
	}
	if m.demo != nil {
		sv.Demo = m.demo.view()
	}
	m.mu.RUnlock()
	sort.SliceStable(sv.Computers, func(i, j int) bool {
		if sv.Computers[i].Order != sv.Computers[j].Order {
			return sv.Computers[i].Order < sv.Computers[j].Order
		}
		return naturalLess(sv.Computers[i].Name, sv.Computers[j].Name)
	})
	b, _ := json.Marshal(sv)
	return b
}

// naturalLess sorts "PC2" before "PC10".
func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		ad, bd := digits(a), digits(b)
		if ad != "" && bd != "" {
			if len(strings.TrimLeft(ad, "0")) != len(strings.TrimLeft(bd, "0")) {
				return len(strings.TrimLeft(ad, "0")) < len(strings.TrimLeft(bd, "0"))
			}
			if ad != bd {
				return strings.TrimLeft(ad, "0") < strings.TrimLeft(bd, "0")
			}
			a, b = a[len(ad):], b[len(bd):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func (m *Master) broadcaster() {
	t := time.NewTicker(350 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-m.Quit:
			return
		case <-m.dirty:
		case <-t.C:
		}
		b := m.snapshot()
		if string(b) == m.lastJSON {
			continue
		}
		m.lastJSON = string(b)
		m.publish(sseMsg{"state", b})
		time.Sleep(120 * time.Millisecond) // coalesce bursts
	}
}

func (m *Master) publish(msg sseMsg) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subs {
		select {
		case ch <- msg:
		default: // slow client: it gets the next full state anyway
		}
	}
}

func (m *Master) subscribe() chan sseMsg {
	ch := make(chan sseMsg, 16)
	m.subMu.Lock()
	m.subs[ch] = true
	m.everSub = true
	m.subMu.Unlock()
	return ch
}

func (m *Master) unsubscribe(ch chan sseMsg) {
	m.subMu.Lock()
	delete(m.subs, ch)
	if len(m.subs) == 0 {
		m.noSubsAt = time.Now()
	}
	m.subMu.Unlock()
}

// WatchClients quits the program when the UI window has been closed.
func (m *Master) WatchClients(grace time.Duration) {
	start := time.Now()
	for {
		select {
		case <-m.Quit:
			return
		case <-time.After(2 * time.Second):
		}
		m.subMu.Lock()
		n, ever, since := len(m.subs), m.everSub, time.Since(m.noSubsAt)
		m.subMu.Unlock()
		if n == 0 && ((ever && since > grace) || (!ever && time.Since(start) > 5*time.Minute)) {
			log.Printf("user interface closed - exiting")
			m.RequestQuit()
			return
		}
	}
}
