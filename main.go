package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	var device string

	flag.StringVar(&device, "wgdev", "", "name of the wireguard network interface")
	flag.Parse()

	if device == "" {
		fmt.Printf("usage: %s -wgdev <dev>\n", os.Args[0])
		return
	}

	wg := NewInterface(device)
	if wg == nil {
		os.Exit(1)
	}

	rts, ok := FindRoutes(wg.Name)
	if !ok {
		os.Exit(1)
	}

	for _, k := range rts {
		wg.UpdatePeerRoute(&k)
	}

	for {
		time.Sleep(10 * time.Second)

		rts, ok := FindRoutes(wg.Name)
		if !ok {
			continue
		}

		for _, k := range rts {
			wg.UpdatePeerRoute(&k)
		}
	}
}
