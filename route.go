package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net/netip"
	"regexp"
	"strings"
)

type Route struct {
	Subnet  netip.Prefix
	Nexthop netip.Addr
}

func ScanRoutes(device, proto string) ([]Route, bool) {
	cmd := []string{"ip", "-o", "route", "show", "table", "all", "dev", device}
	if proto != "" {
		cmd = append(cmd, "proto", proto)
	}

	stdout, stderr, ok := RunCommand(cmd)
	if !ok {
		log.Printf("%s: failed to query routes: %s", device, strings.ToLower(stderr))
		return nil, false
	}

	b := []byte(stdout)
	r := bytes.NewReader(b)
	s := bufio.NewScanner(r)
	re := regexp.MustCompile(`(\S+)\s+via\s+(\S+)`)
	rts := []Route{}

	for s.Scan() {
		line := s.Text()

		m := re.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}

		v := m[1]
		if !strings.ContainsRune(v, '/') {
			if strings.ContainsRune(v, ':') {
				v = fmt.Sprintf("%s/128", v)
			} else {
				v = fmt.Sprintf("%s/32", v)
			}
		}

		prefix, err := netip.ParsePrefix(v)
		if err != nil {
			continue
		}

		addr, err := netip.ParseAddr(m[2])
		if err != nil {
			continue
		}

		rts = append(rts, Route{
			Subnet:  prefix,
			Nexthop: addr,
		})
	}

	return rts, true
}
