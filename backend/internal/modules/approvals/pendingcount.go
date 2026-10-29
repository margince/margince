// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PendingCount is how many approvals wait for the caller, over every page.
type PendingCount struct {
	Count int
	// NextExpiry is the earliest moment one of the counted rows expires, so a
	// reader knows when Count drops with no event to say so. Nil when none of
	// them can expire.
	NextExpiry *time.Time
	// AsOf is the instant Count was taken; NextExpiry is relative to it.
	AsOf time.Time
}

// CountPending counts the rows the inbox would show the caller as pending: the
// same filters, mutes, scope and decidability probe, and the same lazy expiry.
//
// Decidability is a per-row probe in Go (see scanInbox), so no SQL COUNT can
// answer this. The count walks the stored-pending rows server-side in the
// inbox's own keyset batches instead, which keeps it exact and keeps every row
// off the wire. Status, Cursor and Limit in `in` are ignored.
func (s *Service) CountPending(ctx context.Context, in ListInput) (PendingCount, error) {
	p, in, err := admitInboxRead(ctx, in)
	if err != nil {
		return PendingCount{}, err
	}
	status := statusPending
	in.Status, in.Cursor, in.FailedForDecider = &status, "", false
	var out PendingCount
	// One `RepeatableRead` snapshot, so a decision committing mid-walk cannot
	// reach later batches and miss earlier ones. A composed read's snapshot is
	// joined instead. Expiry is judged on the service clock, read once, as
	// every read in this module judges it.
	err = s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		out = PendingCount{AsOf: s.now()}
		waiting := pendingEligible(ctx, tx, p, in, out.AsOf)
		return walkInbox(ctx, tx, in, nil, func(batch []row) (bool, error) {
			for _, a := range batch {
				counted, err := waiting(a)
				if err != nil {
					return false, err
				}
				if counted {
					out.add(a)
				}
			}
			return false, nil
		})
	})
	if err != nil {
		return PendingCount{}, err
	}
	return out, nil
}

func (c *PendingCount) add(a row) {
	c.Count++
	if ExpiresNever(a.Kind) {
		return
	}
	if c.NextExpiry == nil || a.ExpiresAt.Before(*c.NextExpiry) {
		at := a.ExpiresAt
		c.NextExpiry = &at
	}
}

// pendingEligible is "the caller has this row to decide": still pending at
// now, in the requested scope, and decidable by p.
func pendingEligible(ctx context.Context, tx pgx.Tx, p principal.Principal, in ListInput, now time.Time) func(row) (bool, error) {
	probe := inboxProbe(ctx, tx, p, in)
	return func(a row) (bool, error) {
		if !a.PendingAt(now) {
			return false, nil
		}
		return probe(a)
	}
}
