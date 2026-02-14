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

	flag.StringVar(&wgdev, "wgdev", "", "name of the wireguard network device")
	flag.StringVar(&proto, "proto", "", "filter routes by protocol")
	flag.Parse()

	if wgdev == "" {
		fmt.Printf("usage: %s [options] -wgdev <dev>\n", os.Args[0])
		return
	}

	wg := NewInterface(wgdev)
	if wg == nil {
		os.Exit(1)
	}

	rts, ok := FindRoutes(wgdev, proto)
	if !ok {
		os.Exit(1)
	}

	for _, k := range rts {
		wg.UpdatePeerRoute(&k)
	}

	for {
		time.Sleep(10 * time.Second)

		ok := wg.FindInterfacePeers()
		if !ok {
			continue
		}

		rts, ok := FindRoutes(wgdev, proto)
		if !ok {
			continue
		}

		for _, k := range rts {
			wg.UpdatePeerRoute(&k)
		}
	}
}
