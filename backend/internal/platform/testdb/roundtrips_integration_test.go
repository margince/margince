// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

// wireConn is the server end of a metered connection: it keeps what the client
// sent, answers reads from answers, and fails every write with failure.
type wireConn struct {
	net.Conn
	sent    bytes.Buffer
	answers bytes.Buffer
	failure error
}

func (c *wireConn) Write(p []byte) (int, error) {
	c.sent.Write(p)
	return len(p), c.failure
}

func (c *wireConn) Read(p []byte) (int, error) { return c.answers.Read(p) }

// meteredWire is a metered connection over a wireConn, with a clock that
// moves only when the test moves it and a sleep that only records.
func meteredWire(delay time.Duration) (*roundTripConn, *wireConn, *time.Time, *[]time.Duration) {
	server := &wireConn{}
	conn := newRoundTripConn(server, &RoundTripMeter{}, delay)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var slept []time.Duration
	conn.now = func() time.Time { return clock }
	conn.sleep = func(d time.Duration) { slept = append(slept, d) }
	return conn, server, &clock, &slept
}

func encode(t *testing.T, messages ...pgproto3.Message) []byte {
	t.Helper()
	var wire []byte
	for _, message := range messages {
		var err error
		if wire, err = message.Encode(wire); err != nil {
			t.Fatal(err)
		}
	}
	return wire
}

func TestRoundTripFramingSurvivesEveryWriteBoundary(t *testing.T) {
	wire := encode(t,
		&pgproto3.StartupMessage{ProtocolVersion: pgproto3.ProtocolVersionNumber},
		&pgproto3.Parse{Query: "SELECT 'S Q'"},
		&pgproto3.Describe{ObjectType: 'S'},
		&pgproto3.Sync{},
		&pgproto3.Bind{},
		&pgproto3.Execute{},
		&pgproto3.Sync{},
		&pgproto3.Query{String: "SELECT 'Q S'"},
	)
	for split := range len(wire) + 1 {
		conn, _, _, _ := meteredWire(0)
		for _, part := range [][]byte{wire[:split], wire[split:]} {
			if _, err := conn.Write(part); err != nil {
				t.Fatal(err)
			}
		}
		if got := conn.meter.Trips.Load(); got != 3 {
			t.Fatalf("split %d counted %d round trips, want 3", split, got)
		}
	}
	conn, _, _, _ := meteredWire(0)
	for _, b := range wire {
		if _, err := conn.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if got := conn.meter.Trips.Load(); got != 3 {
		t.Fatalf("byte-at-a-time writes counted %d trips, want 3", got)
	}
}

func TestUnsupportedProtocolAndBadFramingCannotPassTheCounter(t *testing.T) {
	for name, wire := range map[string][]byte{
		"flush-and-wait": {'H', 0, 0, 0, 4},
		"bad length":     {'S', 0, 0, 0, 3},
	} {
		t.Run(name, func(t *testing.T) {
			conn, _, _, _ := meteredWire(0)
			conn.sent.startup = false
			if _, err := conn.Write(wire); err == nil {
				t.Fatal("the counter accepted protocol traffic it cannot count")
			}
		})
	}
}

func TestFramingFailureKeepsTheTransportError(t *testing.T) {
	failure := errors.New("connection failed")
	conn, server, _, _ := meteredWire(0)
	server.failure = failure
	conn.sent.startup = false
	if _, err := conn.Write([]byte{'S', 0, 0, 0, 3}); !errors.Is(err, failure) {
		t.Fatalf("framing failure lost the transport error: %v", err)
	}
}

func TestEachRoundTripIsHeldForTheDelayWithEverythingSentBeforeIt(t *testing.T) {
	conn, server, _, slept := meteredWire(3 * time.Millisecond)
	conn.sent.startup = false
	batch := encode(t, &pgproto3.Parse{}, &pgproto3.Bind{}, &pgproto3.Execute{}, &pgproto3.Sync{}, &pgproto3.Query{String: "SELECT 1"})
	if _, err := conn.Write(batch); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{6 * time.Millisecond}; len(*slept) != 1 || (*slept)[0] != want[0] {
		t.Fatalf("held the write for %v, want %v: one delay per round trip in it", *slept, want)
	}
	if !bytes.Equal(server.sent.Bytes(), batch) {
		t.Fatal("the held write did not reach the server whole")
	}
	if _, err := conn.Write(encode(t, &pgproto3.Parse{})); err != nil || len(*slept) != 1 {
		t.Fatalf("a write with no round trip in it was held (%v, err %v)", *slept, err)
	}
}

func TestServerTimeRunsFromSendingToReadyForQueryAndCountsAnOverlapOnce(t *testing.T) {
	conn, server, clock, _ := meteredWire(0)
	conn.sent.startup = false
	ready := encode(t, &pgproto3.ReadyForQuery{TxStatus: 'I'})
	read := func() {
		t.Helper()
		buf := make([]byte, 64)
		if _, err := conn.Read(buf); err != nil {
			t.Fatal(err)
		}
	}
	// The ReadyForQuery that ends the startup closes no round trip.
	server.answers.Write(ready)
	read()
	if got := conn.meter.ServerTime(); got != 0 {
		t.Fatalf("the startup's ReadyForQuery counted %s", got)
	}
	// Two round trips sent together, answered 10 ms and 25 ms later: the server
	// was busy for 25 ms, not 35.
	if _, err := conn.Write(encode(t, &pgproto3.Sync{}, &pgproto3.Sync{})); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(10 * time.Millisecond)
	server.answers.Write(ready)
	read()
	*clock = clock.Add(15 * time.Millisecond)
	server.answers.Write(ready)
	read()
	if got := conn.meter.ServerTime(); got != 25*time.Millisecond {
		t.Fatalf("server time %s, want 25ms", got)
	}
}
