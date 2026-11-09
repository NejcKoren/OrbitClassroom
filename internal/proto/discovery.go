package proto

import (
	"encoding/binary"
	"net"
)

// DiscoverRequest is broadcast by the master on UDP DiscoveryPort.
type DiscoverRequest struct {
	Orbit string `json:"orbit"` // "discover"
	V     int    `json:"v"`
	Nonce string `json:"nonce"`
	KeyFP string `json:"keyfp"` // fingerprint of the master's public key
}

// DiscoverReply is sent back by every agent that hears the request.
type DiscoverReply struct {
	Orbit   string   `json:"orbit"` // "here"
	V       int      `json:"v"`
	Nonce   string   `json:"nonce"`
	Name    string   `json:"name"`
	User    string   `json:"user"`
	Port    int      `json:"port"`
	Version string   `json:"version"`
	OS      string   `json:"os"`
	MACs    []string `json:"macs"`
	IPs     []string `json:"ips"`
	Trusted bool     `json:"trusted"`
}

// LocalNetInfo returns IPv4 addresses and MAC addresses of all active,
// non-loopback interfaces.
func LocalNetInfo() (ips, macs []string) {
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		has := false
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				ips = append(ips, n.IP.String())
				has = true
			}
		}
		if has && len(ifc.HardwareAddr) == 6 {
			macs = append(macs, ifc.HardwareAddr.String())
		}
	}
	return
}

// BroadcastAddrs returns 255.255.255.255 plus the directed broadcast address
// of every local IPv4 network.
func BroadcastAddrs() []net.IP {
	out := []net.IP{net.IPv4bcast}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil || len(n.Mask) != 4 && len(n.Mask) != 16 {
				continue
			}
			ip := n.IP.To4()
			mask := n.Mask
			if len(mask) == 16 {
				mask = mask[12:]
			}
			v := binary.BigEndian.Uint32(ip) | ^binary.BigEndian.Uint32(mask)
			b := make(net.IP, 4)
			binary.BigEndian.PutUint32(b, v)
			out = append(out, b)
		}
	}
	return out
}

// LocalSubnets24 returns the /24 networks of all local IPv4 addresses
// (used for the TCP "deep scan" when broadcasts are blocked).
func LocalSubnets24() []net.IP {
	seen := map[string]bool{}
	var out []net.IP
	ips, _ := LocalNetInfo()
	for _, s := range ips {
		ip := net.ParseIP(s).To4()
		if ip == nil || ip[0] == 169 && ip[1] == 254 {
			continue
		}
		base := net.IPv4(ip[0], ip[1], ip[2], 0).To4()
		if !seen[base.String()] {
			seen[base.String()] = true
			out = append(out, base)
		}
	}
	return out
}

// MagicPacket builds a Wake-on-LAN packet for the given MAC address.
func MagicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return nil, err
	}
	p := make([]byte, 0, 102)
	for i := 0; i < 6; i++ {
		p = append(p, 0xFF)
	}
	for i := 0; i < 16; i++ {
		p = append(p, hw...)
	}
	return p, nil
}
