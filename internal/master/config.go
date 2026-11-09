package master

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Computer is one student computer known to the master.
type Computer struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Host  string   `json:"host"`
	Port  int      `json:"port"`
	MACs  []string `json:"macs,omitempty"`
	Room  string   `json:"room"`
	Order int      `json:"order"`
}

// Room groups computers (a classroom / location).
type Room struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
}

// Fav is a saved website or program.
type Fav struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Args  string `json:"args,omitempty"`
}

// CustomAction is a user defined toolbar action (extensibility without recompiling).
type CustomAction struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"` // run | url | message
	Value string `json:"value"`
	Args  string `json:"args,omitempty"`
}

// Settings are the teacher's preferences.
type Settings struct {
	Language      string         `json:"language"` // "" = automatic
	Theme         string         `json:"theme"`    // dark | light | auto
	Accent        string         `json:"accent"`
	ThumbInterval int            `json:"thumbInterval"` // ms
	ViewQuality   int            `json:"viewQuality"`
	ViewMaxW      int            `json:"viewMaxW"`
	ViewFPS       int            `json:"viewFps"`
	DemoQuality   int            `json:"demoQuality"`
	DemoMaxW      int            `json:"demoMaxW"`
	DemoFPS       int            `json:"demoFps"`
	LockTitle     string         `json:"lockTitle"`
	LockText      string         `json:"lockText"`
	ShowActivity  bool           `json:"showActivity"`
	ConfirmPower  bool           `json:"confirmPower"`
	Sounds        bool           `json:"sounds"`
	Websites      []Fav          `json:"websites"`
	Programs      []Fav          `json:"programs"`
	CustomActions []CustomAction `json:"customActions"`
	FocusList     []string       `json:"focusList"`
	APIToken      string         `json:"apiToken"`
	ScreenshotDir string         `json:"screenshotDir"`
	Onboarded     bool           `json:"onboarded"`
	TileSize      int            `json:"tileSize"`
}

// Config is everything persisted in master.json.
type Config struct {
	Version   int         `json:"version"`
	Secret    string      `json:"teacherKey"` // OTK1-... (keep private!)
	Rooms     []*Room     `json:"rooms"`
	Computers []*Computer `json:"computers"`
	Settings  Settings    `json:"settings"`
}

func defaultSettings() Settings {
	return Settings{
		Theme: "dark", Accent: "indigo", ThumbInterval: 1000,
		ViewQuality: 70, ViewMaxW: 1920, ViewFPS: 20,
		DemoQuality: 70, DemoMaxW: 1920, DemoFPS: 15,
		ShowActivity: true, ConfirmPower: true, Sounds: true, TileSize: 300,
		Websites: []Fav{
			{Name: "Wikipedia", Value: "https://www.wikipedia.org"},
			{Name: "Google Classroom", Value: "https://classroom.google.com"},
			{Name: "Microsoft Teams", Value: "https://teams.microsoft.com"},
		},
		Programs: []Fav{
			{Name: "Notepad", Value: "notepad.exe"},
			{Name: "Calculator", Value: "calc.exe"},
			{Name: "Paint", Value: "mspaint.exe"},
			{Name: "Word", Value: "winword.exe"},
		},
	}
}

func loadConfig(p string) (*Config, error) {
	cfg := &Config{Version: 1, Settings: defaultSettings()}
	b, err := os.ReadFile(p)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, cfg); err != nil {
		return cfg, err
	}
	s := &cfg.Settings
	d := defaultSettings()
	if s.ThumbInterval < 200 {
		s.ThumbInterval = d.ThumbInterval
	}
	if s.ViewQuality <= 0 {
		s.ViewQuality = d.ViewQuality
	}
	if s.ViewMaxW <= 0 {
		s.ViewMaxW = d.ViewMaxW
	}
	if s.ViewFPS <= 0 {
		s.ViewFPS = d.ViewFPS
	}
	if s.DemoQuality <= 0 {
		s.DemoQuality = d.DemoQuality
	}
	if s.DemoMaxW <= 0 {
		s.DemoMaxW = d.DemoMaxW
	}
	if s.DemoFPS <= 0 {
		s.DemoFPS = d.DemoFPS
	}
	if s.TileSize <= 0 {
		s.TileSize = d.TileSize
	}
	return cfg, nil
}

func (c *Config) save(p string) error {
	sort.SliceStable(c.Rooms, func(i, j int) bool { return c.Rooms[i].Order < c.Rooms[j].Order })
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (c *Config) computer(id string) *Computer {
	for _, pc := range c.Computers {
		if pc.ID == id {
			return pc
		}
	}
	return nil
}

func (c *Config) room(id string) *Room {
	for _, r := range c.Rooms {
		if r.ID == id {
			return r
		}
	}
	return nil
}
