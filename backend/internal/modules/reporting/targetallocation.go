// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func (s *Service) targetAllocation(ctx context.Context, tx pgx.Tx, target *crmcontracts.ReportingTarget) error {
	if target.Definition.Scope.Kind == reportingOwner {
		return nil
	}
	members, err := s.authority.MembersFor(ctx, tx, target.Definition.Scope)
	if err != nil {
		return err
	}
	var b bindings
	where := "t.scope_kind='owner' AND t.scope_id=ANY(" + b.add(members) + ") AND t.metric=" + b.add(target.Definition.Metric) + " AND t.pipeline_id IS NOT DISTINCT FROM " + b.add(target.Definition.PipelineId) + "::uuid AND t.period_start=" + b.add(target.Interval.StartAt) + " AND t.period_end=" + b.add(target.Interval.EndAt) + " AND t.unit=" + b.add(target.Unit)
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
