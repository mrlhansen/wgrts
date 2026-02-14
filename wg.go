package main

import (
	"bufio"
	"bytes"
	"log"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

type Peer struct {
	PublicKey           string
	PresharedKey        string
	AllowedIPs          []netip.Prefix
	Endpoint            string
	PersistentKeepalive string
}

type Device struct {
	Name  string
	IPs   []netip.Prefix
	Peers map[string]Peer
}

func (p *Peer) RemoveAllowedIPs(rt netip.Prefix) {
	p.AllowedIPs = slices.DeleteFunc(p.AllowedIPs, func(s netip.Prefix) bool {
		return s == rt
	})
}

func (p *Peer) AppendAllowedIPs(rt netip.Prefix) {
	p.AllowedIPs = append(p.AllowedIPs, rt)
}

func (p *Peer) ListAllowedIPs() string {
	s := []string{}
	for _, k := range p.AllowedIPs {
		s = append(s, k.String())
	}
	return strings.Join(s, ",")
}

func (wg *Device) FindInterfaceIPs() bool {
	cmd := []string{"ip", "-o", "addr", "list", "dev", wg.Name}
	stdout, stderr, ok := RunCommand(cmd)
	if !ok {
		log.Printf("%s: failed to query interface: %s", wg.Name, strings.ToLower(stderr))
		return false
	}

	b := []byte(stdout)
	r := bytes.NewReader(b)
	s := bufio.NewScanner(r)
	re := regexp.MustCompile(`inet6?\s+(\S+)`)
	ips := []netip.Prefix{}

	for s.Scan() {
		line := s.Text()

		m := re.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}

		prefix, err := netip.ParsePrefix(m[1])
		if err != nil {
			log.Printf("%s: failed to parse address: %v", wg.Name, err)
			continue
		}

		ips = append(ips, prefix)
	}

	for _, v := range wg.IPs {
		if !slices.Contains(ips, v) {
			log.Printf("%s: removing address: %s", wg.Name, v)
		}
	}

	for _, v := range ips {
		if !slices.Contains(wg.IPs, v) {
			log.Printf("%s: adding address: %s", wg.Name, v)
		}
	}

	wg.IPs = ips
	return true
}

func (wg *Device) FindInterfacePeers() bool {
	cmd := []string{"wg", "showconf", wg.Name}
	stdout, stderr, ok := RunCommand(cmd)
	if !ok {
		log.Printf("%s: failed to query interface: %s", wg.Name, strings.ToLower(stderr))
		return false
	}

	b := []byte(stdout)
	r := bytes.NewReader(b)
	s := bufio.NewScanner(r)
	re := regexp.MustCompile(`(\S+)\s*=\s*(.+)`)

	p := Peer{}
	peers := map[string]Peer{}

	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		if strings.HasPrefix(line, "[Peer]") {
			if len(p.PublicKey) > 0 {
				peers[p.PublicKey] = p
				p = Peer{}
			}
		}

		m := re.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}

		k := m[1]
		v := m[2]

		switch k {
		case "PublicKey":
			p.PublicKey = v
		case "PresharedKey":
			p.PresharedKey = v
		case "Endpoint":
			p.Endpoint = v
		case "AllowedIPs":
			m = strings.Split(v, ",")
			for _, v := range m {
				v = strings.TrimSpace(v)
				prefix, err := netip.ParsePrefix(v)
				if err != nil {
					log.Printf("%s: failed to parse address: %v", wg.Name, err)
					continue
				}
				p.AllowedIPs = append(p.AllowedIPs, prefix)
			}
		case "PersistentKeepalive":
			p.PersistentKeepalive = v
		}
	}

	if len(p.PublicKey) > 0 {
		peers[p.PublicKey] = p
	}

	for v := range wg.Peers {
		if _, ok := peers[v]; !ok {
			log.Printf("%s: removing peer: %s", wg.Name, v)
		}
	}

	for v := range peers {
		if _, ok := wg.Peers[v]; !ok {
			log.Printf("%s: adding peer: %s", wg.Name, v)
		}
	}

	wg.Peers = peers
	return true
}

func (wg *Device) UpdatePeerRoute(rt *Route) bool {
	var del string
	var add string

	// found := false
	// for _, v := range wg.IPs {
	// 	if v.Contains(rt.Nexthop) {
	// 		found = true
	// 		break
	// 	}
	// }

	// if !found {
	// 	return nil
	// }

	for _, p := range wg.Peers {
		found := false
		owner := false

		for _, v := range p.AllowedIPs {
			if v == rt.Subnet {
				found = true
			}
			if v.Contains(rt.Nexthop) {
				owner = true
			}
		}

		if found && !owner {
			del = p.PublicKey
		}

		if !found && owner {
			add = p.PublicKey
		}
	}

	if del != "" {
		log.Printf("%s: removing route: %s via %s", wg.Name, rt.Subnet, del)

		p := wg.Peers[del]
		p.RemoveAllowedIPs(rt.Subnet)

		cmd := []string{"wg", "set", wg.Name, "peer", p.PublicKey, "allowed-ips", p.ListAllowedIPs()}
		_, stderr, ok := RunCommand(cmd)
		if !ok {
			log.Printf("%s: failed to update peer: %s", wg.Name, strings.ToLower(stderr))
			return false
		}

		wg.Peers[del] = p
	}

	if add != "" {
		log.Printf("%s: adding route: %s via %s", wg.Name, rt.Subnet, add)

		p := wg.Peers[add]
		p.AppendAllowedIPs(rt.Subnet)

		cmd := []string{"wg", "set", wg.Name, "peer", p.PublicKey, "allowed-ips", p.ListAllowedIPs()}
		_, stderr, ok := RunCommand(cmd)
		if !ok {
			log.Printf("%s: failed to update peer: %s", wg.Name, strings.ToLower(stderr))
			return false
		}

		wg.Peers[add] = p
	}

	return true
}

func NewInterface(device string) *Device {
	wg := &Device{
		Name:  device,
		Peers: map[string]Peer{},
	}

	if !wg.FindInterfaceIPs() {
		return nil
	}

	if !wg.FindInterfacePeers() {
		return nil
	}

	return wg
}
