// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

// DetachedOutcomeTimeout bounds recording a detached send's result. It is
// apart from the send's own budget, which a timed-out send has already spent.
const DetachedOutcomeTimeout = 10 * time.Second

// RecoverDetached stops a panic in a goroutine that outlives its request.
// RecoverPanics cannot see it there, and an unrecovered panic ends the process.
// It logs the stack, since no request log exists to find the site.
// Call it deferred, directly: recover only works from the deferred call itself.
func RecoverDetached(what string) {
	if panicked := recover(); panicked != nil {
		slog.Error(what+" panicked", "panic", panicked, "stack", string(debug.Stack()))
	}
}

// DetachedSend is one mail sent off the request that asked for it, so a slow
// relay does not hold the answer. The request's values travel with it; its
// cancellation does not.
type DetachedSend struct {
	// What names the send in a panic log.
	What string
	// Budget bounds Send.
	Budget time.Duration
	// Send mails, or reports attempted false when the mail must not go out.
	Send func(ctx context.Context) (attempted bool, err error)
	// Record writes the relay's answer for an attempted send, under its own
	// budget so a send that ran out of time still lands its refusal.
	Record func(ctx context.Context, sendErr error)
	// Done is a test seam, called once the send and its record are over.
	Done func()
}

// Start runs the send in its own goroutine and returns at once.
func (d DetachedSend) Start(parent context.Context) {
	// release runs in the goroutine, which outlives the caller.
	work, release := context.WithTimeout(context.WithoutCancel(parent), d.Budget)
	go func() {
		defer release()
		if d.Done != nil {
			defer d.Done()
		}
		defer RecoverDetached(d.What)
		attempted, sendErr := d.Send(work)
		if !attempted {
			return
		}
		outcome, releaseOutcome := context.WithTimeout(context.WithoutCancel(work), DetachedOutcomeTimeout)
		defer releaseOutcome()
		d.Record(outcome, sendErr)
	}()
}
