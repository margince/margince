// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// delayproxy forwards Postgres connections to a target and holds each round
// trip for a fixed delay. It reads the client's protocol messages and waits
// once per Sync or simple Query: the points where the client waits for the
// server. It counts those round trips and answers the total on -stats.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const sslRequest = 80877103

var trips atomic.Int64

func main() {
	listen := flag.String("listen", "127.0.0.1:15434", "address to accept on")
	target := flag.String("target", "127.0.0.1:15433", "address to forward to")
	delay := flag.Duration("delay", 2*time.Millisecond, "added to each round trip")
	stats := flag.String("stats", "127.0.0.1:15435", "address that answers the round-trip total")
	flag.Parse()
	go func() {
		err := http.ListenAndServe(*stats, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, err := fmt.Fprint(w, trips.Load()); err != nil {
				log.Print(err)
			}
		}))
		log.Fatal(err)
	}()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	for {
		client, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go serve(client, *target, *delay)
	}
}

func serve(client net.Conn, target string, delay time.Duration) {
	defer client.Close()
	server, err := net.Dial("tcp", target)
	if err != nil {
		log.Print(err)
		return
	}
	defer server.Close()
	go func() {
		if _, err := io.Copy(client, server); err != nil {
			log.Print(err)
		}
		client.Close()
	}()
	if err := forward(client, server, delay); err != nil && err != io.EOF {
		log.Print(err)
	}
}

// forward copies the client's messages: untyped ones until the startup
// message, then typed ones, holding each Sync and Query for the delay.
func forward(client io.Reader, server io.Writer, delay time.Duration) error {
	started := false
	head := make([]byte, 5)
	for {
		if !started {
			if _, err := io.ReadFull(client, head[:4]); err != nil {
				return err
			}
			n := int(binary.BigEndian.Uint32(head[:4]))
			body := make([]byte, n-4)
			if _, err := io.ReadFull(client, body); err != nil {
				return err
			}
			if !(n == 8 && binary.BigEndian.Uint32(body) == sslRequest) {
				started = true
			}
			if _, err := server.Write(append(head[:4:4], body...)); err != nil {
				return err
			}
			continue
		}
		if _, err := io.ReadFull(client, head); err != nil {
			return err
		}
		n := int(binary.BigEndian.Uint32(head[1:]))
		body := make([]byte, n-4)
		if _, err := io.ReadFull(client, body); err != nil {
			return err
		}
		if head[0] == 'S' || head[0] == 'Q' {
			trips.Add(1)
			time.Sleep(delay)
		}
		if _, err := server.Write(append(append([]byte{}, head...), body...)); err != nil {
			return err
		}
	}
}
