// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func (s *Service) targetAllocation(ctx context.Context, tx pgx.Tx, owners MeasurableOwners, target *crmcontracts.ReportingTarget) error {
	if target.Definition.Scope.Kind == reportingOwner || target.Definition.Retired != nil && *target.Definition.Retired {
		return nil
	}
	visible, err := s.allocationVisible(ctx, tx, owners, target.Definition.Scope)
	if err != nil || !visible {
		return err
	}
	members, err := s.authority.MembersFor(ctx, tx, target.Definition.Scope)
	if err != nil {
		return err
	}
	var b bindings
	where := activeTarget + " AND t.scope_kind='owner' AND t.scope_id=ANY(" + b.add(members) + ") AND t.metric=" + b.add(target.Definition.Metric) + " AND t.pipeline_id IS NOT DISTINCT FROM " + b.add(target.Definition.PipelineId) + "::uuid AND t.period_start=" + b.add(target.Interval.StartAt) + " AND t.period_end=" + b.add(target.Interval.EndAt) + " AND t.unit=" + b.add(target.Unit)
	rows, err := tx.Query(ctx, "SELECT t.scope_id,LEAST(9007199254740992,(r.definition->>'value')::numeric)::bigint"+targetFrom+" WHERE "+where, b.values...)
	if err != nil {
		return err
	}
	allocated := map[ids.UUID]int64{}
	for rows.Next() {
		var owner ids.UUID
		var value int64
		if err := rows.Scan(&owner, &value); err != nil {
			rows.Close()
			return err
		}
		allocated[owner] += value
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := s.allocateGoals(ctx, tx, *target, members, allocated); err != nil {
		return err
	}
	var total int64
	for _, value := range allocated {
		if value > values.MaxExactInteger-total {
			return invalid("allocated targets exceed the exact reporting range")
		}
		total += value
	}
	target.AllocatedValue = &total
	difference := target.Definition.Value - total
	target.AllocationDifference = &difference
	return nil
}

// allocateGoals counts a member's published sales goals over the target's
// window as their share when they hold no target of their own for it. An
// evaluation holds that member against the same rule.
func (s *Service) allocateGoals(ctx context.Context, tx pgx.Tx, target crmcontracts.ReportingTarget, members []ids.UUID, allocated map[ids.UUID]int64) error {
	if target.Definition.Metric != goalMetric || target.Definition.PipelineId != nil {
		return nil
	}
	var open []ids.UUID
	for _, member := range members {
		if _, ok := allocated[member]; !ok {
			open = append(open, member)
		}
	}
	goals, whole, err := s.goalsOver(ctx, tx, open, target.Interval)
	if err != nil || !whole || goals.currency != target.Unit {
		return err
	}
	for _, member := range open {
		if value, ok := goals.over([]ids.UUID{member}); ok {
			allocated[member] = value
		}
	}
	return nil
}

// allocationVisible says whether the reader sees a target's allocation. It is
// the sum of every member's owner target, and with one colleague in the scope
// it is that colleague's target. So only a seat that reads all of them sees it:
// row scope all, or a writer whose reach covers the scope.
func (s *Service) allocationVisible(ctx context.Context, tx pgx.Tx, owners MeasurableOwners, scope crmcontracts.ReportingScope) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false, apperrors.ErrPermissionDenied
	}
	// The allocation is the sum of owner targets, which the switch withholds.
	if owners.IndividualOff {
		return false, nil
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
