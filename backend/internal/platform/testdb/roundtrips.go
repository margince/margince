// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CountRoundTrips counts the pool's outbound Query and Sync messages, including
// preparation exchanges. It retains framing bytes only, never SQL or values.
func CountRoundTrips(t testing.TB, cfg *pgxpool.Config) *atomic.Int64 {
	t.Helper()
	return &MeterRoundTrips(t, cfg, 0).Trips
}

// RoundTripMeter is what a metered pool's connections saw, summed over all of them.
type RoundTripMeter struct {
	// Trips counts the Query and Sync messages: each is a wait for the server.
	Trips atomic.Int64
	// Server is the time, in nanoseconds, from sending a round trip to its
	// ReadyForQuery, counted once where two answers overlap on one connection.
	Server atomic.Int64
}

// ServerTime is Server as a duration.
func (m *RoundTripMeter) ServerTime() time.Duration {
	return time.Duration(m.Server.Load())
}

// MeterRoundTrips is CountRoundTrips that also holds every round trip for delay
// before it is sent, as a slower network would. It also times each answer.
func MeterRoundTrips(t testing.TB, cfg *pgxpool.Config, delay time.Duration) *RoundTripMeter {
	t.Helper()
	ip := net.ParseIP(cfg.ConnConfig.Host)
	if cfg.ConnConfig.Host != "localhost" && !strings.HasPrefix(cfg.ConnConfig.Host, "/") && (ip == nil || !ip.IsLoopback()) {
		t.Fatal("testdb: protocol counting requires a local test database")
	}
	if cfg.ConnConfig.TLSConfig != nil {
		t.Fatal("testdb: protocol counting requires an explicitly unencrypted test pool")
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if fallback.TLSConfig != nil {
			t.Fatal("testdb: protocol counting cannot inspect a TLS fallback")
		}
	}
	meter := &RoundTripMeter{}
	dial := cfg.ConnConfig.DialFunc
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if address, ok := conn.RemoteAddr().(*net.TCPAddr); ok && !address.IP.IsLoopback() {
			return nil, errors.Join(errors.New("testdb: protocol counter dialled a non-local address"), conn.Close())
		}
		return newRoundTripConn(conn, meter, delay), nil
	}
	return meter
}

// roundTripConn scans both directions of one connection. pgconn may read in
// the background while it writes, so the open round trips sit behind a mutex.
type roundTripConn struct {
	net.Conn
	meter    *RoundTripMeter
	delay    time.Duration
	sleep    func(time.Duration)
	now      func() time.Time
	sent     messageScanner
	answered messageScanner
	waits    int

	mu       sync.Mutex
	open     []time.Time
	lastSeen time.Time
}

func newRoundTripConn(conn net.Conn, meter *RoundTripMeter, delay time.Duration) *roundTripConn {
	c := &roundTripConn{Conn: conn, meter: meter, delay: delay, sleep: time.Sleep, now: time.Now}
	c.sent = messageScanner{startup: true, done: c.sentMessage}
	c.answered = messageScanner{done: c.answeredMessage}
	return c
}

// Write scans before it sends. A round trip is then held with every message
// sent ahead of it, as a slow network holds the whole packet.
func (c *roundTripConn) Write(p []byte) (int, error) {
	c.waits = 0
	scanErr := c.sent.scan(p)
	if c.waits > 0 && c.delay > 0 {
		c.sleep(time.Duration(c.waits) * c.delay)
	}
	n, err := c.Conn.Write(p)
	if c.waits > 0 {
		sent := c.now()
		c.mu.Lock()
		for range c.waits {
			c.open = append(c.open, sent)
		}
		c.mu.Unlock()
	}
	return n, errors.Join(err, scanErr)
}

func (c *roundTripConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if scanErr := c.answered.scan(p[:n]); scanErr != nil {
		return n, errors.Join(err, scanErr)
	}
	return n, err
}

func (c *roundTripConn) sentMessage(kind byte) error {
	switch kind {
	case 'H':
		return errors.New("testdb: Flush-and-wait is not supported by the round-trip counter")
	case 'Q', 'S':
		c.meter.Trips.Add(1)
		c.waits++
	}
	return nil
}

// answeredMessage closes the oldest open round trip at its ReadyForQuery. The
// ReadyForQuery that ends the startup has no round trip open, so it counts nothing.
func (c *roundTripConn) answeredMessage(kind byte) error {
	if kind != 'Z' {
		return nil
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.open) == 0 {
		return nil
	}
	from := c.open[0]
	c.open = c.open[1:]
	if c.lastSeen.After(from) {
		from = c.lastSeen
	}
	c.lastSeen = now
	c.meter.Server.Add(int64(now.Sub(from)))
	return nil
}

// messageScanner finds the message boundaries in one direction of the wire and
// calls done with each message's type once its last byte has passed.
type messageScanner struct {
	done      func(kind byte) error
	header    [5]byte
	headerLen int
	remaining int64
	message   byte
	startup   bool
}

func (w *messageScanner) scan(p []byte) error {
	for len(p) > 0 {
		if w.remaining > 0 {
			n := min(int64(len(p)), w.remaining)
			p = p[n:]
			w.remaining -= n
			if w.remaining == 0 {
				if err := w.done(w.message); err != nil {
					return err
				}
			}
			continue
		}
		size := len(w.header)
		if w.startup {
			size--
		}
		n := copy(w.header[w.headerLen:size], p)
		w.headerLen += n
		p = p[n:]
		if w.headerLen < size {
			continue
		}
		length := binary.BigEndian.Uint32(w.header[size-4 : size])
		if length < 4 {
			return fmt.Errorf("testdb: invalid protocol message length %d", length)
		}
		w.message = w.header[0]
		if w.startup {
			w.message = 0
			w.startup = false
		}
		w.headerLen = 0
		w.remaining = int64(length) - 4
		if w.remaining == 0 {
			if err := w.done(w.message); err != nil {
				return err
			}
		}
	}
	return nil
}
