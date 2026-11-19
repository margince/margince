// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newForecastStore is the one way compose builds a forecasting store, so every
// snapshot read passes through forecastSnapshotReadLens.
func newForecastStore(db *database.DB) *forecasting.Store {
	return forecasting.NewStore(db).WithSnapshotLens(forecastSnapshotReadLens)
}

// forecastSnapshotReadLens is forecastSnapshotLens for a snapshot read whole,
// such as a movement or a call's baseline. A frozen total of a population is
// that population's figure, so it meets the small-group floor as a live one
// does. The floor counts the seats behind the frozen rows and the members.
func forecastSnapshotReadLens(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, pipelineID *ids.UUID, rows []forecasting.Contribution,
) ([]forecasting.Contribution, error) {
	readable, err := forecastSnapshotLens(ctx, tx, scope, pipelineID, rows)
	if err != nil {
		return nil, err
	}
	allowed, err := individualPerformance(ctx, tx)
	if err != nil || allowed {
		return readable, err
	}
	resolved, err := ResolveAnalyticsScope(ctx, tx, requestedFromForecastScope(scope))
	if err != nil {
		return nil, err
	}
	owners := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		if owner, err := ids.Parse(row.Owner); err == nil {
			owners = append(owners, owner)
		}
	}
	if err := requireFlooredTotals(ctx, tx, resolved, [][]ids.UUID{owners}); err != nil {
		return nil, err
	}
	return readable, nil
}

func newForecastStoreFor(pool *pgxpool.Pool) *forecasting.Store {
	return newForecastStore(InstallationDB(pool))
}

// forecastSnapshotLens holds a frozen snapshot to what the live forecast would
// show this caller. A caller who may not assert about the population finds no
// snapshot, since whether it was frozen is itself the disclosure. Deals the
// caller cannot read are dropped, and the rest carry their live name. A masked
// amount is nulled as the live reading nulls it, so a movement prints no
// figure the forecast hid.
func forecastSnapshotLens(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, pipelineID *ids.UUID, rows []forecasting.Contribution,
) ([]forecasting.Contribution, error) {
	if _, err := ResolveAssertionScope(ctx, tx, requestedFromForecastScope(scope)); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	if err := reportingPipeline(ctx, tx, (*openapi_types.UUID)(pipelineID)); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, apperrors.ErrNotFound
		}
		return nil, err
	}
	if len(rows) == 0 {
		return rows, nil
	}
	sights, err := readableDeals(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]forecasting.Contribution, 0, len(rows))
	for _, row := range rows {
		seen, readable := sights[row.DealID]
		if !readable {
			continue
		}
		row.Name = seen.name
		if seen.withheld {
			row.AmountMinor, row.BaseMinor, row.WeightedMinor = nil, nil, 0
			row.Currency = ""
		}
		out = append(out, row)
	}
	return out, nil
}

// dealSight is what the caller sees of one frozen deal: its live name, and
// whether its amount is masked from them.
type dealSight struct {
	name     string
	withheld bool
}

// readableDeals answers, per deal the caller may read, its dealSight. A deal
// absent from the map is not theirs to see.
func readableDeals(
	ctx context.Context, tx pgx.Tx, rows []forecasting.Contribution,
) (map[string]dealSight, error) {
	if !auth.ReadGranted(ctx, tableDeal) {
		return map[string]dealSight{}, nil
	}
	dealIDs := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		id, err := ids.Parse(row.DealID)
		if err != nil {
			return nil, fmt.Errorf("compose: a snapshot row names a deal id it cannot parse: %w", err)
		}
		dealIDs = append(dealIDs, id)
	}
	args := []any{dealIDs}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}
	amountSQL, err := auth.MaskedColumnSQL(ctx, tableDeal, "amount_minor", "d", "amount_minor", arg)
	if err != nil {
		return nil, err
	}
	found, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT d.id, d.name, d.amount_minor IS NOT NULL AND (%s) IS NULL
		FROM deal d WHERE d.id = ANY($1) AND %s`, amountSQL, scopeClause), args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading which snapshot deals the caller may see: %w", err)
	}
	defer found.Close()
	out := make(map[string]dealSight, len(rows))
	for found.Next() {
		var id ids.UUID
		var seen dealSight
		if err := found.Scan(&id, &seen.name, &seen.withheld); err != nil {
			return nil, fmt.Errorf("compose: reading a snapshot deal's visibility: %w", err)
		}
		out[id.String()] = seen
	}
	if err := found.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading which snapshot deals the caller may see: %w", err)
	}
	return out, nil
}
