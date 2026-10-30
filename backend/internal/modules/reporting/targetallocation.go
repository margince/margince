// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Service) targetAllocation(ctx context.Context, tx pgx.Tx, target *crmcontracts.ReportingTarget) error {
	if target.Definition.Scope.Kind == reportingOwner || target.Definition.Retired != nil && *target.Definition.Retired {
		return nil
	}
	visible, err := s.allocationVisible(ctx, tx, target.Definition.Scope)
	if err != nil || !visible {
		return err
	}
	members, err := s.authority.MembersFor(ctx, tx, target.Definition.Scope)
	if err != nil {
		return err
	}
	var b bindings
	where := activeTarget + " AND t.scope_kind='owner' AND t.scope_id=ANY(" + b.add(members) + ") AND t.metric=" + b.add(target.Definition.Metric) + " AND t.pipeline_id IS NOT DISTINCT FROM " + b.add(target.Definition.PipelineId) + "::uuid AND t.period_start=" + b.add(target.Interval.StartAt) + " AND t.period_end=" + b.add(target.Interval.EndAt) + " AND t.unit=" + b.add(target.Unit)
	var total int64
	if err := tx.QueryRow(ctx, "SELECT LEAST(9007199254740992,COALESCE(sum((r.definition->>'value')::numeric),0))::bigint"+targetFrom+" WHERE "+where, b.values...).Scan(&total); err != nil {
		return err
	}
	if total > 9007199254740991 {
		return invalid("allocated targets exceed the exact reporting range")
	}
	target.AllocatedValue = &total
	difference := target.Definition.Value - total
	target.AllocationDifference = &difference
	return nil
}

// allocationVisible says whether the reader sees a target's allocation. It is
// the sum of every member's owner target, and with one colleague in the scope
// it is that colleague's target. So only a seat that reads all of them sees it:
// row scope all, or a writer whose reach covers the scope.
func (s *Service) allocationVisible(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false, apperrors.ErrPermissionDenied
	}
	// A lead's own teams are the members the lead reads every target of.
	if auth.Unbounded(actor) || actor.Permissions.RowScope == principal.RowScopeAll || scope.Kind == reportingManagedTeams {
		return true, nil
	}
	return s.writeReaches(ctx, tx, scope)
}

func (s *Service) targetHistory(ctx context.Context, tx pgx.Tx, target *crmcontracts.ReportingTarget) error {
	var b bindings
	query := "SELECT definition FROM sales_target_revision WHERE target_id=" + b.add(target.Id) + " ORDER BY revision"
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return err
	}
	history, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingTargetInput, error) {
		var definition crmcontracts.ReportingTargetInput
		err := row.Scan(&definition)
		return definition, err
	})
	if err != nil {
		return err
	}
	target.History = &history
	return nil
}
