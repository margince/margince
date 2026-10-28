// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	targetRetiredSQL = "COALESCE((r.definition->>'retired')::boolean,false)"
	activeTarget     = targetRetiredSQL + "=false"
	targetColumns    = "t.id,r.definition,t.period_start,t.period_end,t.unit,t.revision,t.version,t.created_at"
	targetFrom       = " FROM sales_target t JOIN sales_target_revision r ON r.target_id=t.id AND r.revision=t.revision"
)

func scanTarget(row pgx.Row) (crmcontracts.ReportingTarget, error) {
	var out crmcontracts.ReportingTarget
	err := row.Scan(&out.Id, &out.Definition, &out.Interval.StartAt, &out.Interval.EndAt, &out.Unit, &out.Revision, &out.Version, &out.CreatedAt)
	return out, storedError(err)
}

func (s *Service) target(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (crmcontracts.ReportingTarget, error) {
	var b bindings
	// One target by id answers to the same visibility as the list.
	visible, err := s.targetClause(ctx, tx, &b)
	if err != nil {
		return crmcontracts.ReportingTarget{}, err
	}
	query := "SELECT " + targetColumns + targetFrom + " WHERE " + visible + " AND t.id=" + b.add(id)
	if lock {
		query += " FOR UPDATE OF t"
	}
	out, err := scanTarget(tx.QueryRow(ctx, query, b.values...))
	if err != nil {
		return out, err
	}
	_, err = s.authority.Scope(ctx, tx, out.Definition.Scope, false)
	return out, err
}

// GetTarget checks allocation scope before returning target history.
func (s *Service) GetTarget(ctx context.Context, id ids.UUID) (crmcontracts.ReportingTarget, error) {
	if err := auth.Require(ctx, "sales_target", principal.ActionRead); err != nil {
		return crmcontracts.ReportingTarget{}, err
	}
	var out crmcontracts.ReportingTarget
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.target(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if err := s.targetHistory(ctx, tx, &out); err != nil {
			return err
		}
		return s.targetAllocation(ctx, tx, &out)
	})
	return out, err
}

func (s *Service) targetClause(ctx context.Context, tx pgx.Tx, b *bindings) (string, error) {
	reach, err := s.targetReach(ctx, tx)
	return reach.clause(b), err
}

// targetReach is which targets a seat reads, resolved once so a read that
// matches many targets asks for the seat's members once.
type targetReach struct {
	every   bool
	members []ids.UUID
	teams   []ids.UUID
}

func (s *Service) targetReach(ctx context.Context, tx pgx.Tx) (targetReach, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return targetReach{}, apperrors.ErrPermissionDenied
	}
	if auth.Unbounded(actor) || actor.Permissions.RowScope == principal.RowScopeAll {
		return targetReach{every: true}, nil
	}
	// Only a seat that reaches every team and owner may set the company's
	// target, so it reads every target it may set.
	every, err := s.writeReaches(ctx, tx, crmcontracts.ReportingScope{Kind: reportingWorkspace})
	if err != nil || every {
		return targetReach{every: every}, err
	}
	members, err := s.authority.Members(ctx, tx)
	return targetReach{members: members, teams: actor.TeamIDs}, err
}

func (r targetReach) clause(b *bindings) string {
	if r.every {
		return "TRUE"
	}
	return "(t.scope_kind='workspace' OR (t.scope_kind='owner' AND t.scope_id=ANY(" + b.add(r.members) + ")) OR (t.scope_kind='team' AND t.scope_id=ANY(" + b.add(r.teams) + ")))"
}

// writeReaches reports whether the seat may set a target on scope, under the
// reach CreateTarget checks.
func (s *Service) writeReaches(ctx context.Context, tx pgx.Tx, scope crmcontracts.ReportingScope) (bool, error) {
	if !auth.Allows(ctx, "sales_target", principal.ActionCreate) && !auth.Allows(ctx, "sales_target", principal.ActionUpdate) {
		return false, nil
	}
	_, err := s.authority.Scope(ctx, tx, scope, true)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// TargetFilter selects current target definitions before pagination.
type TargetFilter struct {
	Retired     *bool
	PeriodStart *openapi_types.Date
}

// ListTargets exposes only targets within the reader’s live scope.
func (s *Service) ListTargets(ctx context.Context, after *ids.UUID, limit int, filter TargetFilter) (crmcontracts.ReportingTargetList, error) {
	if err := auth.Require(ctx, "sales_target", principal.ActionRead); err != nil {
		return crmcontracts.ReportingTargetList{}, err
	}
	limit = max(1, min(limit, 100))
	out := crmcontracts.ReportingTargetList{Data: []crmcontracts.ReportingTarget{}}
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var b bindings
		where, err := s.targetClause(ctx, tx, &b)
		if err != nil {
			return err
		}
		if after != nil {
			where += " AND t.id>" + b.add(*after)
		}
		if filter.Retired != nil {
			where += " AND " + targetRetiredSQL + "=" + b.add(*filter.Retired)
		}
		if filter.PeriodStart != nil {
			// The filter is a local calendar date; t.period_start is its zoned instant.
			where += " AND r.definition->>'period_start'=" + b.add(filter.PeriodStart.Format("2006-01-02"))
		}
		query := "SELECT " + targetColumns + targetFrom + " WHERE " + where + " ORDER BY t.id LIMIT " + b.add(limit+1)
		rows, err := tx.Query(ctx, query, b.values...)
		if err != nil {
			return err
		}
		out.Data, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingTarget, error) { return scanTarget(row) })
		if err != nil {
			return err
		}
		for i := range out.Data {
			if err := s.targetAllocation(ctx, tx, &out.Data[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if len(out.Data) > limit {
		cursor := out.Data[limit-1].Id.String()
		out.NextCursor = &cursor
		out.Data = out.Data[:limit]
	}
	return out, err
}

// checkedTarget is what validateTarget derived from a valid target input.
type checkedTarget struct {
	interval crmcontracts.ReportingWindow
	unit     string
	metric   crmcontracts.ReportingMetricDefinition
}

func (s *Service) validateTarget(ctx context.Context, tx pgx.Tx, in *crmcontracts.ReportingTargetInput) (checkedTarget, error) {
	if in.Value < 0 || in.Value > values.MaxExactInteger {
		return checkedTarget{}, refuse("value", "out_of_range", "enter a non-negative target")
	}
	if !values.HasVisibleText(in.Reason) {
		return checkedTarget{}, refuse("reason", "required", "enter a revision reason")
	}
	if utf8.RuneCountInString(in.Reason) > 1000 {
		return checkedTarget{}, refuse("reason", "too_long", "a revision reason holds at most 1000 characters")
	}
	catalog, err := s.evaluator.Catalog(ctx, textlang.English)
	if err != nil {
		return checkedTarget{}, err
	}
	index := slices.IndexFunc(catalog.Metrics, func(m crmcontracts.ReportingMetricDefinition) bool { return m.Id == in.Metric && m.SupportsTarget })
	if index < 0 {
		return checkedTarget{}, refuse("metric", "unsupported", "this metric does not support targets")
	}
	scope, err := s.authority.Scope(ctx, tx, in.Scope, true)
	if err != nil {
		return checkedTarget{}, refusedField("scope", err)
	}
	in.Scope = scope
	if err := s.authority.Pipeline(ctx, tx, (*ids.UUID)(in.PipelineId)); err != nil {
		return checkedTarget{}, err
	}
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return checkedTarget{}, err
	}
	zone, err := location(calendar)
	if err != nil {
		return checkedTarget{}, err
	}
	day := in.PeriodStart.Time
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	interval, err := TargetWindow(crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 1)}, string(in.PeriodKind), calendar)
	if err != nil {
		return checkedTarget{}, refusedField("period_kind", err)
	}
	if !interval.StartAt.Equal(start) {
		return checkedTarget{}, refuse("period_start", "not_period_start", "the target must start on the first day of its month or fiscal quarter")
	}
	unit := catalog.Metrics[index].Unit
	if unit == "money" {
		unit = calendar.Currency
	}
	return checkedTarget{interval: interval, unit: unit, metric: catalog.Metrics[index]}, nil
}

// CreateTarget records an explicit allocation without inferring child targets.
func (s *Service) CreateTarget(ctx context.Context, in crmcontracts.ReportingTargetInput) (crmcontracts.ReportingTarget, error) {
	if err := requireReadWrite(ctx, "sales_target", principal.ActionCreate); err != nil {
		return crmcontracts.ReportingTarget{}, err
	}
	out := crmcontracts.ReportingTarget{Id: openapi_types.UUID(ids.NewV7()), Definition: in, Revision: 1, Version: 1, CreatedAt: s.now()}
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		checked, err := s.validateTarget(ctx, tx, &in)
		if err != nil {
			return err
		}
		if err := refuseUnfilteredPipeline(checked.metric, in); err != nil {
			return err
		}
		interval, unit := checked.interval, checked.unit
		out.Definition = in
		out.Interval = interval
		out.Unit = unit
		var b bindings
		values := []string{b.add(out.Id), b.add(in.Metric), b.add(in.Scope.Kind), b.add(in.Scope.Id), b.add(in.PipelineId), b.add(in.PeriodKind), b.add(interval.StartAt), b.add(interval.EndAt), b.add(unit), b.add(1), b.add(1), b.add(out.CreatedAt)}
		_, err = tx.Exec(ctx, "INSERT INTO sales_target(id,metric,scope_kind,scope_id,pipeline_id,period_kind,period_start,period_end,unit,revision,version,created_at) VALUES ("+strings.Join(values, ",")+")", b.values...)
		if err != nil {
			return err
		}
		if err := s.saveTargetRevision(ctx, tx, out); err != nil {
			return err
		}
		return recordChange(ctx, tx, "sales_target", ids.UUID(out.Id), "create", nil, out)
	})
	return out, storedError(err)
}

// refuseUnfilteredPipeline refuses a new target on a pipeline its figure ignores.
// Only a create asks, since a stored target can never change its pipeline.
func refuseUnfilteredPipeline(metric crmcontracts.ReportingMetricDefinition, in crmcontracts.ReportingTargetInput) error {
	if in.PipelineId != nil && (metric.AllowedFilters == nil || !slices.Contains(*metric.AllowedFilters, "pipeline")) {
		return refuse("pipeline_id", "not_filterable", "this metric counts every pipeline; save the target without one")
	}
	return nil
}

func (s *Service) saveTargetRevision(ctx context.Context, tx pgx.Tx, target crmcontracts.ReportingTarget) error {
	author, err := actorID(ctx)
	if err != nil {
		return err
	}
	raw, err := encode(target.Definition)
	if err != nil {
		return err
	}
	var b bindings
	values := []string{b.add(target.Id), b.add(target.Revision), b.add(raw), b.add(s.now()), b.add(author)}
	_, err = tx.Exec(ctx, "INSERT INTO sales_target_revision(target_id,revision,definition,effective_at,created_by) VALUES ("+strings.Join(values, ",")+")", b.values...)
	return err
}

func sameID(left, right *openapi_types.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// refuseIdentityChange names the field that differs. A revision keeps its
// target's metric, scope, pipeline and period, and a caller fixing one field
// must not be pointed at another.
func refuseIdentityChange(before crmcontracts.ReportingTarget, in crmcontracts.ReportingTargetInput, interval crmcontracts.ReportingWindow, unit string) error {
	const message = "create a separate target for a different metric, scope, pipeline or period"
	switch {
	case before.Definition.Metric != in.Metric || before.Unit != unit:
		return refuse("metric", "immutable", message)
	case before.Definition.Scope.Kind != in.Scope.Kind || !sameID(before.Definition.Scope.Id, in.Scope.Id):
		return refuse("scope", "immutable", message)
	case !sameID(before.Definition.PipelineId, in.PipelineId):
		return refuse("pipeline_id", "immutable", message)
	case before.Definition.PeriodKind != in.PeriodKind:
		return refuse("period_kind", "immutable", message)
	case !interval.StartAt.Equal(before.Interval.StartAt) || !interval.EndAt.Equal(before.Interval.EndAt):
		return refuse("period_start", "immutable", message)
	}
	return nil
}

// UpdateTarget appends a reasoned revision while preserving prior editions.
func (s *Service) UpdateTarget(ctx context.Context, id ids.UUID, version int64, in crmcontracts.ReportingTargetInput) (crmcontracts.ReportingTarget, error) {
	if err := requireReadWrite(ctx, "sales_target", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingTarget{}, err
	}
	var out crmcontracts.ReportingTarget
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.target(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if before.Version != version {
			return apperrors.ErrVersionSkew
		}
		checked, err := s.validateTarget(ctx, tx, &in)
		if err != nil {
			return err
		}
		interval, unit := checked.interval, checked.unit
		if err := refuseIdentityChange(before, in, interval, unit); err != nil {
			return err
		}
		out = before
		out.Definition = in
		out.Revision++
		out.Version++
		var b bindings
		query := "UPDATE sales_target SET revision=" + b.add(out.Revision) + ",version=" + b.add(out.Version) + " WHERE id=" + b.add(id)
		if _, err := tx.Exec(ctx, query, b.values...); err != nil {
			return err
		}
		if err := s.saveTargetRevision(ctx, tx, out); err != nil {
			return err
		}
		return recordChange(ctx, tx, "sales_target", id, "update", &before, out)
	})
	return out, err
}
