// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The one way a deal row changes.
//
// This module writes the deal row from flows that agree on nothing else: a
// client's partial update under an If-Match, a stage advance, an archive, an
// accepted offer syncing its gross, and the nightly close-date corrector
// re-dating under a held lock. They differ in what guards the write and in what
// else their transaction does.
//
// What they may not differ in is whether the forecast move is recorded, because
// deal_forecast_history's completeness is a property of its READERS: a
// reconstruction that omits one writer's moves is a partial sum wearing the
// label of a total. So recording is not a step a writer performs. It is what
// applying a deal patch means, and the two seams below are where it means it.
//
// TestEveryDealRowWriteRecordsTheForecastItMoved holds it: a patch applied to
// the deal row from a function that does not record fails that gate.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealTable names the row the seams below write, and the row the sweep and the
// offer sync take their locks on. It exists so the gate can ask this module
// which table it protects instead of carrying its own copy of the name — a gate
// that spelled the table itself would go on guarding the old one through a
// rename.
//
// The identical string appears in this package as the RBAC object and as the
// audit entity type (auth.Require, storekit.Audit): different subjects wearing
// the same word, which is why those are not spelled with this constant. So is
// basecurrencyfreeze.go's frozenRateTables, which genuinely names this table —
// it stays a literal because the gate that derives it from the migrations reads
// the list as string literals, and would stop seeing the entry.
const dealTable = "deal"

// applyDealPatchGuarded is the client-driven write: the caller's If-Match
// version when it sent one, a row lock when it did not (storekit's ApplyGuarded
// contract). Errors come back unwrapped so the caller can still map a
// constraint violation onto the refusal that names the field.
func applyDealPatchGuarded(ctx context.Context, tx pgx.Tx, id ids.DealID,
	p *storekit.Patch, ifVersion *int64,
) error {
	if err := p.ApplyGuarded(ctx, tx, dealTable, id.UUID, ifVersion); err != nil {
		return err
	}
	return recordForecastMovement(ctx, tx, id, p.After())
}

// applyDealPatchLocked is the internal-flow write: the caller took the row lock
// before the reads that decided this patch, so the decision cannot have gone
// stale under it.
//
// The row it records is the row the LOCK names, not one the caller passes
// alongside. ApplyLocked writes where the lock points, so a second id would be a
// second answer to "which deal is this" — and the two disagreeing would write
// one deal's figures into another deal's history, with both rows resolving and
// nothing to notice. A guard the caller supplies is not a guard.
func applyDealPatchLocked(ctx context.Context, tx pgx.Tx, p *storekit.Patch, lock storekit.RowLock) error {
	if err := p.ApplyLocked(ctx, tx, lock); err != nil {
		return err
	}
	return recordForecastMovement(ctx, tx, ids.From[ids.DealKind](lock.ID()), p.After())
}
