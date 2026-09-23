// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// The select+fetch phase is bounded by something the client cannot clear.
//
// The deadline armed on the connection is not that thing: go-imap sets its own
// before every response read and CLEARS it afterwards, so what survives is a
// per-response bound and nothing on the phase. A server answering every
// command inside its own thirty seconds could hold a pull — and the worker
// running it — open indefinitely, and a standing connector is one registry
// singleton serving every mailbox.

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// A pull that outlives its bound fails, rather than running on.
//
// Driven through Sync against the real in-memory server, so what is asked is
// that the bound is WIRED INTO the phase — a helper that closes a socket
// proves only that a helper closes a socket.
//
// The bound is one nanosecond, which has passed by the time the first command
// is written. No clock is waited on: the abort fires, the connection closes,
// and the command in flight fails, which is what returns.
func TestAPullThatOutlivesItsBoundFails(t *testing.T) {
	addr, _ := startMemServer(t, 3)
	c := NewStanding().withDialer(plainDialer(addr)).withPhaseBound(time.Nanosecond)

	sink := &recordingSink{}
	_, err := c.Sync(context.Background(), standingAuth(t), nil, sink)
	if err == nil {
		t.Fatal("a pull past its bound answered normally — the phase is bounded by nothing, " +
			"and a server that dribbles inside its own per-response timeout holds the worker")
	}
	// The watermark does not advance on a failure, so the next cycle retries
	// from where this one started. A partial cursor here would skip mail.
	if len(sink.records) != 0 {
		t.Errorf("%d record(s) captured by an aborted pull", len(sink.records))
	}
}

// And the ordinary pull is not aborted by its own bound. Without this the case
// above passes against a bound that fires on every sync.
func TestAPullInsideItsBoundIsNotAborted(t *testing.T) {
	addr, _ := startMemServer(t, 3)
	c := NewStanding().withDialer(plainDialer(addr)).withPhaseBound(time.Hour)

	sink := &recordingSink{}
	if _, err := c.Sync(context.Background(), standingAuth(t), nil, sink); err != nil {
		t.Fatalf("a pull well inside its bound was refused: %v", err)
	}
	if len(sink.records) == 0 {
		t.Fatal("no records captured, so the case above could pass over a pull that reads nothing")
	}
}

// The disarm: a phase that finishes does not leave a timer that will close a
// connection the pool may have handed on.
func TestTheBoundIsDisarmedWhenThePhaseEnds(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		//craft:ignore swallowed-errors test pipe teardown; the assertions already ran
		_ = server.Close()
	})

	abortAfter(client, time.Nanosecond)()

	// Closed by the abort, or open? A read answers without waiting on a clock:
	// the far end writes, and a connection the timer closed cannot take it.
	go func() {
		//craft:ignore swallowed-errors the read below is the assertion; a failed write shows up there
		_, _ = server.Write([]byte("x"))
	}()
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("the connection was closed after its phase ended: %v", err)
	}
}
