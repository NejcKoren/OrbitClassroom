package agent

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"orbit/internal/proto"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Config is the agent configuration (agent.json / orbit-agent.json).
type Config struct {
	TrustedKeys    []string `json:"trusted_keys"`
	Port           int      `json:"port"`
	Discovery      bool     `json:"discovery"`
	Language       string   `json:"language"`        // auto | en | sl | ...
	NotifyOnView   bool     `json:"notify_on_view"`  // tell students when they are viewed/controlled
	AllowControl   bool     `json:"allow_control"`   // allow remote mouse/keyboard
	ShowTray       bool     `json:"show_tray"`       // tray icon
	AllowHand      bool     `json:"allow_hand"`      // raise hand feature
	AllowMessages  bool     `json:"allow_messages"`  // student -> teacher messages
	ShareActivity  bool     `json:"share_activity"`  // report active app + idle time
	ReceivedFolder string   `json:"received_folder"` // default: Desktop\Orbit

	path string
}

// DefaultConfig returns the recommended defaults.
func DefaultConfig() *Config {
	return &Config{
		Port: proto.AgentPort, Discovery: true, Language: "auto", NotifyOnView: true,
		AllowControl: true, ShowTray: true, AllowHand: true, AllowMessages: true,
		ShareActivity: true,
	}
}

// ProgramDataDir is the machine wide folder used by an installed agent.
func ProgramDataDir() string {
	if d := os.Getenv("ORBIT_AGENT_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, "Orbit Classroom")
	}
	return "/etc/orbit-classroom"
}

// UserDir is the per-user folder (logs, portable mode config).
func UserDir() string {
	if d := os.Getenv("ORBIT_AGENT_DIR"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "Orbit Classroom")
}

// ExeDir is the folder containing the running executable.
func ExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// ConfigCandidates lists config files in priority order.
func ConfigCandidates(explicit string) []string {
	var c []string
	if explicit != "" {
		c = append(c, explicit)
	}
	return append(c,
		filepath.Join(ProgramDataDir(), "agent.json"),
		filepath.Join(ExeDir(), "orbit-agent.json"),
		filepath.Join(UserDir(), "agent.json"),
	)
}

// LoadConfig returns the first readable config with at least one key.
func LoadConfig(explicit string) (*Config, error) {
	for _, p := range ConfigCandidates(explicit) {
		cfg, err := ReadConfig(p)
		if err == nil && len(cfg.TrustedKeys) > 0 {
			return cfg, nil
		}
	}
	return nil, errors.New("no configuration with a classroom key found")
}

// ReadConfig reads one config file, filling in defaults.
func ReadConfig(p string) (*Config, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	if cfg.Port == 0 {
		cfg.Port = proto.AgentPort
	}
	cfg.path = p
	return cfg, nil
}

// Save writes the config to p.
func (c *Config) Save(p string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	c.path = p
	return os.Rename(tmp, p)
}

// Path returns where the config was loaded from.
func (c *Config) Path() string { return c.path }

// LoadOrCreateCert loads agent-cert.pem/agent-key.pem from the first dir that
// has them, or creates a new self-signed certificate in the first writable dir.
func LoadOrCreateCert(dirs ...string) (tls.Certificate, error) {
	for _, d := range dirs {
		c, err := tls.LoadX509KeyPair(filepath.Join(d, "agent-cert.pem"), filepath.Join(d, "agent-key.pem"))
		if err == nil {
			return c, nil
		}
	}
	var lastErr error
	for _, d := range dirs {
		if err := CreateCert(d); err != nil {
			lastErr = err
			continue
		}
		return tls.LoadX509KeyPair(filepath.Join(d, "agent-cert.pem"), filepath.Join(d, "agent-key.pem"))
	}
	return tls.Certificate{}, lastErr
}

// CreateCert writes a new ECDSA P-256 self-signed certificate into dir.
func CreateCert(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host, Organization: []string{"Orbit Classroom Agent"}},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().AddDate(30, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "agent-cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}
