// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The drain: the only thing here that happens without a user, and the half that
// turns a queue of accepted requests into entries on the CRM timeline.
//
// THE SHAPE TO HOLD ONTO is that a tick reads its work in one transaction,
// closes it, and only then ingests. Runtime.Ingest hands a record to the core's
// capture pipeline, which opens its own transaction — so calling it inside one
// of this unit's would take a second connection while holding one, which on a
// small pool does not fail, it hangs. The core refuses that (ErrNestedIngest)
// rather than letting it happen, and this file is what obeying the rule looks
// like: read, close, ingest, then open a second transaction to move the row.
//
// THE ROW MOVES AFTER THE INGEST AND NEVER BEFORE IT, and that asymmetry is the
// whole safety argument. A request not advanced past though it landed costs one
// deduplicated retry, because the natural key makes a replay a no-op; a request
// advanced past that did not land costs the message permanently. A worker
// stopped mid-batch therefore leaves everything it had not reached still
// pending, which is the state it arrived in.
//
// EVERY Disposition IS A SUCCESS, Skipped included. The core drops a
// wholly-internal message deliberately and commits a breadcrumb saying so;
// treating that as a failure would retry a deliberate drop forever, on a cadence.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// drainBatch bounds one tick's work. It is the page the governed listing
// answers and the batch the core's own recovery sweep takes, so the three
// numbers a reader compares are comparable — and against the endpoint queue cap
// of maxPendingInbound it means a single endpoint that filled to the brim drains
// over ten ticks rather than in one, which keeps one flooded member from
// spending every other member's turn.
const drainBatch = 100

// maxDrainAttempts is how many ticks a request that keeps stalling gets before
// it PARKS. It is not a ladder — the interval is the dispatcher's cadence, the
// same on every attempt — it is a bound on how long the queue holds something
// nobody is being told about.
//
// Five, against a one-minute cadence: five minutes of a stall that nothing on a
// screen explains is long enough to ride out a restart and short enough that the
// member sees a parked request while they still remember the message that did
// not arrive. A parked request is never a silent drop — the row stays, carrying
// the class that stopped it, and the governed listing renders both.
const maxDrainAttempts = 5

// queued is one accepted request the drain is about to act on, with the address
// it arrived on: the record's natural key is namespaced by that address, so it
// is read here rather than re-derived per row.
type queued struct {
	id       string
	owner    string
	ref      string
	body     []byte
	sentAt   time.Time
	attempts int
}

// queuedColumns is the drain's projection, in one place so a column added to it
// is one edit rather than three.
const queuedColumns = `q.id::text, q.user_id::text, e.ref, q.body, q.sent_at, q.attempts`

// drain is the workspace tick: one bounded batch of accepted requests, landed.
func drain(ctx context.Context, rt extension.Runtime) error {
	pending, err := pendingRequests(ctx, rt)
	if err != nil {
		return extension.Failure(classDrainFailed, err)
	}
	if err := landAll(ctx, rt, pending); err != nil {
		return err
	}
	return sweepDecided(ctx, rt)
}

// pendingRequests reads the batch and CLOSES the transaction before anything is
// ingested.
//
// Oldest first, across every endpoint: a request waits behind requests that
// arrived before it and behind nothing else, so a member whose sender is quiet
// is not starved by one whose sender is busy — the batch bound is what keeps a
// busy endpoint from taking the whole tick anyway.
//
// A PAUSED ENDPOINT'S QUEUE STILL DRAINS. Pausing refuses what arrives next; it
// is not a claim about requests this installation already accepted and told a
// sender it had taken.
func pendingRequests(ctx context.Context, rt extension.Runtime) ([]queued, error) {
	var found []queued
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+queuedColumns+` FROM `+inboundTable+` q
			   JOIN `+endpointTable+` e ON e.id = q.endpoint_id
			  WHERE q.state = $1
			  ORDER BY q.received_at, q.id
			  LIMIT $2`, stateWaiting, drainBatch)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var req queued
			if err := rows.Scan(&req.id, &req.owner, &req.ref, &req.body, &req.sentAt, &req.attempts); err != nil {
				return err
			}
			found = append(found, req)
		}
		return rows.Err()
	})
	return found, err
}

// landAll ingests one batch, one request at a time, and decides what the TICK
// then reports.
//
// One request's failure does not stop the others: a payload one sender is
// posting wrong must not be the reason nobody else's messages arrive. What each
// failure costs is recorded on its own row, where the member whose endpoint it
// is can read it.
func landAll(ctx context.Context, rt extension.Runtime, pending []queued) error {
	var stalled []extension.FailureClass
	landed := 0
	for _, req := range pending {
		result, err := ingestOne(ctx, rt, req)
		if err == nil {
			landed++
			if noted := markLanded(ctx, rt, req, result); noted != nil {
				return extension.Failure(classDrainFailed, noted)
			}
			continue
		}
		class, terminal := drainFailure(err)
		// On the tick's own context, and it is the one write that must not be
		// lost: it is what stops the next tick starting at the same request with
		// no record of why the last one stopped.
		if noted := markStalled(ctx, rt, req, class, terminal); noted != nil {
			return extension.Failure(classDrainFailed, noted)
		}
		if !terminal {
			stalled = append(stalled, class)
		}
	}
	if landed == 0 && len(stalled) > 0 {
		return stallFailure(ctx, stalled)
	}
	return nil
}

// ingestOne builds one record and hands it to the core, with none of this
// unit's transactions open.
//
// The core answers a DISPOSITION rather than a row, and both of them are
// successes: an accepted record names the timeline entry it became, and a
// skipped one is the core having decided deliberately not to keep it. This
// function distinguishes neither — see markLanded, which is where the difference
// between them is a column and not an outcome.
func ingestOne(ctx context.Context, rt extension.Runtime, req queued) (extension.Result, error) {
	rec, err := recordFor(req.ref, req.body, req.sentAt)
	if err != nil {
		return extension.Result{}, err
	}
	// The OWNER, taken from the row and frozen there at arrival. The record lands
	// on that member's LIVE authority — the core resolves what they may do right
	// now — so a member demoted since the request arrived lands nothing, which is
	// the point of naming a person rather than the installation.
	return rt.Ingest(ctx, extension.UserID(req.owner), rec)
}

// markLanded advances one request past the core's answer, in a transaction of
// its own — opened after the ingest returned, which is the rule this file exists
// to keep.
//
// The activity id is stored for an ACCEPTED record and left null for a skipped
// one, because a skip creates no entry to name. That null is not an absence of
// information: the state already says the request was decided about, and what
// the column is for is the archive subscription's join back from an activity id.
func markLanded(ctx context.Context, rt extension.Runtime, req queued, result extension.Result) error {
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		// Still pending, so a request the archive subscription withdrew while
		// this tick was ingesting is not dragged back to `ingested` by a write
		// that started before it.
		_, err := tx.Exec(ctx,
			`UPDATE `+inboundTable+`
			    SET state = $2, activity_id = nullif($3, '')::uuid,
			        attempts = attempts + 1, last_error_class = NULL, updated_at = now()
			  WHERE id = $1::uuid AND state = $4`,
			req.id, stateLanded, result.Ref.ID, stateWaiting)
		return err
	})
}

// markStalled records on the row what stopped it, and parks the row when nothing
// further will change the answer.
//
// PARKING IS NEVER A SILENT DROP. The row stays, in a state the governed listing
// renders, carrying the class that stopped it — and it writes a ledger row,
// because a message this installation accepted and will now never act on is
// exactly the kind of fact somebody asks about afterwards.
func markStalled(ctx context.Context, rt extension.Runtime, req queued, class extension.FailureClass, terminal bool) error {
	// AN OUTAGE IS NOT THE REQUEST'S FAULT, so it does not spend the request's
	// budget. The attempt cap exists to stop ONE request being retried forever
	// on a fault of its own — a body the core keeps refusing, an owner whose
	// authority is gone. classCaptureUnavailable is the opposite: nothing about
	// this row was tried, the whole pipeline was unreachable, and every waiting
	// row got the same answer. Counting it would park every request an
	// installation held the moment an outage outlived five cadences, which is
	// exactly what classCaptureUnavailable's remedy promises does not happen —
	// "no received request is lost" has to be true of the row, not only of the
	// tick that reported it.
	shared := class.Class == classCaptureUnavailable.Class && !terminal
	state := stateWaiting
	if terminal || (!shared && req.attempts+1 >= maxDrainAttempts) {
		state = stateParked
	}
	spend := 1
	if shared {
		spend = 0
	}
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE `+inboundTable+`
			    SET state = $2, attempts = attempts + $5, last_error_class = $3, updated_at = now()
			  WHERE id = $1::uuid AND state = $4`,
			req.id, state, class.Class, stateWaiting, spend); err != nil {
			return err
		}
		if state != stateParked {
			return nil
		}
		return recordParked(ctx, tx, req, class)
	})
}
