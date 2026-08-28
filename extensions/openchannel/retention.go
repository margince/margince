// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// What this unit deletes, and why nothing else ever would.
//
// THE ext SCHEMA IS OUTSIDE THE PRODUCT'S OWN DATA SWEEP. Core retention walks
// tables core knows about; a unit's are not among them, so a connector that does
// not clean up after itself accumulates rows for the life of the installation.
// Both of this unit's growing tables are fed by parties with no session — an
// arriving request, and an attempt to post to an address a member registered — so
// "forever" is a remote party choosing how much this installation stores.
//
// IT DELETES ONLY WHAT IS DECIDED ABOUT. A pending request has not been acted on
// and is not this pass's to remove at any age: it is a message somebody was told
// had been accepted, and the drain's own parking is what turns one nothing will
// ever land into a decided row. So an aged-out queue cannot silently swallow
// work — the only way out of `pending` is through a decision.

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

// retainDecidedDays is how long a decided request and a recorded send attempt
// are kept.
//
// Thirty days, and the number comes from what the rows are FOR. Both exist so
// that "a message did not arrive on the timeline" and "a message never reached
// my system" are questions somebody can answer afterwards; both are asked within
// days of the message, and a month is a comfortable margin over a member being
// on leave when it happened. Past that, what is kept is the timeline entry
// itself, which is the product's record and has the product's own retention —
// these rows are the connector's working evidence, not a second copy of the CRM.
//
// It is deliberately not longer. A queue row holds the payload an anonymous
// sender chose, verbatim, and keeping that indefinitely is keeping a stranger's
// document indefinitely.
const retainDecidedDays = 30

// sweepDecided removes what is past the retention window, in the drain's own
// tick.
//
// In the tick rather than in a job of its own: a second scheduled pair would be
// two kinds, two wall clocks and two attempt caps to hold a DELETE that matches
// nothing on almost every run. It runs after the batch, so a tick that failed to
// land anything is not also one that deleted things.
//
// IT RECORDS NO LEDGER ROW, and that is the one place in this unit where a write
// is not recorded. The rows it removes are ones whose decisions were already
// recorded when they were made — a parked request writes its ledger row at the
// moment it parks — so a second row per deletion would say only that a retention
// window elapsed, thousands of times, in the trail that exists to hold decisions
// somebody made.
func sweepDecided(ctx context.Context, rt extension.Runtime) error {
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		// The window runs from updated_at — WHEN IT WAS DECIDED — never from
		// received_at. A request can sit `pending` for as long as an endpoint
		// stays open with nobody draining it, and the whole point of parking
		// one is to preserve it as evidence once a decision is finally made.
		// Keying the window on the arrival time instead would let a request
		// that sat pending past the window get parked and be swept away on
		// the very next tick — the parking decision recorded and destroyed in
		// the same breath, which is the one outcome parking exists to avoid.
		if _, err := tx.Exec(ctx,
			`DELETE FROM `+inboundTable+`
			  WHERE state <> $1 AND updated_at < now() - make_interval(days => $2::int)`,
			stateWaiting, retainDecidedDays); err != nil {
			return err
		}
		// The same window on the outbound ledger, because it answers the mirror
		// question and a member comparing the two screens should not find one of
		// them remembering a month further back than the other.
		_, err := tx.Exec(ctx,
			`DELETE FROM `+outboundTable+`
			  WHERE created_at < now() - make_interval(days => $1::int)`,
			retainDecidedDays)
		return err
	})
	if err != nil {
		return extension.Failure(classDrainFailed, err)
	}
	return nil
}
