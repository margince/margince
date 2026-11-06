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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reportingSelectionNarrows says whether a selection names a narrower set of
// deals. The workspace scope with no dimension is the whole list, so it reads no
// deal ids.
func reportingSelectionNarrows(selection crmcontracts.ReportingSelection) bool {
	scoped := selection.Scope.Kind != "" && selection.Scope.Kind != crmcontracts.ReportingScopeKindWorkspace
	return scoped || selection.PipelineId != nil || selection.ProductId != nil || selection.InitiativeId != nil ||
		selection.Country != nil || selection.Industry != nil
}

// reportingDealPopulationSQL renders the deals a frame reads, over the deal alias
// `t`. That is its scope, the reader's row scope and field masks, its dimensions
// and its initiative. withheld reports deals a mask left out.
func reportingDealPopulationSQL(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, owner string, fields []string, b *reportingBindings) (string, bool, error) {
	_, population, err := analyticsPopulationExpression(ctx, tx, reportingRequested(frame.Scope), owner, b.arg, unownedIsExcluded)
	if err != nil {
		return "", false, err
	}
	rowScope, err := auth.ScopeClauseFor(ctx, string(recordTypeDeal), "t", b.arg)
	if err != nil {
		return "", false, err
	}
	fieldScope, withheld, err := reportingFieldScope(ctx, string(recordTypeDeal), "t", fields, b)
	if err != nil {
		return "", false, err
	}
	dimensions, err := dealDimensionSQL(ctx, frame, b, "t")
	if err != nil {
		return "", false, err
	}
	where := orUnnarrowed(population) + " AND " + orUnnarrowed(rowScope) + " AND " + fieldScope
	return where + dimensions + initiativeMemberSQL(ctx, b, "t", string(recordTypeDeal)), withheld, nil
}

// reportingSelectionDeals answers the live deals a review selection reads. A
// period selects no deals of its own, since each metric reads its own window.
func reportingSelectionDeals(ctx context.Context, tx pgx.Tx, store *initiatives.Store, selection crmcontracts.ReportingSelection) ([]ids.UUID, error) {
	if err := auth.Require(ctx, string(recordTypeDeal), principal.ActionRead); err != nil {
		return nil, err
	}
	var frame crmcontracts.ReportingContext
	scope, err := (reportingAuthority{}).Scope(ctx, tx, selection.Scope, false)
	if err != nil {
		return nil, err
	}
	frame.Scope = scope
	if err := reportingDimensionContext(ctx, tx, selection, &frame); err != nil {
		return nil, err
	}
	ctx, err = withInitiativeMembers(ctx, tx, store, frame)
	if err != nil {
		return nil, err
	}
	var b reportingBindings
	population, _, err := reportingDealPopulationSQL(ctx, tx, frame, "t.owner_id", []string{fieldOwnerID, fieldPipelineID}, &b)
	if err != nil {
		return nil, err
	}
	where := "t.archived_at IS NULL AND " + population
	if selection.PipelineId != nil {
		where += " AND t.pipeline_id=" + b.add(*selection.PipelineId)
	}
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
