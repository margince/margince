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
//
// Every case here drives the abort ITSELF. What is worth proving is an
// ordering — the abort reaches a phase that is still running — and a real
// clock cannot be asked for one: a nanosecond bound against a loopback server
// wins nearly always, and a test that is right nearly always is a flake
// wearing a regression's clothes.

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
)

// A pull that outlives its bound fails, rather than running on.
//
// Driven through Sync against the real in-memory server, so what is asked is
// that the bound is WIRED INTO the phase — a helper that closes a socket
// proves only that a helper closes a socket. The abort fires on the first
// response the client waits for, which is the moment the defect describes: a
// command has gone out and the server has not answered it yet.
func TestAPullThatOutlivesItsBoundFails(t *testing.T) {
	addr, _ := startMemServer(t, 3)
	held := &heldTimer{}
	c := NewStanding().
		withDialer(abortingOnFirstRead(addr, held)).
		withPhaseTimer(held.schedule)

	sink := &recordingSink{}
	_, err := c.Sync(context.Background(), standingAuth(t), nil, sink)
	if err == nil {
		t.Fatal("a pull past its bound answered normally — the phase is bounded by nothing, " +
			"and a server that dribbles inside its own per-response timeout holds the worker")
	}
	if !held.fired {
		t.Fatal("the phase never armed an abort, so this case passed on some other failure: " +
			"the bound is not wired into the pull at all")
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
	held := &heldTimer{}
	c := NewStanding().withDialer(plainDialer(addr)).withPhaseTimer(held.schedule)

	sink := &recordingSink{}
	if _, err := c.Sync(context.Background(), standingAuth(t), nil, sink); err != nil {
		t.Fatalf("a pull well inside its bound was refused: %v", err)
	}
	if len(sink.records) == 0 {
		t.Fatal("no records captured, so the case above could pass over a pull that reads nothing")
	}
	if !held.armed {
		t.Error("an ordinary pull armed no abort, so nothing bounds it and the case above " +
			"was proving something else entirely")
	}
}

// The disarm: a phase that finishes does not leave a timer that will close a
// connection the pool may have handed on.
//
// The assertion is that the disarm reaches the TIMER, which is what nothing
// else here can see. Whether a stopped time.Timer then stays quiet is the
// standard library's guarantee and not this package's to re-prove; what a
// regression would look like is abortAfter answering a func that stops
// nothing, and that is what fails below.
func TestTheBoundIsDisarmedWhenThePhaseEnds(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		//craft:ignore swallowed-errors test pipe teardown; the assertions already ran
		_ = server.Close()
	})
	held := &heldTimer{}

	disarm := abortAfter(client, time.Hour, held.schedule)
	if !held.armed {
		t.Fatal("no abort was armed, so the disarm below has nothing to prove")
	}
	disarm()

	if !held.disarmed {
		t.Fatal("the phase ended without disarming its abort: the timer outlives the pull, " +
			"and closes whatever connection the pool has handed on by the time it fires")
	}
	// And a disarmed timer that fires anyway must not be what closes it — the
	// double honours Stop, so this is the connection surviving the ordering
	// abortAfter is responsible for. net.Pipe is unbuffered, so the far end
	// has to be reading for the write to land at all.
	held.fire()
	read := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := server.Read(buf)
		read <- err
	}()
	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatalf("the connection was closed after its phase ended: %v", err)
	}
	if err := <-read; err != nil {
		t.Fatalf("the far end could not read from a connection whose phase merely ended: %v", err)
	}
}

// heldTimer is a phase timer nothing but the test advances. It models the one
// property abortAfter depends on: a disarmed timer does not fire.
type heldTimer struct {
	armed    bool
	disarmed bool
	fired    bool
	run      func()
}

func (h *heldTimer) schedule(_ time.Duration, fire func()) func() {
	h.armed = true
	h.run = fire
	return func() { h.disarmed = true }
}

// fire runs the abort, unless it has been disarmed.
func (h *heldTimer) fire() {
	if h.disarmed || h.run == nil {
		return
	}
	h.fired = true
	h.run()
}

// abortingOnFirstRead dials the memory server and trips the phase timer the
// first time the client waits for a response — a command written, nothing back
// yet, which is exactly the state the bound exists for.
func abortingOnFirstRead(addr string, held *heldTimer) func(context.Context, Credentials) (*imapclient.Client, net.Conn, error) {
	return func(_ context.Context, creds Credentials) (*imapclient.Client, net.Conn, error) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, nil, err
		}
		// The CLIENT reads through the wrapper, which is the whole point: the
		// trip has to fire on a read the pull is waiting for, and a wrapper the
		// client never touches would sit there while the pull ran to
		// completion.
		wrapped := &tripOnRead{Conn: conn}
		client := imapclient.New(wrapped, &imapclient.Options{})
		if err := client.Login(creds.Email, creds.Password).Wait(); err != nil {
			//craft:ignore swallowed-errors best-effort close of a session whose login already failed
			_ = client.Close()
			return nil, nil, ErrLoginRejected
		}
		// Armed only AFTER login, so the abort lands in the phase rather than
		// in the session setup the bound says nothing about.
		wrapped.trip = held.fire
		return client, wrapped, nil
	}
}

// tripOnRead runs trip once, before the first read the pull waits on. A nil
// trip is a connection that behaves like any other, which is what the login
// round trip needs.
type tripOnRead struct {
	net.Conn
	trip    func()
	tripped bool
}

func (c *tripOnRead) Read(b []byte) (int, error) {
	if c.trip != nil && !c.tripped {
		c.tripped = true
		c.trip()
	}
	return c.Conn.Read(b)
}
