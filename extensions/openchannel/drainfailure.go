// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// What a failed drain is CALLED, whether the request that failed can EVER land,
// and what the tick does about it. Three questions, deliberately kept together
// and deliberately kept out of drain.go, which is about reading a queue and
// handing what it holds to the core.
//
// They are three questions rather than one because the same class means
// different things at different scales. A payload one sender is posting wrong is
// recorded on that request's row and the tick carries on. The same failure across
// EVERY request is not one sender's problem at all, and what the tick answers
// then is what the operator reads.
//
// And a name is not a disposition. A capture pipeline nobody can reach and a
// composition that does not admit this unit's ingress are both failures, both
// classified, and only one of them needs a human — so only one of them becomes
// dead work.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// errPayload is a stored body this connector cannot build a record from.
//
// A CONSTANT rather than an errors.New value, and that is the tier's rule rather
// than a style: a unit's root package may hold no package-level initializer that
// CALLS anything, because an initializer runs at import — before the declaration
// has been validated and before anything has decided this unit may run at all. A
// string-kinded error type is comparable, so errors.Is answers about it exactly
// as it does about a sentinel.
const errPayload payloadError = "openchannel: the request body is not a message this connector can land"

// payloadError is this unit's own refusal class for a document it cannot read.
type payloadError string

func (e payloadError) Error() string { return string(e) }

// pollRetryDelay is how long a tick postpones itself for when the capture
// pipeline could not be reached at all. It is the DISPATCHER'S OWN CADENCE
// (api/jobs.yaml), and the match is the design rather than a coincidence.
//
// A postponed child sits in `scheduled`, which is one of the states the fan-out's
// uniqueness window covers, so while it waits the dispatcher's next insert for
// this workspace collapses into it. The postponement therefore REPLACES the tick
// it would have raced, and asking for the cadence is asking for exactly the
// rhythm the connector already has. A shorter delay would drain harder during an
// outage than in health; a longer one would let the dispatcher insert a second
// tick before the postponed row woke, which is the collapse the postponement
// depends on. The seam clamps a postponement at its own ceiling, well above this.
//
// It is deliberately NOT a backoff. No row was advanced, so what a slower drain
// buys is a later recovery in exchange for one saved pass a minute.
//
// It keeps the name the tier's parity gate reads, which reconciles this number
// against the cadence declared in api/jobs.yaml.
//
// Held by: TestEveryPostponingConnectorPostponesByItsOwnDeclaredCadence (backend/gates/pollcadenceparity_test.go)
const pollRetryDelay = 60 * time.Second

// drainFailure names what went wrong in this unit's own vocabulary, and says
// whether the request that failed could EVER land.
//
// Terminal is the load-bearing half. A body nothing will accept and a member
// whose role can be restored both stop a request, and spending five ticks on the
// first is five ticks of a queue head that will never move — while parking the
// second on the first attempt would strand a message that an administrator fixes
// in a minute.
//
// The core's own sentinels are what this reads, never a message: the core
// publishes them precisely so a unit does not match on its error text.
func drainFailure(cause error) (extension.FailureClass, bool) {
	switch {
	case errors.Is(cause, errPayload):
		return classPayloadUnusable, true
	case errors.Is(cause, extension.ErrInvalid):
		return classRefusedByTheCore, true
	case errors.Is(cause, extension.ErrForbidden):
		return classMemberNotPermitted, false
	case errors.Is(cause, extension.ErrIngressNotDeclared),
		errors.Is(cause, extension.ErrAttendedIngest),
		errors.Is(cause, extension.ErrNestedIngest):
		// This unit's own standing at the port, not this request's. Every other
		// request in the batch meets the same wall, which is why it is not
		// terminal for the row: the rows are fine and the installation is not.
		return classCaptureNotDeclared, false
	default:
		// Everything a unit can get WRONG is one of the sentinels above, so what
		// is left is the pipeline itself failing to answer.
		return classCaptureUnavailable, false
	}
}

// stallFailure classifies a tick in which NOTHING landed and at least one
// request stalled for a reason that is not the request's own fault.
//
// A SHARED class is reported as itself, and that is the case worth getting right:
// every request stalling because the capture pipeline is unreachable is one
// outage happening in several places, and reporting it as its own class is what
// turns a screenful of dead jobs into a sentence naming the thing to go fix.
// Answering "everything failed" there would throw away the one fact the tick
// actually established.
//
// Requests stalling for DIFFERENT reasons is the genuinely different situation,
// and the class for it says so rather than picking one request's cause to speak
// for the rest.
//
// A batch whose every failure was TERMINAL never reaches here: those requests are
// parked, visible, and decided about, which is a tick that did its work rather
// than one that failed.
func stallFailure(ctx context.Context, stalled []extension.FailureClass) error {
	cause := fmt.Errorf("openchannel: %d received request(s) could not be landed this pass", len(stalled))
	shared := stalled[0]
	for _, class := range stalled[1:] {
		if class.Class != shared.Class {
			return extension.Failure(classEveryRequestFailed, cause)
		}
	}
	return dispositionFor(ctx, shared, cause)
}

// dispositionFor decides whether a stalled tick FAILS or postpones itself, which
// is a different question from what the stall is called.
//
// A capture pipeline nobody can reach is the one class whose own remedy says that
// nothing needs doing — no row moved, so the next reachable tick drains the same
// requests and loses nothing. Failing it anyway spends the child's three attempts
// and discards the row, and at this cadence an outage of any length therefore
// manufactures dead work every minute, each piece of it raising a banner that
// says a human must intervene in work no human can help with.
//
// Every other class still fails, INCLUDING the mixed one and including a
// composition that does not admit this unit's ingress: those are problems with
// owners, and postponing one would be this unit quietly deciding not to tell
// anybody.
//
// A TICK WHOSE OWN CONTEXT IS DONE IS REFUSED, and this arm is the reason
// dispositionFor takes a context at all. The classification is still right —
// nothing landed — but the disposition is not. A tick that ran out of wall clock
// did not meet an outage: it met its own window, because there is more work here
// than the window holds, and every later tick spends the same window and expires
// in the same place. Postponing that hides a drain that can NEVER finish behind a
// row that looks like it is waiting patiently, with no dead work and no error
// column anywhere to say otherwise; it needs a wider timeout or a smaller batch,
// and a human to choose. A CANCELLED context is a role shutting down, and
// postponing that delays the next drain by a whole cadence on every restart.
func dispositionFor(ctx context.Context, class extension.FailureClass, cause error) error {
	if class.Class == classCaptureUnavailable.Class && ctx.Err() == nil {
		return extension.Reschedule(class, pollRetryDelay, cause)
	}
	return extension.Failure(class, cause)
}
