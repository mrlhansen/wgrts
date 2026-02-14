package main

import (
	"bufio"
	"bytes"
	"log"
	"net/netip"
	"regexp"
	"strings"
)

type Route struct {
	Subnet  netip.Prefix
	Nexthop netip.Addr
}

func FindRoutes(device string) ([]Route, bool) {
	cmd := []string{"ip", "-o", "route", "show", "table", "all", "dev", device}
	stdout, stderr, ok := RunCommand(cmd)
	if !ok {
		log.Printf("%s: failed to query routes: %s", device, strings.ToLower(stderr))
		return nil, false
	}

	rts := []Route{}

	b := []byte(stdout)
	r := bytes.NewReader(b)
	s := bufio.NewScanner(r)
	re := regexp.MustCompile(`(\S+)\s+via\s+(\S+)`)

	for s.Scan() {
		line := s.Text()

		// if !strings.Contains(line, "proto bird") { // might not be needed
		// 	continue
		// }

		m := re.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}

		prefix, err := netip.ParsePrefix(m[1])
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
