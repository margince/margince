// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
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
	query := "SELECT " + targetColumns + targetFrom + " WHERE t.id=" + b.add(id)
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
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", apperrors.ErrPermissionDenied
	}
	if auth.Unbounded(actor) || actor.Permissions.RowScope == principal.RowScopeAll {
		return "TRUE", nil
	}
	members, err := s.authority.Members(ctx, tx)
	if err != nil {
		return "", err
	}
	return "((t.scope_kind='owner' AND t.scope_id=ANY(" + b.add(members) + ")) OR (t.scope_kind='team' AND t.scope_id=ANY(" + b.add(actor.TeamIDs) + ")))", nil
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
		rows, err := tx.Query(ctx, "SELECT "+targetColumns+targetFrom+" WHERE "+where+" ORDER BY t.id LIMIT "+b.add(limit+1), b.values...)
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

func (s *Service) validateTarget(ctx context.Context, tx pgx.Tx, in *crmcontracts.ReportingTargetInput) (crmcontracts.ReportingWindow, string, error) {
	empty := crmcontracts.ReportingWindow{}
	if in.Value < 0 || in.Value > values.MaxExactInteger || !values.HasVisibleText(in.Reason) || utf8.RuneCountInString(in.Reason) > 1000 {
		return empty, "", invalid("enter a non-negative target and a revision reason")
	}
	catalog, err := s.evaluator.Catalog(ctx)
	if err != nil {
		return empty, "", err
	}
	index := slices.IndexFunc(catalog.Metrics, func(m crmcontracts.ReportingMetricDefinition) bool { return m.Id == in.Metric && m.SupportsTarget })
	if index < 0 {
		return empty, "", invalid("this metric does not support targets")
	}
	scope, err := s.authority.Scope(ctx, tx, in.Scope, true)
	if err != nil {
		return empty, "", err
	}
	in.Scope = scope
	if err := s.authority.Pipeline(ctx, tx, (*ids.UUID)(in.PipelineId)); err != nil {
		return empty, "", err
	}
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return empty, "", err
	}
	zone, err := location(calendar)
	if err != nil {
		return empty, "", err
	}
	day := in.PeriodStart.Time
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	interval, err := TargetWindow(crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 0, 1)}, string(in.PeriodKind), calendar)
	if err != nil {
		return empty, "", err
	}
	if !interval.StartAt.Equal(start) {
		return empty, "", invalid("the target must start on the first day of its month or fiscal quarter")
	}
	unit := catalog.Metrics[index].Unit
	if unit == "money" {
		unit = calendar.Currency
	}
	return interval, unit, nil
}

// CreateTarget records an explicit allocation without inferring child targets.
func (s *Service) CreateTarget(ctx context.Context, in crmcontracts.ReportingTargetInput) (crmcontracts.ReportingTarget, error) {
	if err := requireReadWrite(ctx, "sales_target", principal.ActionCreate); err != nil {
		return crmcontracts.ReportingTarget{}, err
	}
	out := crmcontracts.ReportingTarget{Id: openapi_types.UUID(ids.NewV7()), Definition: in, Revision: 1, Version: 1, CreatedAt: s.now()}
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		interval, unit, err := s.validateTarget(ctx, tx, &in)
		if err != nil {
			return err
		}
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
		interval, unit, err := s.validateTarget(ctx, tx, &in)
		if err != nil {
			return err
		}
		if before.Definition.Metric != in.Metric || before.Definition.Scope.Kind != in.Scope.Kind || !sameID(before.Definition.Scope.Id, in.Scope.Id) || !sameID(before.Definition.PipelineId, in.PipelineId) || !interval.StartAt.Equal(before.Interval.StartAt) || !interval.EndAt.Equal(before.Interval.EndAt) || before.Unit != unit {
			return invalid("create a separate target for a different metric, scope or period")
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
