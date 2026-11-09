// Package proto contains everything that Orbit Master and Orbit Agent share:
// wire types, key encoding, authentication helpers, the delta-tile screen
// stream codec and the LAN discovery packets.
//
// Changing anything here changes the network protocol. Bump ProtoVersion when
// you make an incompatible change and add a new feature string (see Features)
// for compatible additions so older peers keep working.
package proto

const (
	Product       = "Orbit Classroom"
	ProtoVersion  = 1
	AgentPort     = 21400 // TCP, TLS - agent API
	DiscoveryPort = 21401 // UDP - LAN discovery
)

// Version is overwritten at build time with -ldflags "-X orbit/internal/proto.Version=x.y.z".
var Version = "1.0.0"

// Features advertised by an agent. The master enables UI actions based on these,
// which allows new features to be added without breaking older agents.
var Features = []string{
	"thumb", "stream", "input", "lock", "lock-input", "unlock", "message",
	"demo", "demo-window", "open_url", "run", "power", "file", "screenshot",
	"focus", "poll", "hand", "inbox", "monitors", "text-input",
}

// Monitor describes one physical display in virtual-desktop coordinates.
type Monitor struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	W       int  `json:"w"`
	H       int  `json:"h"`
	Primary bool `json:"primary"`
}

// Hello is returned by the unauthenticated GET /api/v1/hello endpoint.
type Hello struct {
	Product string `json:"product"`
	Proto   int    `json:"proto"`
	Version string `json:"version"`
	Name    string `json:"name"`
	Nonce   string `json:"nonce"`
}

// AuthRequest proves that the caller owns a trusted teacher key.
type AuthRequest struct {
	Nonce  string `json:"nonce"`
	Pub    string `json:"pub"`    // OPK1-... public key
	Sig    string `json:"sig"`    // base64 Ed25519 signature over AuthPayload
	Master string `json:"master"` // human readable teacher computer name
}

// AuthResponse carries the session token used for all further requests.
type AuthResponse struct {
	Token string `json:"token"`
	TTL   int    `json:"ttl"`
}

// PollState is the agent's view of the currently running poll.
type PollState struct {
	ID     string `json:"id"`
	Answer int    `json:"answer"` // -1 = not answered yet
	At     int64  `json:"at"`
}

// InboxMsg is a message a student sent to the teacher.
type InboxMsg struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// Status is returned by GET /api/v1/status.
type Status struct {
	Host        string     `json:"host"`
	User        string     `json:"user"`
	OS          string     `json:"os"`
	Version     string     `json:"version"`
	Proto       int        `json:"proto"`
	IPs         []string   `json:"ips"`
	MACs        []string   `json:"macs"`
	ScreenW     int        `json:"sw"`
	ScreenH     int        `json:"sh"`
	Monitors    []Monitor  `json:"monitors"`
	ActiveApp   string     `json:"app"`
	ActiveTitle string     `json:"title"`
	IdleSec     int        `json:"idle"`
	Locked      bool       `json:"locked"`
	LockMode    string     `json:"lockMode,omitempty"`
	Demo        bool       `json:"demo"`
	DemoMode    string     `json:"demoMode,omitempty"`
	Hand        bool       `json:"hand"`
	HandAt      int64      `json:"handAt,omitempty"`
	Focus       bool       `json:"focus"`
	Viewers     int        `json:"viewers"`
	Controlled  bool       `json:"controlled"`
	Poll        *PollState `json:"poll,omitempty"`
	Inbox       []InboxMsg `json:"inbox,omitempty"`
	Features    []string   `json:"features"`
	Uptime      int64      `json:"uptime"`
	Lang        string     `json:"lang"`
	AllowCtl    bool       `json:"allowControl"`
}

// Command is sent to POST /api/v1/cmd. Only the fields relevant for the
// action are filled.
//
// Actions: lock, unlock, message, open_url, run, power, focus, poll,
// poll_close, hand_lower, inbox_ack, demo_stop.
type Command struct {
	Action  string   `json:"action"`
	Mode    string   `json:"mode,omitempty"`
	Title   string   `json:"title,omitempty"`
	Text    string   `json:"text,omitempty"`
	URL     string   `json:"url,omitempty"`
	Program string   `json:"program,omitempty"`
	Args    string   `json:"args,omitempty"`
	Delay   int      `json:"delay,omitempty"`
	List    []string `json:"list,omitempty"`
	Enabled bool     `json:"enabled,omitempty"`
	ID      string   `json:"id,omitempty"`
	Options []string `json:"options,omitempty"`
	Upto    int64    `json:"upto,omitempty"`
}

// CommandResult is the reply to a Command.
type CommandResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// InputEvent is one remote-control input event.
//
//	t = "mm" mouse move (X,Y normalised 0..1 within the viewed area)
//	t = "md"/"mu" mouse button down/up (B: 0 left, 1 middle, 2 right)
//	t = "wh" vertical wheel, "hw" horizontal wheel (D = delta, 120 per notch)
//	t = "kd"/"ku" key down/up (C = KeyboardEvent.code, layout independent)
//	t = "tx" type unicode text (S)
//	t = "combo" special combination (S = win, alttab, ctrlesc, ctrlshiftesc)
type InputEvent struct {
	T string  `json:"t"`
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`
	B int     `json:"b,omitempty"`
	D int     `json:"d,omitempty"`
	C string  `json:"c,omitempty"`
	S string  `json:"s,omitempty"`
}
