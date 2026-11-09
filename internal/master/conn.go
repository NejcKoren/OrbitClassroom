package master

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"orbit/internal/proto"
	"strconv"
	"sync"
	"time"
)

// Conn keeps one student computer connected: it authenticates, polls status
// and thumbnails, and executes commands.
type Conn struct {
	m  *Master
	id string

	mu       sync.Mutex
	host     string
	port     int
	state    string // offline | connecting | online | denied | incompatible
	errMsg   string
	token    string
	status   *proto.Status
	statusAt time.Time
	thumb    []byte
	etag     string
	thumbVer int
	lastHand bool
	seen     map[int64]bool

	client       *http.Client
	streamClient *http.Client
	wake         chan struct{}
	stop         chan struct{}
}

var errUnauthorized = errors.New("unauthorized")

func newConn(m *Master, pc *Computer) *Conn {
	c := &Conn{m: m, id: pc.ID, host: pc.Host, port: pc.Port, state: "connecting",
		wake: make(chan struct{}, 1), stop: make(chan struct{}), seen: map[int64]bool{}}
	tlsCfg := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} // identity is verified by the signed handshake
	dial := (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	c.client = &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{
		TLSClientConfig: tlsCfg, DialContext: dial, MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second,
		TLSHandshakeTimeout: 4 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
	}}
	c.streamClient = &http.Client{Transport: &http.Transport{
		TLSClientConfig: tlsCfg, DialContext: dial, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second,
		TLSHandshakeTimeout: 4 * time.Second,
	}}
	return c
}

func (c *Conn) base() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	port := c.port
	if port == 0 {
		port = proto.AgentPort
	}
	return "https://" + net.JoinHostPort(c.host, strconv.Itoa(port))
}

func (c *Conn) setAddr(host string, port int) {
	c.mu.Lock()
	changed := c.host != host || c.port != port
	c.host, c.port = host, port
	if changed {
		c.token = ""
	}
	c.mu.Unlock()
	c.poke()
}

func (c *Conn) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Conn) setState(state, msg string) {
	c.mu.Lock()
	changed := c.state != state || c.errMsg != msg
	c.state, c.errMsg = state, msg
	if state != "online" {
		c.token = ""
	}
	c.mu.Unlock()
	if changed {
		c.m.markDirty()
	}
}

func (c *Conn) online() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state == "online" && c.token != ""
}

// authenticate performs the hello / signed auth handshake.
func (c *Conn) authenticate() error {
	base := c.base()
	resp, err := c.client.Get(base + "/api/v1/hello")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return errors.New("no TLS")
	}
	fp := proto.CertFingerprint(resp.TLS.PeerCertificates[0].Raw)
	var h proto.Hello
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&h); err != nil {
		return errors.New("not an Orbit Agent")
	}
	if h.Proto != proto.ProtoVersion {
		c.setState("incompatible", fmt.Sprintf("agent protocol %d, master %d (agent %s)", h.Proto, proto.ProtoVersion, h.Version))
		return errIncompatible
	}
	priv := c.m.key()
	req := proto.AuthRequest{Nonce: h.Nonce, Pub: proto.PublicKeyString(priv), Sig: proto.SignAuth(priv, h.Nonce, fp), Master: c.m.name}
	b, _ := json.Marshal(req)
	r2, err := c.client.Post(base+"/api/v1/auth", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer r2.Body.Close()
	if r2.TLS == nil || len(r2.TLS.PeerCertificates) == 0 || proto.CertFingerprint(r2.TLS.PeerCertificates[0].Raw) != fp {
		return errors.New("agent certificate changed during login")
	}
	if r2.StatusCode == http.StatusForbidden {
		var e map[string]string
		json.NewDecoder(io.LimitReader(r2.Body, 4096)).Decode(&e)
		c.setState("denied", e["error"])
		return errDenied
	}
	if r2.StatusCode != 200 {
		return fmt.Errorf("auth: HTTP %d", r2.StatusCode)
	}
	var ar proto.AuthResponse
	if err := json.NewDecoder(r2.Body).Decode(&ar); err != nil || ar.Token == "" {
		return errors.New("bad auth response")
	}
	c.mu.Lock()
	c.token = ar.Token
	c.state, c.errMsg = "online", ""
	c.mu.Unlock()
	c.m.markDirty()
	return nil
}

var (
	errDenied       = errors.New("denied")
	errIncompatible = errors.New("incompatible")
)

// do performs an authenticated request.
func (c *Conn) do(ctx context.Context, client *http.Client, method, path string, body io.Reader, ctype string, hdr map[string]string) (*http.Response, error) {
	c.mu.Lock()
	tok := c.token
	c.mu.Unlock()
	if tok == "" {
		return nil, errors.New("not connected")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		c.mu.Lock()
		if c.token == tok {
			c.token = ""
		}
		c.mu.Unlock()
		c.poke()
		return nil, errUnauthorized
	}
	return resp, nil
}

// Cmd sends a command to the agent.
func (c *Conn) Cmd(cmd proto.Command) error {
	b, _ := json.Marshal(cmd)
	resp, err := c.do(context.Background(), c.client, "POST", "/api/v1/cmd", bytes.NewReader(b), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var r proto.CommandResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&r); err != nil {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if !r.OK {
		return errors.New(r.Error)
	}
	return nil
}

func (c *Conn) fetchStatus() error {
	resp, err := c.do(context.Background(), c.client, "GET", "/api/v1/status", nil, "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var st proto.Status
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&st); err != nil {
		return err
	}
	c.mu.Lock()
	old := c.status
	c.status = &st
	c.statusAt = time.Now()
	raised := st.Hand && !c.lastHand
	c.lastHand = st.Hand
	var fresh []proto.InboxMsg
	var maxID int64
	for _, msg := range st.Inbox {
		if !c.seen[msg.ID] {
			c.seen[msg.ID] = true
			fresh = append(fresh, msg)
		}
		maxID = max(maxID, msg.ID)
	}
	c.mu.Unlock()
	if old == nil || !sameStatus(old, &st) {
		c.m.markDirty()
	}
	c.m.statusUpdate(c, &st, raised, fresh)
	if maxID > 0 {
		go c.Cmd(proto.Command{Action: "inbox_ack", Upto: maxID})
	}
	return nil
}

func sameStatus(a, b *proto.Status) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func (c *Conn) fetchThumb(width int) error {
	c.mu.Lock()
	etag := c.etag
	c.mu.Unlock()
	hdr := map[string]string{}
	if etag != "" {
		hdr["If-None-Match"] = etag
	}
	resp, err := c.do(context.Background(), c.client, "GET", fmt.Sprintf("/api/v1/thumb?w=%d&q=62", width), nil, "", hdr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified || resp.StatusCode == http.StatusServiceUnavailable {
		return nil
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("thumb: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.thumb = data
	c.etag = resp.Header.Get("ETag")
	c.thumbVer++
	c.mu.Unlock()
	c.m.markDirty()
	return nil
}

// run is the connection loop.
func (c *Conn) run() {
	backoff := 2 * time.Second
	var nextStatus, nextThumb time.Time
	for {
		select {
		case <-c.stop:
			return
		default:
		}
		c.mu.Lock()
		authed := c.token != ""
		c.mu.Unlock()
		if !authed {
			if c.state != "denied" && c.state != "incompatible" {
				c.setState("connecting", "")
			}
			err := c.authenticate()
			if err != nil {
				switch err {
				case errDenied:
					backoff = 15 * time.Second
				case errIncompatible:
					backoff = 60 * time.Second
				default:
					c.setState("offline", shortErr(err))
					backoff = min(backoff*3/2, 15*time.Second)
				}
				c.sleep(backoff)
				continue
			}
			backoff = 2 * time.Second
			nextStatus, nextThumb = time.Time{}, time.Time{}
		}
		visible, width, interval := c.m.viewParams(c.id)
		now := time.Now()
		if !now.Before(nextStatus) {
			if err := c.fetchStatus(); err != nil {
				if err != errUnauthorized {
					c.setState("offline", shortErr(err))
					c.sleep(backoff)
				}
				continue
			}
			if visible {
				nextStatus = now.Add(1500 * time.Millisecond)
			} else {
				nextStatus = now.Add(5 * time.Second)
			}
		}
		if visible && !now.Before(nextThumb) {
			if err := c.fetchThumb(width); err != nil && err != errUnauthorized {
				c.setState("offline", shortErr(err))
				continue
			}
			nextThumb = time.Now().Add(interval)
		}
		wait := time.Until(nextStatus)
		if visible {
			wait = min(wait, time.Until(nextThumb))
		}
		c.sleep(max(wait, 30*time.Millisecond))
	}
}

func (c *Conn) sleep(d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-c.wake:
	case <-c.stop:
	}
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	var oe *net.OpError
	if errors.As(err, &oe) {
		if oe.Timeout() {
			return "timeout"
		}
		return oe.Err.Error()
	}
	s := err.Error()
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}
