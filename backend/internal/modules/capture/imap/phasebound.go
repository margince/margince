// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imap

// What bounds a select+fetch phase, and how the bound is armed.
//
// Its own file because it is its own concept and standing.go is a session's
// worth of other ones. It is also the file a reader lands on after finding a
// pull that ran all night: the bound, the reason it is a CLOSE rather than a
// deadline, and the seam that lets a test drive it are the three things that
// question needs, and they read as one page.

import (
	"net"
	"time"
)

// withPhaseBound overrides how long a select+fetch phase may run before the
// connection is closed under it. The seam exists so the bound can be asked
// about in a test without one waiting out the shipped ninety seconds.
func (c *Connector) withPhaseBound(d time.Duration) *Connector {
	c.phaseBound = d
	return c
}

// phaseBoundOr is the configured bound, or the shipped one. Read through a
// method so a Connector built any other way still carries a bound: a zero
// duration here would fire the abort immediately and fail every pull.
func (c *Connector) phaseBoundOr() time.Duration {
	if c.phaseBound > 0 {
		return c.phaseBound
	}
	return pullDeadline
}

// withPhaseTimer overrides how the abort is scheduled, so a test can fire it at
// a chosen moment instead of waiting on a clock.
func (c *Connector) withPhaseTimer(schedule phaseTimer) *Connector {
	c.schedulePhase = schedule
	return c
}

// phaseTimerOr is the injected scheduler, or the real clock. Read through a
// method for the same reason phaseBoundOr is: a Connector built any other way
// must still arm the abort, and a nil here would leave the phase unbounded —
// the exact defect this file exists to close.
func (c *Connector) phaseTimerOr() phaseTimer {
	if c.schedulePhase != nil {
		return c.schedulePhase
	}
	return systemPhaseTimer
}

// abortAfter closes the connection once the phase has run too long, and
// answers the func that disarms it.
//
// A CLOSE rather than a deadline, because a deadline is the client's to
// manage: go-imap sets its own before every response read and clears it
// afterwards, so anything armed on the connection is gone by the second one.
// Closing is also the honest shape — the client reads on one goroutine, so a
// read deadline firing fails every pending command anyway. This says that is
// what it meant.
//
// The commands in flight fail with a use-of-closed-connection error, which
// reaches the caller as an ordinary pull failure: the watermark does not
// advance, and the next cycle retries from where this one started.
// The timer is a SEAM rather than time.AfterFunc directly, because the thing
// worth testing is an ordering: the abort has to reach a phase that is still
// running. A test driving that with a one-nanosecond bound is asking a real
// clock to win a race against a local server, which it usually does and not
// always — and "usually" in a test is a flake that reads as a regression. With
// the seam the test fires the abort itself, at the moment it means to.
func abortAfter(conn net.Conn, after time.Duration, schedule phaseTimer) func() {
	return schedule(after, func() {
		//craft:ignore swallowed-errors the abort IS the remedy; a close that fails leaves the pull to its own error
		_ = conn.Close()
	})
}

// phaseTimer schedules the abort and answers the func that disarms it —
// time.AfterFunc's shape, narrowed to the two things abortAfter uses.
type phaseTimer func(after time.Duration, fire func()) (disarm func())

// systemPhaseTimer is the shipped one, and the only one outside a test.
func systemPhaseTimer(after time.Duration, fire func()) func() {
	timer := time.AfterFunc(after, fire)
	return func() { timer.Stop() }
}
