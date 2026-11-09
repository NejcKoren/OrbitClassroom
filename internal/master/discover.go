package master

import (
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"orbit/internal/proto"
	"sort"
	"strings"
	"sync"
	"time"
)

// Found is one computer found by discovery.
type Found struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip"`
	Port    int      `json:"port"`
	User    string   `json:"user"`
	Version string   `json:"version"`
	OS      string   `json:"os"`
	MACs    []string `json:"macs"`
	IPs     []string `json:"ips,omitempty"`
	Trusted *bool    `json:"trusted"` // nil = unknown (found by scan)
	Known   string   `json:"known"`   // id of an existing computer with this address
	Via     string   `json:"via"`
}

// Discover finds Orbit Agents on the local network: UDP broadcast and,
// optionally, a TCP scan of the local /24 networks (when broadcasts are blocked).
func (m *Master) Discover(scan bool) []Found {
	res := map[string]*Found{}
	var mu sync.Mutex
	add := func(f *Found) {
		mu.Lock()
		defer mu.Unlock()
		// one agent may answer from several addresses (multiple network cards):
		// identify it by name, port and MAC address and prefer a non-loopback address
		k := fmt.Sprintf("%s:%d", f.IP, f.Port)
		if len(f.MACs) > 0 {
			k = fmt.Sprintf("%s|%d|%s", strings.ToLower(f.Name), f.Port, f.MACs[0])
		}
		if old, ok := res[k]; ok {
			if ip := net.ParseIP(old.IP); ip != nil && ip.IsLoopback() && !net.ParseIP(f.IP).IsLoopback() {
				old.IP = f.IP
			}
			if old.Trusted == nil {
				old.Trusted = f.Trusted
			}
			if old.User == "" {
				old.User = f.User
			}
			return
		}
		res[k] = f
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); m.broadcastDiscover(add) }()
	if scan {
		wg.Add(1)
		go func() { defer wg.Done(); m.scanSubnets(add) }()
	}
	wg.Wait()

	m.mu.RLock()
	for _, f := range res {
		for _, pc := range m.cfg.Computers {
			port := pc.Port
			if port == 0 {
				port = proto.AgentPort
			}
			hostMatch := strings.EqualFold(pc.Host, f.IP) || strings.EqualFold(pc.Host, f.Name)
			for _, ip := range f.IPs {
				hostMatch = hostMatch || strings.EqualFold(pc.Host, ip)
			}
			if ip := net.ParseIP(pc.Host); ip != nil && ip.IsLoopback() && len(f.MACs) > 0 && len(pc.MACs) > 0 && pc.MACs[0] == f.MACs[0] {
				hostMatch = true
			}
			if hostMatch && port == f.Port {
				f.Known = pc.ID
			}
		}
	}
	m.mu.RUnlock()
	out := make([]Found, 0, len(res))
	for _, f := range res {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return naturalLess(out[i].Name, out[j].Name) })
	return out
}

func (m *Master) keyFP() string {
	return proto.Fingerprint(m.key().Public().(ed25519.PublicKey))
}

func (m *Master) broadcastDiscover(add func(*Found)) {
	pc, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return
	}
	defer pc.Close()
	nonce := proto.RandomToken(9)
	b, _ := json.Marshal(proto.DiscoverRequest{Orbit: "discover", V: 1, Nonce: nonce, KeyFP: m.keyFP()})
	targets := append(proto.BroadcastAddrs(), net.IPv4(127, 0, 0, 1))
	send := func() {
		for _, ip := range targets {
			pc.WriteTo(b, &net.UDPAddr{IP: ip, Port: proto.DiscoveryPort})
		}
	}
	send()
	deadline := time.Now().Add(2500 * time.Millisecond)
	resent := false
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		if !resent && time.Until(deadline) < 1500*time.Millisecond {
			send() // UDP can be lost: send twice
			resent = true
		}
		pc.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		var r proto.DiscoverReply
		if json.Unmarshal(buf[:n], &r) != nil || r.Orbit != "here" || r.Nonce != nonce {
			continue
		}
		ip := addr.(*net.UDPAddr).IP.String()
		t := r.Trusted
		add(&Found{Name: r.Name, IP: ip, IPs: r.IPs, Port: r.Port, User: r.User, Version: r.Version, OS: r.OS, MACs: r.MACs, Trusted: &t, Via: "broadcast"})
	}
}

func (m *Master) scanSubnets(add func(*Found)) {
	client := &http.Client{Timeout: 900 * time.Millisecond, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext:     (&net.Dialer{Timeout: 600 * time.Millisecond}).DialContext,
	}}
	sem := make(chan struct{}, 128)
	var wg sync.WaitGroup
	for _, base := range proto.LocalSubnets24() {
		for i := 1; i < 255; i++ {
			ip := net.IPv4(base[0], base[1], base[2], byte(i)).String()
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				resp, err := client.Get(fmt.Sprintf("https://%s:%d/api/v1/hello", ip, proto.AgentPort))
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var h proto.Hello
				if json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&h) != nil || h.Product != proto.Product {
					return
				}
				add(&Found{Name: h.Name, IP: ip, Port: proto.AgentPort, Version: h.Version, Via: "scan"})
			}()
		}
	}
	wg.Wait()
}

// WakeOnLAN sends magic packets for the given computers.
func (m *Master) WakeOnLAN(ids []string) map[string]string {
	res := map[string]string{}
	m.mu.RLock()
	var macs [][2]string
	for _, id := range ids {
		pc := m.cfg.computer(id)
		if pc == nil {
			continue
		}
		if len(pc.MACs) == 0 {
			res[id] = "no MAC address known yet (the computer must have been online once)"
			continue
		}
		for _, mac := range pc.MACs {
			macs = append(macs, [2]string{id, mac})
		}
	}
	m.mu.RUnlock()
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		for _, x := range macs {
			res[x[0]] = err.Error()
		}
		return res
	}
	defer conn.Close()
	bcast := proto.BroadcastAddrs()
	for _, x := range macs {
		p, err := proto.MagicPacket(x[1])
		if err != nil {
			res[x[0]] = "invalid MAC " + x[1]
			continue
		}
		for _, ip := range bcast {
			conn.WriteTo(p, &net.UDPAddr{IP: ip, Port: 9})
			conn.WriteTo(p, &net.UDPAddr{IP: ip, Port: 7})
		}
		if _, ok := res[x[0]]; !ok {
			res[x[0]] = ""
		}
	}
	return res
}
