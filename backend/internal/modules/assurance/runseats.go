// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// The seats behind a run's counts, kept so the small-group floor of "Show
// individual performance" can judge them when the run is read.

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Eligible is what a pass checked: how many deals and signals, and the owner
// of each. A later read judges those counts by who made them.
type Eligible struct {
	Deals, Signals           int
	DealOwners, SignalOwners []string
}

// seatsOf lists the named seats without repeats. An unowned subject names none.
func seatsOf(owners []string) []ids.UUID {
	out := []ids.UUID{}
	for _, owner := range owners {
		if seat := nullableID(owner); seat != nil && !slices.Contains(out, *seat) {
			out = append(out, *seat)
		}
	}
	return out
}

// InSnapshot is InTx in one `REPEATABLE READ` snapshot, for a read whose
// figures and whose floor must see the same rows.
func (s *Store) InSnapshot(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return err
	}
	return s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		return fn(ctx, tx)
	})
}
