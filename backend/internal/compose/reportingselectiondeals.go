// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reportingSelectionNarrows says whether a selection names a scope or a
// dimension. One that names neither reads every deal the reader may see.
func reportingSelectionNarrows(selection crmcontracts.ReportingSelection) bool {
	return selection.Scope.Kind != "" || selection.ProductId != nil || selection.InitiativeId != nil ||
		selection.Country != nil || selection.Industry != nil
}

// reportingSelectionDeals answers the deals a review selection reads, by the
// clauses the metric readers apply. Those are the deals its scope owns that the
// reader may see, of its product, country and industry, and in its initiative.
// A period selects no deals of its own, since each metric reads its own window.
func reportingSelectionDeals(ctx context.Context, tx pgx.Tx, store *initiatives.Store, selection crmcontracts.ReportingSelection) ([]ids.UUID, error) {
	var frame crmcontracts.ReportingContext
	scope, err := (reportingAuthority{}).Scope(ctx, tx, selection.Scope, false)
	if err != nil {
		return nil, err
	}
	frame.Scope = scope
	if err := reportingDimensionContext(ctx, tx, selection, &frame); err != nil {
		return nil, err
	}
	if err := requireInitiativeRead(ctx, frame); err != nil {
		return nil, err
	}
	members, err := loadInitiativeMembership(ctx, tx, store, frame)
	if err != nil {
		return nil, err
	}
	if members != nil {
		ctx = withInitiativeMembership(ctx, members)
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), "t.owner_id", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, err
	}
	if population == "" {
		population = sqlUnnarrowed
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeDeal), "t", b.arg)
	if err != nil {
		return nil, err
	}
	if rowScope == "" {
		rowScope = sqlUnnarrowed
	}
	dimensions, err := dealDimensionSQL(ctx, frame, &b, "t")
	if err != nil {
		return nil, err
	}
	where := population + " AND " + rowScope + dimensions + initiativeMemberSQL(ctx, &b, "t", string(recordTypeDeal))
	rows, err := tx.Query(ctx, "SELECT t.id FROM deal t WHERE "+where+" LIMIT "+b.add(reportingFactLimit+1), b.values...)
	if err != nil {
		return nil, fmt.Errorf("read the deals of the review selection: %w", err)
	}
	deals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("read the deals of the review selection: %w", err)
	}
	if len(deals) > reportingFactLimit {
		return nil, fmt.Errorf("narrow the review selection before listing its decisions: %w", apperrors.ErrInvalidArgument)
	}
	return deals, nil
}
