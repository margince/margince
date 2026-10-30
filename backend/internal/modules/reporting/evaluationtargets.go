// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// targetKey is one figure an evaluation may hold against a target: a metric
// over a scope, inside the evaluation's pipeline and target interval.
type targetKey struct {
	metric  crmcontracts.ReportingMetricID
	kind    crmcontracts.ReportingScopeKind
	subject string
}

func keyFor(scope crmcontracts.ReportingScope, metric crmcontracts.ReportingMetricID) targetKey {
	key := targetKey{metric: metric, kind: scope.Kind}
	if scope.Id != nil {
		key.subject = scope.Id.String()
	}
	return key
}

func ownerKey(owner openapi_types.UUID, metric crmcontracts.ReportingMetricID) targetKey {
	return keyFor(crmcontracts.ReportingScope{Kind: reportingOwner, Id: &owner}, metric)
}

// heldTargets are the targets one evaluation matched, by the figure each holds.
type heldTargets map[targetKey]crmcontracts.ReportingTarget

// matchingTargets reads every target the keys name in one statement. So an
// evaluation's target cost does not grow with its metrics or owners. A figure
// is held against a target only when the reader may read the target. The
// reader must also measure the evaluation's scope and each owner in owners.
func (s *Service) matchingTargets(ctx context.Context, tx pgx.Tx, reach targetReach, frame crmcontracts.ReportingContext, keys []targetKey, owners []openapi_types.UUID) (heldTargets, error) {
	held := heldTargets{}
	if frame.TargetInterval == nil || !auth.Allows(ctx, "sales_target", principal.ActionRead) || len(keys) == 0 {
		return held, nil
	}
	if err := s.authorizeTargetScopes(ctx, tx, frame.Scope, owners); err != nil {
		return nil, err
	}
	query, values := targetsQuery(reach, frame, keys)
	rows, err := tx.Query(ctx, query, values...)
	if err != nil {
		return nil, err
	}
	targets, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingTarget, error) { return scanTarget(row) })
	if err != nil {
		return nil, err
	}
	// A target's metric and scope never change after creation, so its
	// definition names the figure it holds.
	for _, target := range targets {
		held[keyFor(target.Definition.Scope, target.Definition.Metric)] = target
	}
	return held, nil
}

// targetsQuery matches keys on the raw natural-key columns, so
// sales_target_identity narrows on metric, scope and pipeline. A key with no
// subject matches only a target with no scope id, and an owner key only its
// own owner. The two arms are a `UNION ALL` because an `OR` of them is not
// planned as an index scan.
//
//craft:ignore naked-any pgx binds heterogeneous scalar types at the SQL boundary.
func targetsQuery(reach targetReach, frame crmcontracts.ReportingContext, keys []targetKey) (string, []any) {
	var whole, subject struct{ metrics, kinds, ids []string }
	seen := map[targetKey]bool{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		arm := &whole
		if key.subject != "" {
			arm = &subject
			arm.ids = append(arm.ids, key.subject)
		}
		arm.metrics, arm.kinds = append(arm.metrics, string(key.metric)), append(arm.kinds, string(key.kind))
	}
	var b bindings
	common := reach.clause(&b) + " AND " + activeTarget + " AND t.period_start=" + b.add(frame.TargetInterval.StartAt) + " AND t.period_end=" + b.add(frame.TargetInterval.EndAt)
	if frame.PipelineId == nil {
		common += " AND t.pipeline_id IS NULL"
	} else {
		common += " AND t.pipeline_id=" + b.add(*frame.PipelineId) + "::uuid"
	}
	var arms []string
	if len(whole.metrics) > 0 {
		arms = append(arms, "SELECT "+targetColumns+targetFrom+" WHERE "+common+" AND t.scope_id IS NULL AND (t.metric,t.scope_kind) IN (SELECT * FROM unnest("+b.add(whole.metrics)+"::text[],"+b.add(whole.kinds)+"::text[]))")
	}
	if len(subject.metrics) > 0 {
		arms = append(arms, "SELECT "+targetColumns+targetFrom+" WHERE "+common+" AND (t.metric,t.scope_kind,t.scope_id) IN (SELECT * FROM unnest("+b.add(subject.metrics)+"::text[],"+b.add(subject.kinds)+"::text[],"+b.add(subject.ids)+"::uuid[]))")
	}
	return strings.Join(arms, " UNION ALL "), b.values
}

// authorizeTargetScopes refuses the evaluation when the reader may not
// measure its scope or one of the owners, as asking each scope alone would.
func (s *Service) authorizeTargetScopes(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope, owners []openapi_types.UUID) error {
	if _, err := s.authority.Scope(ctx, tx, scope, false); err != nil {
		return err
	}
	if len(owners) == 0 {
		return nil
	}
	distinct := map[ids.UUID]bool{}
	candidates := make([]ids.UUID, 0, len(owners))
	for _, owner := range owners {
		if !distinct[ids.UUID(owner)] {
			distinct[ids.UUID(owner)] = true
			candidates = append(candidates, ids.UUID(owner))
		}
	}
	measurable, err := s.authority.MeasurableOwners(ctx, tx, candidates)
	if err != nil {
		return err
	}
	if len(measurable) != len(candidates) {
		return apperrors.ErrNotFound
	}
	return nil
}

func (s *Service) applyTargets(ctx context.Context, tx pgx.Tx, out *crmcontracts.ReportingEvaluation) error {
	var reach targetReach
	reached := out.Context.TargetInterval != nil && auth.Allows(ctx, "sales_target", principal.ActionRead)
	if reached {
		var err error
		if reach, err = s.targetReach(ctx, tx); err != nil {
			return err
		}
	}
	keys := make([]targetKey, 0, len(out.Metrics))
	for _, metric := range out.Metrics {
		keys = append(keys, keyFor(out.Context.Scope, metric.Id))
	}
	pointOwners := map[int]map[int]openapi_types.UUID{}
	var owners []openapi_types.UUID
	for index := range out.Charts {
		chart := &out.Charts[index]
		if chart.Kind != reportingOwnerAttainment {
			continue
		}
		points, takesTargets, err := ownerTargetPoints(ctx, tx, out.Context, chart)
		if err != nil {
			return err
		}
		if !takesTargets {
			continue
		}
		pointOwners[index] = points
		for _, owner := range points {
			keys = append(keys, ownerKey(owner, chart.Metric))
			owners = append(owners, owner)
		}
		keys = append(keys, keyFor(out.Context.Scope, chart.Metric))
	}
	held, err := s.matchingTargets(ctx, tx, reach, out.Context, keys, owners)
	if err != nil {
		return err
	}
	for index := range out.Metrics {
		metric := &out.Metrics[index]
		target, ok := held[keyFor(out.Context.Scope, metric.Id)]
		if !ok || metric.Coverage.Withheld {
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
	if len(pointOwners) > 0 && !reached {
		if reach.owners, err = s.authority.MeasurableOwnerSet(ctx, tx); err != nil {
			return err
		}
	}
	for index := range out.Charts {
		chart := &out.Charts[index]
		if points, ok := pointOwners[index]; ok {
			applyOwnerTargets(chart, points, held)
			if err := s.ownerTargetAllocation(ctx, tx, reach.owners, out.Context, chart, held); err != nil {
				return err
			}
		}
		applyChartTarget(out.Context, out.Metrics, chart)
	}
	return nil
}

// ownerTargetPoints fills in the owners an attainment chart stands against
// and answers which point is which measurable owner. It answers false when
// the chart takes no targets.
func ownerTargetPoints(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, chart *crmcontracts.ReportingChart) (map[int]openapi_types.UUID, bool, error) {
	// A failed calculation has no members to fill in: a zero for each would
	// read as measured sales.
	if chart.Coverage.Withheld || chart.Coverage.Status == crmcontracts.ReportingStatusFailed {
		return nil, false, nil
	}
	if err := addUnmeasuredOwners(ctx, tx, frame, chart); err != nil {
		return nil, false, err
	}
	if !targetAppliesToInterval(frame, chart.Interval) {
		return nil, false, nil
	}
	points := map[int]openapi_types.UUID{}
	for index, point := range chart.Points {
		parsed, err := ids.Parse(point.Key)
		if err != nil {
			return nil, false, invalid("an attainment reading has no owner identity")
		}
		owner := openapi_types.UUID(parsed)
		if !parsed.IsZero() && slices.Contains(frame.MemberIds, owner) {
			points[index] = owner
		}
	}
	return points, true, nil
}

func applyOwnerTargets(chart *crmcontracts.ReportingChart, points map[int]openapi_types.UUID, held heldTargets) {
	for index, owner := range points {
		if target, ok := held[ownerKey(owner, chart.Metric)]; ok {
			value := float64(target.Definition.Value)
			chart.Points[index].Target = &value
		}
	}
}

// addUnmeasuredOwners gives each member with no reading a zero point, so an
// owner who sold nothing still stands against a target.
func addUnmeasuredOwners(ctx context.Context, tx pgx.Tx, frame crmcontracts.ReportingContext, chart *crmcontracts.ReportingChart) error {
	var b bindings
	query := "SELECT id,display_name FROM app_user WHERE id=ANY(" + b.add(frame.MemberIds) + ") ORDER BY display_name,id"
	rows, err := tx.Query(ctx, query, b.values...)
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

func (s *Service) ownerTargetAllocation(
	ctx context.Context, tx pgx.Tx, owners MeasurableOwners, frame crmcontracts.ReportingContext, chart *crmcontracts.ReportingChart, held heldTargets,
) error {
	visible, err := s.allocationVisible(ctx, tx, owners, frame.Scope)
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
		if target, ok := held[keyFor(frame.Scope, chart.Metric)]; ok {
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
