// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

//nolint:nilnil // An absent or unreadable target is a valid optional allocation.
func (s *Service) matchingTarget(ctx context.Context, tx pgx.Tx, reach targetReach, frame crmcontracts.ReportingContext, scope crmcontracts.ReportingScope, metric crmcontracts.ReportingMetricID) (*crmcontracts.ReportingTarget, error) {
	if frame.TargetInterval == nil || !auth.Allows(ctx, "sales_target", principal.ActionRead) {
		return nil, nil
	}
	if _, err := s.authority.Scope(ctx, tx, scope, false); err != nil {
		return nil, err
	}
	var b bindings
	// A figure is held against a target only when the reader may read it.
	where := reach.clause(&b) + " AND " + activeTarget + " AND t.metric=" + b.add(metric) + " AND t.scope_kind=" + b.add(scope.Kind) + " AND t.scope_id IS NOT DISTINCT FROM " + b.add(scope.Id) + "::uuid AND t.pipeline_id IS NOT DISTINCT FROM " + b.add(frame.PipelineId) + "::uuid AND t.period_start=" + b.add(frame.TargetInterval.StartAt) + " AND t.period_end=" + b.add(frame.TargetInterval.EndAt)
	out, err := scanTarget(tx.QueryRow(ctx, "SELECT "+targetColumns+targetFrom+" WHERE "+where, b.values...))
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) applyTargets(ctx context.Context, tx pgx.Tx, out *crmcontracts.ReportingEvaluation) error {
	var reach targetReach
	if out.Context.TargetInterval != nil && auth.Allows(ctx, "sales_target", principal.ActionRead) {
		var err error
		if reach, err = s.targetReach(ctx, tx); err != nil {
			return err
		}
	}
	for index := range out.Metrics {
		metric := &out.Metrics[index]
		target, err := s.matchingTarget(ctx, tx, reach, out.Context, out.Context.Scope, metric.Id)
		if err != nil {
			return err
		}
		if target == nil || metric.Coverage.Withheld {
			continue
		}
		value := float64(target.Definition.Value)
		metric.Target = &value
		metric.TargetRevision = &target.Revision
		if value > 0 && metric.TargetActual != nil {
			attainment := *metric.TargetActual / value * 100
			metric.Attainment = &attainment
		}
	}
	for index := range out.Charts {
		chart := &out.Charts[index]
		if chart.Kind == reportingOwnerAttainment {
			if err := s.ownerTargets(ctx, tx, reach, out.Context, chart); err != nil {
				return err
			}
		}
		applyChartTarget(out.Context, out.Metrics, chart)
	}
	return nil
}

func (s *Service) ownerTargets(ctx context.Context, tx pgx.Tx, reach targetReach, frame crmcontracts.ReportingContext, chart *crmcontracts.ReportingChart) error {
	// A failed calculation has no members to fill in: a zero for each would
	// read as measured sales.
	if chart.Coverage.Withheld || chart.Coverage.Status == crmcontracts.ReportingStatusFailed {
		return nil
	}
	if err := addUnmeasuredOwners(ctx, tx, frame, chart); err != nil {
		return err
	}
	if !targetAppliesToInterval(frame, chart.Interval) {
		return nil
	}

	for index := range chart.Points {
		point := &chart.Points[index]
		parsed, err := ids.Parse(point.Key)
		if err != nil {
			return invalid("an attainment reading has no owner identity")
		}
		owner := openapi_types.UUID(parsed)
		if parsed.IsZero() || !slices.Contains(frame.MemberIds, owner) {
			continue
		}
		target, err := s.matchingTarget(ctx, tx, reach, frame, crmcontracts.ReportingScope{Kind: reportingOwner, Id: &owner}, chart.Metric)
		if err != nil {
			return err
		}
		if target != nil {
			value := float64(target.Definition.Value)
			point.Target = &value
		}
	}
	return s.ownerTargetAllocation(ctx, tx, reach, frame, chart)
}

// addUnmeasuredOwners gives each member with no reading a zero point, so an
// owner who sold nothing still stands against a target.
func addUnmeasuredOwners(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, chart *crmcontracts.ReportingChart) error {
	var b bindings
	rows, err := tx.Query(ctx, "SELECT id,display_name FROM app_user WHERE id=ANY("+b.add(frame.MemberIds)+") ORDER BY display_name,id", b.values...)
	if err != nil {
		return err
	}
	type member struct {
		id    openapi_types.UUID
		label string
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (member, error) {
		var m member
		err := row.Scan(&m.id, &m.label)
		return m, err
	})
	if err != nil {
		return err
	}
	for _, m := range members {
		if slices.ContainsFunc(chart.Points, func(point crmcontracts.ReportingPoint) bool { return point.Key == m.id.String() }) {
			continue
		}
		zero := float64(0)
		group := "owner:" + m.id.String()
		chart.Points = append(chart.Points, crmcontracts.ReportingPoint{Key: m.id.String(), Label: m.label, Value: &zero, Status: "ok", Evidence: &crmcontracts.ReportingEvidenceRef{Metric: chart.Metric, ContextId: chart.ContextId, GroupKey: &group}})
	}
	return nil
}

func applyChartTarget(frame crmcontracts.ReportingContext, metrics []crmcontracts.ReportingMetric, chart *crmcontracts.ReportingChart) {
	if chart.Kind == "bookings_trend" || chart.Kind == "target_progress" {
		for _, metric := range metrics {
			if metric.Id == chart.Metric && metric.Target != nil && targetAppliesToInterval(frame, chart.Interval) {
				for point := range chart.Points {
					chart.Points[point].Target = metric.Target
				}
			}
		}
	}
}

func (s *Service) ownerTargetAllocation(ctx context.Context, tx pgx.Tx, reach targetReach, frame crmcontracts.ReportingContext, chart *crmcontracts.ReportingChart) error {
	visible, err := s.allocationVisible(ctx, tx, frame.Scope)
	if err != nil || !visible {
		return err
	}
	if frame.Scope.Kind != reportingOwner {
		total := float64(0)
		for _, point := range chart.Points {
			if point.Target != nil {
				total += *point.Target
			}
		}
		if total > 9007199254740991 {
			return invalid("allocated targets exceed the exact reporting range")
		}
		chart.AllocatedTarget = &total
		target, err := s.matchingTarget(ctx, tx, reach, frame, frame.Scope, chart.Metric)
		if err != nil {
			return err
		}
		if target != nil {
			difference := float64(target.Definition.Value) - total
			chart.AllocationDifference = &difference
		}
	}
	return nil
}

func targetAppliesToInterval(frame crmcontracts.ReportingContext, interval *crmcontracts.ReportingWindow) bool {
	target := frame.TargetInterval
	if interval == nil || target == nil || !interval.StartAt.Equal(target.StartAt) {
		return false
	}
	if interval.EndAt.Equal(target.EndAt) {
		return true
	}
	if !interval.EndAt.Equal(frame.EvaluatedAt) || !interval.EndAt.Before(target.EndAt) {
		return false
	}
	months := 0
	switch frame.PeriodKind {
	case reportingThisMonth:
		months = 1
	case "this_quarter":
		months = 3
	default:
		return false
	}
	return target.EndAt.Equal(target.StartAt.AddDate(0, months, 0))
}
