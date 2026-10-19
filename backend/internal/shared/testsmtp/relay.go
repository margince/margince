// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package testsmtp supplies an in-process SMTP relay for delivery tests.
package testsmtp

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"
)

// Relay records the accepted DATA body on a loopback SMTP connection.
type Relay struct {
	ln         net.Listener
	rejectRcpt bool
	// DropOnQuit closes the connection instead of answering QUIT, after the
	// message itself was accepted.
	DropOnQuit bool

	mu   sync.Mutex
	data string
}

// Start accepts one message on a test-owned loopback listener.
func Start(t *testing.T, rejectRcpt bool) *Relay {
	t.Helper()
	var config net.ListenConfig
	ln, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &Relay{ln: ln, rejectRcpt: rejectRcpt}
	go r.serveOne()
	t.Cleanup(func() {
		//craft:ignore swallowed-errors test-fixture teardown; the assertions ran already
		_ = ln.Close()
	})
	return r
}

// Address returns the loopback host and allocated port.
func (r *Relay) Address(t *testing.T) (host string, port int) {
	t.Helper()
	tcp, ok := r.ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr %T is not TCP", r.ln.Addr())
	}
	return tcp.IP.String(), tcp.Port
}

func (r *Relay) serveOne() {
	conn, err := r.ln.Accept()
	if err != nil {
		return
	}
	//craft:ignore swallowed-errors test-fixture teardown; the client side asserts the outcome
	defer func() { _ = conn.Close() }()
	w := bufio.NewWriter(conn)
	sc := bufio.NewScanner(conn)
	say := func(line string) {
		//craft:ignore swallowed-errors a broken test pipe surfaces as the client's protocol error
		_, _ = w.WriteString(line + "\r\n")
		//craft:ignore swallowed-errors same pipe
		_ = w.Flush()
	}
	say("220 fake ESMTP")
	inData := false
	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if inData {
			if line == "." {
				inData = false
				r.mu.Lock()
				r.data = data.String()
				r.mu.Unlock()
				say("250 queued")
				continue
			}
			// The client dot-stuffs a line that starts with '.'; undo it as a
			// relay does.
			data.WriteString(strings.TrimPrefix(line, ".") + "\n")
			continue
		}
		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			say("250-fake")
			say("250 8BITMIME") // cleartext loopback fixture
		case strings.HasPrefix(line, "MAIL FROM"):
			say("250 ok")
		case strings.HasPrefix(line, "RCPT TO"):
			if r.rejectRcpt {
				say("550 no such user")
				continue
			}
			say("250 ok")
		case line == "DATA":
			inData = true
			say("354 go ahead")
		case line == "QUIT":
			if !r.DropOnQuit {
				say("221 bye")
			}
			return
		default:
			say("250 ok")
		}
	}
}

// Received returns the accepted DATA bytes.
func (r *Relay) Received() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data
}
