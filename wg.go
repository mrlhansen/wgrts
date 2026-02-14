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

func (wg *Device) AddOrUpdatePeer(p *Peer) {
	if len(p.PublicKey) > 0 {
		if _, ok := wg.Peers[p.PublicKey]; !ok {
			log.Printf("%s: adding peer: %s", wg.Name, p.PublicKey)
		}
		wg.Peers[p.PublicKey] = *p
	}
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

	for s.Scan() {
		line := s.Text()

		m := re.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		v := m[1]

		prefix, err := netip.ParsePrefix(v)
		if err != nil {
			log.Printf("%s: failed to parse address: %v", wg.Name, err)
			continue
		}

		if !slices.Contains(wg.IPs, prefix) {
			log.Printf("%s: adding address: %s", wg.Name, prefix)
			wg.IPs = append(wg.IPs, prefix)
		}
	}

	// TODO: remove stale IPs?
	// secondary list with found ips and then check if any in wg.IPs is not in that list

	return true
}

func (wg *Device) FindInterfacePeers() bool {
	cmd := []string{"wg", "showconf", wg.Name}
	stdout, stderr, ok := RunCommand(cmd)
	if !ok {
		log.Printf("%s: failed to query interface: %s", wg.Name, strings.ToLower(stderr))
		return false
	}

	p := &Peer{}
	b := []byte(stdout)
	r := bytes.NewReader(b)
	s := bufio.NewScanner(r)
	re := regexp.MustCompile(`(\S+)\s*=\s*(.+)`)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())

		if strings.HasPrefix(line, "[Peer]") {
			wg.AddOrUpdatePeer(p)
			p = &Peer{}
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

	wg.AddOrUpdatePeer(p)

	// TODO: remove stale peers?
	// secondary list with found public keys and then check if any in wg.Peers is not in that list

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
