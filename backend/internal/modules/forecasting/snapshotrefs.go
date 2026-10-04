// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recentSnapshotRefs is how many of a period's newest snapshots a reading lists;
// the period's first is added beside them, so the list is at most one longer.
const recentSnapshotRefs = 10

// SnapshotRef is the handle a movement read takes: which frozen state, when, and
// why it was taken.
type SnapshotRef struct {
	ID      ids.UUID
	TakenAt time.Time
	Trigger string
}

// SnapshotRefsTx lists the frozen states of one period and population, newest
// first: the most recent few plus the period's first, the "since it opened"
// anchor a movement read needs.
//
// The caller passes the scope its reading RESOLVED, so a wider one was already
// refused and this adds no population rule of its own. Whole-pipeline snapshots
// only: one captured for a single pipeline, or for a fixed population (it has a
// fingerprint), covers a different population, and differencing it against a
// workspace one would report the population change as movement. managed_teams has nothing frozen against it, so it answers empty
// without a query. Empty is never nil: nothing frozen is a real answer.
func (s *Store) SnapshotRefsTx(ctx context.Context, tx pgx.Tx, period Period, scope Scope) ([]SnapshotRef, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return nil, err
	}
	refs := []SnapshotRef{}
	if scope.Kind == ScopeManagedTeams {
		return refs, nil
	}
	// UNION, not UNION ALL: the first snapshot is also among the newest ones
	// when a period holds few, and must be listed once.
	rows, err := tx.Query(ctx, `
		(SELECT id, taken_at, trigger FROM forecast_snapshot
		 WHERE period_start = $1 AND period_end = $2 AND scope_kind = $3
		   AND scope_id IS NOT DISTINCT FROM $4 AND pipeline_id IS NULL AND population_fingerprint = ''
		 ORDER BY taken_at DESC, id DESC LIMIT $5)
		UNION
		(SELECT id, taken_at, trigger FROM forecast_snapshot
		 WHERE period_start = $1 AND period_end = $2 AND scope_kind = $3
		   AND scope_id IS NOT DISTINCT FROM $4 AND pipeline_id IS NULL AND population_fingerprint = ''
		 ORDER BY taken_at ASC, id ASC LIMIT 1)
		ORDER BY taken_at DESC, id DESC`,
		period.StartDate, period.EndDate, scope.Kind, scope.ID, recentSnapshotRefs)
	if err != nil {
		return nil, fmt.Errorf("forecasting: listing the period's snapshots: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref SnapshotRef
		if err := rows.Scan(&ref.ID, &ref.TakenAt, &ref.Trigger); err != nil {
			return nil, fmt.Errorf("forecasting: reading a snapshot reference: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("forecasting: listing the period's snapshots: %w", err)
	}
	return refs, nil
}
