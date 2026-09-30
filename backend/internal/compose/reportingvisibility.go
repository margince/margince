// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type reportingVisibilityKey struct {
	table  string
	metric crmcontracts.ReportingMetricID
	money  bool
}

func factVisibilityKey(fact reporting.Fact) reportingVisibilityKey {
	return reportingVisibilityKey{table: fact.SourceType, metric: fact.Metric, money: fact.SourceType == string(recordTypeDeal) && fact.Metric != reportingClosedWinRate && fact.Metric != reportingStageAge}
}

func (a reportingAuthority) Visible(ctx context.Context, tx pgx.Tx, facts []reporting.Fact) ([]reporting.Fact, bool, error) {
	members, err := a.Members(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, false, apperrors.ErrPermissionDenied
	}
	all := auth.Unbounded(actor) || actor.Permissions.RowScope == principal.RowScopeAll
	groups := map[reportingVisibilityKey][]ids.UUID{}
	for _, fact := range facts {
		if fact.SourceType != reportingForecastProjection && (all || slices.Contains(members, fact.OwnerID)) {
			key := factVisibilityKey(fact)
			groups[key] = append(groups[key], fact.SourceID)
		}
	}
	visible := map[reportingVisibilityKey]map[ids.UUID]bool{}
	for key, sourceIDs := range groups {
		allowed, err := reportingVisibleSources(ctx, tx, key, sourceIDs)
		if err != nil {
			return nil, false, err
		}
		visible[key] = allowed
	}
	out := make([]reporting.Fact, 0, len(facts))
	projections := []reporting.Fact{}
	withheld := false
	for _, fact := range facts {
		if fact.SourceType == reportingForecastProjection {
			projections = append(projections, fact)
			continue
		}
		if (!all && !slices.Contains(members, fact.OwnerID)) || !visible[factVisibilityKey(fact)][fact.SourceID] {
			withheld = true
			continue
		}
		out = append(out, fact)
	}
	return a.visibleForecastProjections(ctx, tx, out, projections, withheld)
}

func reportingVisibleSources(ctx context.Context, tx pgx.Tx, key reportingVisibilityKey, sourceIDs []ids.UUID) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	object := key.table
	switch key.table {
	case string(recordTypeDeal), string(recordTypeActivity):
	case "sdr_handoff":
		object = reportingCreditObject
	default:
		return nil, apperrors.ErrInvalidArgument
	}
	if !auth.Allows(ctx, object, principal.ActionRead) {
		return out, nil
	}
	var b reportingBindings
	clause, err := reportingSourceScope(ctx, key, &b)
	if err != nil {
		return nil, err
	}
	// Credit grants reveal the frozen fact alone; source links and names were
	// omitted at capture, so ownership transfer cannot erase the SDR's credit.
	query := "SELECT source.id FROM " + pgx.Identifier{key.table}.Sanitize() + " source WHERE source.id=ANY(" + b.add(sourceIDs) + ") AND " + clause
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	allowed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	for _, id := range allowed {
		out[id] = true
	}
	return out, nil
}

func (a reportingAuthority) visibleForecastProjections(ctx context.Context, tx pgx.Tx, out, projections []reporting.Fact, withheld bool) ([]reporting.Fact, bool, error) {
	for _, fact := range projections {
		if withheld || fact.Scope == nil || !auth.Allows(ctx, objectForecast, principal.ActionRead) {
			withheld = true
			continue
		}
		if _, err := a.Scope(ctx, tx, *fact.Scope, false); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
				withheld = true
				continue
			}
			return nil, false, err
		}
		out = append(out, fact)
	}
	return out, withheld, nil
}

func reportingSourceScope(ctx context.Context, key reportingVisibilityKey, b *reportingBindings) (string, error) {
	clause := sqlUnnarrowed
	var err error
	switch key.table {
	case string(recordTypeActivity):
		clause, err = auth.ActivityContentClause(ctx, "source", b.arg)
	case string(recordTypeDeal):
		clause, err = auth.ScopeClauseFor(ctx, key.table, "source", b.arg)
	}
	if err != nil {
		return "", err
	}
	if clause == "" {
		clause = sqlUnnarrowed
	}
	if key.table == string(recordTypeDeal) {
		fields, _, err := reportingFieldScope(ctx, string(recordTypeDeal), "source", reportingDealFields(key.metric), b)
		if err != nil {
			return "", err
		}
		clause += " AND " + fields
	}
	if key.table == string(recordTypeActivity) {
		fields, _, err := reportingFieldScope(ctx, string(recordTypeActivity), "source", reportingMeetingFields(), b)
		if err != nil {
			return "", err
		}
		clause += " AND " + fields
	}
	if key.money {
		mask, masked, err := auth.MaskExcludedClause(ctx, string(recordTypeDeal), "amount_minor", "source", b.arg)
		if err != nil {
			return "", err
		}
		if masked && mask != "" {
			clause += " AND " + mask
		}
	}
	return clause, nil
}
