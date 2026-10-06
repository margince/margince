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
// snapshot read passes through forecastSnapshotLens.
func newForecastStore(db *database.DB) *forecasting.Store {
	return forecasting.NewStore(db).WithSnapshotLens(forecastSnapshotLens)
}

func newForecastStoreFor(pool *pgxpool.Pool) *forecasting.Store {
	return newForecastStore(InstallationDB(pool))
}

// forecastSnapshotLens holds a frozen snapshot to what the live forecast would
// show this caller, since its rows carry the same per-deal figures.
//
// The population must be one the caller may measure (the same resolution
// GetForecast applies), or the snapshot is not found: whether somebody else's
// forecast was frozen is itself the disclosure. Deals the caller cannot read are
// dropped, and an amount a field mask withholds is nulled exactly as the live
// reading nulls it, so a movement cannot print a figure the forecast hid.
func forecastSnapshotLens(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, pipelineID *ids.UUID, rows []forecasting.Contribution,
) ([]forecasting.Contribution, error) {
	if _, err := ResolveAnalyticsScope(ctx, tx, requestedFromForecastScope(scope)); err != nil {
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
	masked, err := readableDeals(ctx, tx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]forecasting.Contribution, 0, len(rows))
	for _, row := range rows {
		withheld, readable := masked[row.DealID]
		if !readable {
			continue
		}
		if withheld {
			row.AmountMinor, row.BaseMinor, row.WeightedMinor = nil, nil, 0
			row.Currency = ""
		}
		out = append(out, row)
	}
	return out, nil
}

// readableDeals answers, per deal the caller may read, whether its amount is
// masked from them. A deal absent from the map is not theirs to see.
func readableDeals(
	ctx context.Context, tx pgx.Tx, rows []forecasting.Contribution,
) (map[string]bool, error) {
	if !auth.ReadGranted(ctx, tableDeal) {
		return map[string]bool{}, nil
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
		SELECT d.id, d.amount_minor IS NOT NULL AND (%s) IS NULL
		FROM deal d WHERE d.id = ANY($1) AND %s`, amountSQL, scopeClause), args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading which snapshot deals the caller may see: %w", err)
	}
	defer found.Close()
	out := make(map[string]bool, len(rows))
	for found.Next() {
		var id ids.UUID
		var withheld bool
		if err := found.Scan(&id, &withheld); err != nil {
			return nil, fmt.Errorf("compose: reading a snapshot deal's visibility: %w", err)
		}
		out[id.String()] = withheld
	}
	if err := found.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading which snapshot deals the caller may see: %w", err)
	}
	return out, nil
}
