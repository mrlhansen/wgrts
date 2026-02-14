package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	var wgdev string
	var proto string
	var interval uint

	flag.StringVar(&wgdev, "wgdev", "", "name of the wireguard network device")
	flag.StringVar(&proto, "proto", "", "filter routes by protocol")
	flag.UintVar(&interval, "interval", 10, "interval for scanning the routing table (in seconds)")
	flag.Parse()

	if wgdev == "" {
		fmt.Printf("usage: %s [options] -wgdev <dev>\n", os.Args[0])
		return
	}

	wg := NewInterface(wgdev)
	if wg == nil {
		os.Exit(1)
	}

	rts, ok := ScanRoutes(wgdev, proto)
	if !ok {
		os.Exit(1)
	}

	for _, k := range rts {
		wg.UpdatePeerRoute(&k)
	}

	d := time.Duration(interval) * time.Second

	for {
		time.Sleep(d)

		ok := wg.ScanInterfacePeers()
		if !ok {
			continue
		}

		rts, ok := ScanRoutes(wgdev, proto)
		if !ok {
			continue
		}

		for _, k := range rts {
			wg.UpdatePeerRoute(&k)
		}
	}
}
