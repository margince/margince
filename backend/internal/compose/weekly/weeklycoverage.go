// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Whether each figure on a review was measured at all.
//
// A zero for a week before the scope held a single deal reads as a verdict on
// the rep. Each family therefore states when its source's first usable record
// in the review's owner scope appeared, read from the same population the
// counts beside it draw on, so a reader can tell an empty week from an
// unmeasured one. A capture connection's date never answers this: a record
// keyed in by hand before any connection existed is still a record.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// figureStarts is the earliest usable record behind each figure family; nil
// where the scope holds none.
type figureStarts struct {
	deals, tasks, meetings, leads, commitments *time.Time
}

// figureCoverageOf states each family's coverage of [start, end) for the
// owners' records this reader may see. planned is false when the review could
// not settle the plan, and commitments then carry no statement at all.
func figureCoverageOf(
	ctx context.Context, tx pgx.Tx, owners []ids.UUID, start, end time.Time, planned bool,
) (*crmcontracts.WeeklyFigureCoverageSet, error) {
	since, err := readFigureStarts(ctx, tx, owners)
	if err != nil {
		return nil, err
	}
	set := &crmcontracts.WeeklyFigureCoverageSet{
		Deals:    figureCoverage(since.deals, start, end, "deal"),
		Tasks:    figureCoverage(since.tasks, start, end, "task"),
		Meetings: figureCoverage(since.meetings, start, end, "meeting"),
		Leads:    figureCoverage(since.leads, start, end, "inbound lead"),
	}
	if planned {
		set.Commitments = figureCoverage(since.commitments, start, end, "weekly plan")
	}
	return set, nil
}

// figureCoverage judges one source's first record against the week. A record
// at the week's first instant measures all of it, so that is recorded, not
// partial.
func figureCoverage(since *time.Time, start, end time.Time, noun string) *crmcontracts.WeeklyFigureCoverage {
	var status crmcontracts.WeeklyFigureCoverageStatus
	var reason string
	switch {
	case since == nil:
		status = crmcontracts.WeeklyFigureCoverageStatusNotRecorded
		reason = fmt.Sprintf("No %s was recorded in this scope, so this figure is not a measurement.", noun)
	case !since.Before(end):
		status = crmcontracts.WeeklyFigureCoverageStatusNotRecorded
		reason = fmt.Sprintf("The first %s in this scope was recorded after this week ended, so this figure is not a measurement.", noun)
	case since.After(start):
		status = crmcontracts.WeeklyFigureCoverageStatusPartial
		reason = fmt.Sprintf("The first %s in this scope was recorded during this week. Only what happened from then on is counted.", noun)
	default:
		return &crmcontracts.WeeklyFigureCoverage{Status: crmcontracts.WeeklyFigureCoverageStatusRecorded, RecordedSince: since}
	}
	return &crmcontracts.WeeklyFigureCoverage{Status: status, RecordedSince: since, Reason: &reason}
}

// readFigureStarts reads every family's first record in one statement, each
// under the scope clause and owner rule its count uses.
//
// Tasks date by created_at alone because the task count admits only rows
// created before the week closed, so an imported task's older occurred_at
// cannot make a past week countable. Meetings take the earlier of created_at
// and occurred_at because their count dates by occurred_at, which a calendar
// import carries from the source. Imported leads start nothing: the lead count
// excludes them, having no response stamps to judge.
func readFigureStarts(ctx context.Context, tx pgx.Tx, owners []ids.UUID) (figureStarts, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	captured := make([]string, 0, len(owners))
	for _, owner := range owners {
		captured = append(captured, "human:"+owner.String())
	}
	owned := fmt.Sprintf("ANY($%d)", arg(owners))
	capturedBy := fmt.Sprintf("ANY($%d)", arg(captured))
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return figureStarts{}, err
	}
	leadScope, err := auth.ScopeClauseFor(ctx, "lead", "l", arg)
	if err != nil {
		return figureStarts{}, err
	}
	taskScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return figureStarts{}, err
	}
	meetingScope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return figureStarts{}, err
	}
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return figureStarts{}, err
	}
	zonePos := arg(zone)
	var s figureStarts
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT min(LEAST(d.created_at, d.closed_at,
		          (SELECT min(h.changed_at) FROM deal_stage_history h WHERE h.deal_id = d.id)))
		     FROM deal d WHERE d.owner_id = %[1]s AND (%[2]s)),
		  (SELECT min(a.created_at) FROM activity a
		    WHERE a.kind = 'task' AND a.archived_at IS NULL AND a.assignee_id = %[1]s AND (%[3]s)),
		  (SELECT min(LEAST(m.created_at, m.occurred_at)) FROM activity m
		    WHERE m.kind = 'meeting' AND m.archived_at IS NULL AND %[4]s AND (%[5]s)),
		  (SELECT min(COALESCE(l.routed_at, l.created_at)) FROM lead l
		    WHERE l.owner_id = %[1]s AND l.archived_at IS NULL AND `+nonImportedLeadSQL+` AND (%[6]s)),
		  (SELECT min(p.local_week_start) FROM weekly_plan p WHERE p.owner_id = %[1]s)::timestamp
		      AT TIME ZONE $%[7]d`,
		owned, orUnbounded(dealScope), taskScope, meetingIsTheirsSQL(owned, capturedBy), meetingScope,
		orUnbounded(leadScope), zonePos), args...).
		Scan(&s.deals, &s.tasks, &s.meetings, &s.leads, &s.commitments)
	if err != nil {
		return figureStarts{}, fmt.Errorf("weekly: reading when each figure's source began: %w", err)
	}
	return s, nil
}

// orUnbounded renders an empty scope clause as the unnarrowed predicate.
func orUnbounded(clause string) string {
	if clause == "" {
		return sqlUnbounded
	}
	return clause
}
