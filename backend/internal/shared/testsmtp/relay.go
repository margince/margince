// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package testsmtp supplies an in-process SMTP relay for delivery tests.
package testsmtp

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"net"
	"strings"
	"sync"
	"testing"
)

// Options shape a relay before it serves its first connection.
type Options struct {
	RejectRcpt bool
	// TLS, when set, secures the session: with ImplicitTLS from the first
	// byte (port 465), else by STARTTLS, which the greeting then offers.
	TLS         *tls.Config
	ImplicitTLS bool
	// Username, when set, demands a login with these credentials before the
	// sender is named.
	Username string
	Password string
}

// Relay records the accepted DATA body on a loopback SMTP connection.
type Relay struct {
	ln   net.Listener
	opts Options
	// DropOnQuit closes the connection instead of answering QUIT, after the
	// message itself was accepted.
	DropOnQuit bool
	// RejectMessage refuses the message at its final line, after the whole
	// body was sent.
	RejectMessage bool
	// HoldAnswer, when set, holds the answer to the message's final line until
	// the channel is closed: a relay that is slow to take the message.
	HoldAnswer chan struct{}

	mu         sync.Mutex
	data       string
	recipients []string
	secured    bool
}

// Start accepts messages on a test-owned loopback listener.
func Start(t *testing.T, rejectRcpt bool) *Relay {
	t.Helper()
	return StartWith(t, Options{RejectRcpt: rejectRcpt})
}

// StartWith accepts messages on a test-owned loopback listener, one
// connection at a time.
func StartWith(t *testing.T, opts Options) *Relay {
	t.Helper()
	var config net.ListenConfig
	ln, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := &Relay{ln: ln, opts: opts}
	go r.serve()
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

func (r *Relay) serve() {
	for {
		conn, err := r.ln.Accept()
		if err != nil {
			return
		}
		r.serveOne(conn)
	}
}

// session is one connection's reader and writer, rebuilt when STARTTLS
// replaces the connection under them.
type session struct {
	conn net.Conn
	w    *bufio.Writer
	sc   *bufio.Scanner
}

func newSession(conn net.Conn) *session {
	return &session{conn: conn, w: bufio.NewWriter(conn), sc: bufio.NewScanner(conn)}
}

func (s *session) say(line string) {
	//craft:ignore swallowed-errors a broken test pipe surfaces as the client's protocol error
	_, _ = s.w.WriteString(line + "\r\n")
	//craft:ignore swallowed-errors same pipe
	_ = s.w.Flush()
}

func (r *Relay) serveOne(conn net.Conn) {
	if r.opts.TLS != nil && r.opts.ImplicitTLS {
		conn = tls.Server(conn, r.opts.TLS)
		r.markSecured()
	}
	s := newSession(conn)
	//craft:ignore swallowed-errors test-fixture teardown; the client side asserts the outcome
	defer func() { _ = s.conn.Close() }()
	s.say("220 fake ESMTP")
	authed := r.opts.Username == ""
	for s.sc.Scan() {
		line := s.sc.Text()
		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			r.greet(s)
		case line == "STARTTLS" && r.opts.TLS != nil:
			s.say("220 go ahead")
			s = newSession(tls.Server(s.conn, r.opts.TLS))
			r.markSecured()
		case strings.HasPrefix(line, "AUTH PLAIN"):
			authed = r.authenticate(s, strings.TrimSpace(strings.TrimPrefix(line, "AUTH PLAIN")))
		case strings.HasPrefix(line, "MAIL FROM"):
			if !authed {
				s.say("530 authentication required")
				continue
			}
			s.say("250 ok")
		case strings.HasPrefix(line, "RCPT TO"):
			r.recipient(s, line)
		case line == "DATA":
			s.say("354 go ahead")
			r.readData(s)
		case line == "QUIT":
			if !r.DropOnQuit {
				s.say("221 bye")
			}
			return
		default:
			s.say("250 ok")
		}
	}
}

// greet lists what the relay offers: STARTTLS before the session is secure,
// AUTH when a login is demanded.
func (r *Relay) greet(s *session) {
	s.say("250-fake")
	_, onTLS := s.conn.(*tls.Conn)
	if r.opts.TLS != nil && !onTLS {
		s.say("250-STARTTLS")
	}
	if r.opts.Username != "" {
		s.say("250-AUTH PLAIN")
	}
	s.say("250 8BITMIME")
}

func (r *Relay) authenticate(s *session, encoded string) bool {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	parts := strings.Split(string(raw), "\x00")
	if err != nil || len(parts) != 3 || parts[1] != r.opts.Username || parts[2] != r.opts.Password {
		s.say("535 authentication failed")
		return false
	}
	s.say("235 accepted")
	return true
}

func (r *Relay) recipient(s *session, line string) {
	if r.opts.RejectRcpt {
		s.say("550 no such user")
		return
	}
	address := strings.Trim(strings.TrimPrefix(line, "RCPT TO:"), "<>")
	r.mu.Lock()
	r.recipients = append(r.recipients, address)
	r.mu.Unlock()
	s.say("250 ok")
}

func (r *Relay) readData(s *session) {
	var data strings.Builder
	for s.sc.Scan() {
		line := s.sc.Text()
		if line != "." {
			// The client dot-stuffs a line that starts with '.'; undo it as a
			// relay does.
			data.WriteString(strings.TrimPrefix(line, ".") + "\n")
			continue
		}
		if r.HoldAnswer != nil {
			<-r.HoldAnswer
		}
		if r.RejectMessage {
			s.say("554 message rejected")
			return
		}
		r.mu.Lock()
		r.data = data.String()
		r.mu.Unlock()
		s.say("250 queued")
		return
	}
}

func (r *Relay) markSecured() {
	r.mu.Lock()
	r.secured = true
	r.mu.Unlock()
}

// Received returns the accepted DATA bytes.
func (r *Relay) Received() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.data
}

// Recipients returns every RCPT TO address the relay accepted, in order.
func (r *Relay) Recipients() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.recipients...)
}

// Secured reports whether any session ran over TLS.
func (r *Relay) Secured() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.secured
}
