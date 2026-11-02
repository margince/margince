// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// delayproxy forwards TCP to a target and holds each chunk the client sends
// for a fixed delay, so every statement pays one simulated round trip.
package main

import (
	"flag"
	"io"
	"log"
	"net"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:15434", "address to accept on")
	target := flag.String("target", "127.0.0.1:15433", "address to forward to")
	delay := flag.Duration("delay", 2*time.Millisecond, "added to each client chunk")
	flag.Parse()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	for {
		client, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			server, err := net.Dial("tcp", *target)
			if err != nil {
				log.Print(err)
				client.Close()
				return
			}
			go func() {
				if _, err := io.Copy(client, server); err != nil {
					log.Print(err)
				}
				client.Close()
			}()
			buf := make([]byte, 64<<10)
			for {
				n, err := client.Read(buf)
				if n > 0 {
					time.Sleep(*delay)
					if _, werr := server.Write(buf[:n]); werr != nil {
						break
					}
				}
				if err != nil {
					break
				}
			}
			server.Close()
		}()
	}
}
