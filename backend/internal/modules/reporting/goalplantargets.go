// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GoalCell is one owner's monthly goal target as it stands.
type GoalCell struct {
	Value    int64
	Unit     string
	TargetID ids.UUID
	Revision int64
	Retired  bool
}

// GoalYear holds an owner's goal targets by month, 1 to 12.
type GoalYear map[int]GoalCell

// yearTargets reads the goal targets the owners hold in a calendar year, keyed
// by the local date each month was declared with. Matching on that date keeps a
// goal in its month when the reporting zone changes later.
func yearTargets(ctx context.Context, tx pgx.Tx, owners []ids.UUID, year int, retired bool) (map[ids.UUID]GoalYear, error) {
	out := map[ids.UUID]GoalYear{}
	if len(owners) == 0 {
		return out, nil
	}
	var b bindings
	where := "t.metric=" + b.add(goalMetric) + " AND t.scope_kind='owner' AND t.scope_id=ANY(" + b.add(owners) + ") AND t.pipeline_id IS NULL AND t.period_kind='month'" +
		" AND r.definition->>'period_start'>=" + b.add(fmt.Sprintf("%04d-01-01", year)) + " AND r.definition->>'period_start'<" + b.add(fmt.Sprintf("%04d-01-01", year+1))
	if !retired {
		where += " AND " + activeTarget
	}
	query := "SELECT t.scope_id,r.definition->>'period_start',(r.definition->>'value')::bigint,t.unit,t.id,t.revision," + targetRetiredSQL + targetFrom + " WHERE " + where
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var owner ids.UUID
		var start string
		var cell GoalCell
		if err := rows.Scan(&owner, &start, &cell.Value, &cell.Unit, &cell.TargetID, &cell.Revision, &cell.Retired); err != nil {
			return nil, err
		}
		day, err := time.Parse(time.DateOnly, start)
		if err != nil {
			return nil, err
		}
		if out[owner] == nil {
			out[owner] = GoalYear{}
		}
		out[owner][int(day.Month())] = cell
	}
	return out, rows.Err()
}

// OwnerGoalYears reads each owner's live monthly goals for a calendar year.
// readable is false when the reader may not read targets at all. An owner the
// reader may not measure is left out, so the caller tells withheld from unset.
func (s *Service) OwnerGoalYears(ctx context.Context, tx pgx.Tx, owners []ids.UUID, year int) (goals map[ids.UUID]GoalYear, readable bool, err error) {
	if !auth.Allows(ctx, "sales_target", principal.ActionRead) {
		return nil, false, nil
	}
	goals, err = yearTargets(ctx, tx, owners, year, false)
	return goals, true, err
}

// GoalPlanDrift lists the owners whose published plan for year no longer
// matches its targets, because another route changed one.
func (s *Service) GoalPlanDrift(ctx context.Context, tx pgx.Tx, owners []ids.UUID, year int) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	if len(owners) == 0 || !auth.Allows(ctx, "sales_target", principal.ActionRead) {
		return out, nil
	}
	rows, err := tx.Query(ctx, `SELECT p.owner_id, r.targets FROM sales_goal_plan p
		JOIN sales_goal_plan_revision r ON r.plan_id=p.id AND r.revision=p.published_revision
		WHERE p.owner_id=ANY($1) AND p.year=$2`, owners, year)
	if err != nil {
		return nil, err
	}
	links := map[ids.UUID][]goalTargetLink{}
	for rows.Next() {
		var owner ids.UUID
		var published []goalTargetLink
		if err := rows.Scan(&owner, &published); err != nil {
			rows.Close()
			return nil, err
		}
		links[owner] = published
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	current, err := yearTargets(ctx, tx, owners, year, true)
	if err != nil {
		return nil, err
	}
	for owner, published := range links {
		out[owner] = drifted(published, current[owner])
	}
	return out, nil
}

// drifted reports a publication whose targets moved on without it.
func drifted(published []goalTargetLink, current GoalYear) bool {
	for _, link := range published {
		cell, ok := current[link.Month]
		if !ok || cell.TargetID != link.TargetID || cell.Revision != link.Revision || cell.Retired {
			return true
		}
	}
	return false
}

// publishMonth writes one month of a plan through the target route's own
// writers, reusing the month's existing target, retired or not.
func (s *Service) publishMonth(ctx context.Context, tx pgx.Tx, owner ids.UUID, year, month int, value int64, retire bool, reason string, existing GoalYear) (goalTargetLink, error) {
	id := openapi_types.UUID(owner)
	in := crmcontracts.ReportingTargetInput{
		Metric: goalMetric, Scope: crmcontracts.ReportingScope{Kind: reportingOwner, Id: &id}, PeriodKind: crmcontracts.ReportingTargetInputPeriodKindMonth,
		PeriodStart: openapi_types.Date{Time: time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)},
		Value:       value, Reason: reason,
	}
	if retire {
		in.Retired = &retire
	}
	var out crmcontracts.ReportingTarget
	var err error
	if cell, ok := existing[month]; ok {
		if err := auth.Require(ctx, "sales_target", principal.ActionUpdate); err != nil {
			return goalTargetLink{}, err
		}
		before, readErr := s.target(ctx, tx, cell.TargetID, true)
		if readErr != nil {
			return goalTargetLink{}, readErr
		}
		out, err = s.updateTargetTx(ctx, tx, before, in)
	} else {
		if err := auth.Require(ctx, "sales_target", principal.ActionCreate); err != nil {
			return goalTargetLink{}, err
		}
		out, err = s.createTargetTx(ctx, tx, in)
	}
	if err != nil {
		return goalTargetLink{}, refusedField("months["+strconv.Itoa(month-1)+"]", err)
	}
	return goalTargetLink{Month: month, TargetID: ids.UUID(out.Id), Revision: out.Revision}, nil
}
