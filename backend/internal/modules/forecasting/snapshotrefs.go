// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
// first: the latest few plus the period's first, the "since it opened" anchor a
// movement read starts from.
//
// Only snapshots of the reading's own population are comparable: another
// pipeline or a different currency would turn that difference into reported
// movement. The population fingerprint is deliberately not a filter: the
// nightly whole-pipeline capture carries one, so requiring it empty would list
// nothing. The scope is the one the reading resolved,
// so a wider one was already refused. managed_teams has nothing frozen against
// it and answers without a query.
func (s *Store) SnapshotRefsTx(
	ctx context.Context, tx pgx.Tx, period Period, scope Scope, baseCurrency string,
) ([]SnapshotRef, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return nil, err
	}
	refs := []SnapshotRef{}
	if scope.Kind == ScopeManagedTeams {
		return refs, nil
	}
	// UNION, not UNION ALL: with few snapshots the first is also among the
	// newest and is listed once.
	rows, err := tx.Query(ctx, `
		WITH comparable AS (
			SELECT id, taken_at, trigger FROM forecast_snapshot
			WHERE period_start = $1 AND period_end = $2 AND scope_kind = $3
			  AND scope_id IS NOT DISTINCT FROM $4 AND base_currency = $5
			  AND pipeline_id IS NULL
		)
		(SELECT * FROM comparable ORDER BY taken_at DESC, id DESC LIMIT $6)
		UNION
		(SELECT * FROM comparable ORDER BY taken_at ASC, id ASC LIMIT 1)
		ORDER BY taken_at DESC, id DESC`,
		period.StartDate, period.EndDate, scope.Kind, scope.ID, baseCurrency, recentSnapshotRefs)
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

// snapshotRefsToWire maps the period's snapshots onto the wire shape. Never
// nil: an empty list is the answer "nothing frozen", and an absent field would
// read as "not asked".
func snapshotRefsToWire(refs []SnapshotRef) *[]crmcontracts.ForecastSnapshotRef {
	wire := make([]crmcontracts.ForecastSnapshotRef, 0, len(refs))
	for _, ref := range refs {
		wire = append(wire, crmcontracts.ForecastSnapshotRef{
			Id:      openapi_types.UUID(ref.ID),
			TakenAt: ref.TakenAt,
			Trigger: crmcontracts.ForecastSnapshotRefTrigger(ref.Trigger),
		})
	}
	return &wire
}
