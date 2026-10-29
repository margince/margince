// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
// Decidability is asked of each batch (see scanInbox), so no SQL COUNT can
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
		out, err = countPendingIn(ctx, tx, p, in, s.now())
		return err
	})
	if err != nil {
		return PendingCount{}, err
	}
	return out, nil
}

// CountPendingForTarget counts every row PendingForTarget would list for one
// record, past its scan cap. It applies the same stored-pending rows, lazy
// expiry and decidability, and includes personal mutes as the record page does.
// It joins the caller's transaction. The account page reads at repeatable read,
// so there the count and the capped list see one snapshot.
func (s *Service) CountPendingForTarget(ctx context.Context, tx pgx.Tx, targetType string, targetID ids.UUID) (int, error) {
	if err := actingForAHuman(ctx); err != nil {
		return 0, err
	}
	p, _ := principal.Actor(ctx)
	out, err := countPendingIn(ctx, tx, p, targetPending(targetType, targetID), s.now())
	return out.Count, err
}

// countPendingIn walks every stored-pending row `in` selects and counts the
// ones p has to decide at now.
func countPendingIn(ctx context.Context, tx pgx.Tx, p principal.Principal, in ListInput, now time.Time) (PendingCount, error) {
	out := PendingCount{AsOf: now}
	waiting := pendingEligible(ctx, tx, p, in, now)
	err := walkInbox(ctx, tx, in, nil, func(batch []row) (bool, error) {
		counted, err := waiting(batch)
		if err != nil {
			return false, err
		}
		for i, a := range batch {
			if counted[i] {
				out.add(a)
			}
		}
		return false, nil
	})
	return out, err
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
func pendingEligible(ctx context.Context, tx pgx.Tx, p principal.Principal, in ListInput, now time.Time) batchProbe {
	probe := inboxProbe(ctx, tx, p, in)
	return func(batch []row) ([]bool, error) {
		return askWhere(batch, func(a row) bool { return a.PendingAt(now) }, probe)
	}
}
