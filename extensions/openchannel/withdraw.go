// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// What this connector does when the timeline entry one of its requests became is
// archived.
//
// WHY THIS IS AN OBLIGATION RATHER THAN A DEMONSTRATION. The drain writes two
// things: a core activity on the timeline, and the queue row's claim to have
// landed it. Archiving that activity takes the entry away — and leaves the queue
// row saying a message reached the CRM when nothing on any screen shows it did.
// Nothing in the core knows this unit's column exists, so if the unit does not
// listen, the claim stays wrong forever.
//
// NOBODY IS BEHIND THIS CALL. rt.Caller() is the zero Caller, which is exactly
// right for the work: the row this touches is the unit's own, and the archive
// that triggered it was already authorized by whoever made it.

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

// withdrawCaptured marks every request that landed as this activity as
// withdrawn.
//
// IT IS SAFE TO RUN TWICE, which the bus requires: the UPDATE matches on rows
// that are still `ingested`, so a redelivery matches nothing, writes no ledger
// row and publishes nothing. An audit trail of writes that did not happen would
// be worse than no trail at all.
//
// It does NOT return the request to the queue. The message was landed and then
// deliberately taken off the timeline by a person; draining it again would put
// it straight back, which is the one outcome archiving must not have.
func withdrawCaptured(ctx context.Context, rt extension.Runtime, d extension.Delivery) error {
	if d.Entity.Type != "activity" || !extension.IsCanonicalUUID(d.Entity.ID) {
		// A delivery this handler cannot act on is ACKED, not failed. Failing it
		// would put it back in the pending set for the reclaim pass to hand over
		// again, forever, at the cost of one wasted handler run per pass — and it
		// would say "this failed" about an event that is simply not this
		// handler's business.
		return nil
	}
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		withdrawn, err := withdrawFor(ctx, tx, d.Entity.ID)
		if err != nil {
			return err
		}
		for _, requestID := range withdrawn {
			if err := recordWithdrawn(ctx, tx, requestID, d); err != nil {
				return err
			}
		}
		return nil
	})
}

// withdrawFor takes the landed claim off every request that named this activity,
// and answers their ids.
//
// EVERY request, not the first: two arrivals whose bodies carry the same
// message_id under one endpoint are one natural key and therefore one activity,
// and nothing in the schema stops two endpoints landing onto one either. A loop
// that handled one would leave the others claiming a timeline entry that is
// gone — the exact state this handler exists to remove.
func withdrawFor(ctx context.Context, tx extension.Tx, activityID string) ([]string, error) {
	rows, err := tx.Query(ctx,
		`UPDATE `+inboundTable+` SET state = $2, updated_at = now()
		  WHERE activity_id = $1::uuid AND state = $3
		 RETURNING id::text`, activityID, stateWithdrawn, stateLanded)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var withdrawn []string
	for rows.Next() {
		var requestID string
		if err := rows.Scan(&requestID); err != nil {
			return nil, err
		}
		withdrawn = append(withdrawn, requestID)
	}
	return withdrawn, rows.Err()
}
