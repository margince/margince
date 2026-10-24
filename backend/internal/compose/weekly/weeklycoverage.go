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
//
// Commitments follow one rule at both scopes: a figure states its coverage only
// where every week it sums settled a plan. A rep's week settles it in
// measureWeek; a team's sum inherits the statements its members' weeks froze.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// planNoun names the commitments source in a coverage reason.
const planNoun = "weekly plan"

// figureStarts is the earliest usable record behind each record family; nil
// where the scope holds none.
type figureStarts struct {
	deals, tasks, meetings, leads *time.Time
	// leadsRead is false for a seat without lead read, whose leads get no statement.
	leadsRead bool
}

// figureCoverageOf states the record families' coverage of [start, end) for
// the owners' records this reader may see. Commitments are the caller's to add.
func (said teamCopy) figureCoverageOf(
	ctx context.Context, tx pgx.Tx, owners []ids.UUID, start, end time.Time,
) (*crmcontracts.WeeklyFigureCoverageSet, error) {
	since, err := readFigureStarts(ctx, tx, owners, end)
	if err != nil {
		return nil, err
	}
	set := &crmcontracts.WeeklyFigureCoverageSet{
		Deals:    said.figureCoverage(since.deals, start, end, "deal"),
		Tasks:    said.figureCoverage(since.tasks, start, end, "task"),
		Meetings: said.figureCoverage(since.meetings, start, end, "meeting"),
	}
	if since.leadsRead {
		set.Leads = said.figureCoverage(since.leads, start, end, "inbound lead")
	}
	return set, nil
}

// measureRepCoverage freezes one rep's coverage onto their summary. planned is
// false when the week settled no plan, and commitments then state nothing.
func measureRepCoverage(
	ctx context.Context, tx pgx.Tx, summary *crmcontracts.WeeklyNumericSummary,
	userID ids.UUID, start, end time.Time, planned bool,
) error {
	said := teamCopyFor(string(identity.BaseLanguageForRecord(ctx, tx)))
	set, err := said.figureCoverageOf(ctx, tx, []ids.UUID{userID}, start, end)
	if err != nil {
		return err
	}
	readable, err := mayReadPlans(ctx)
	if err != nil {
		return err
	}
	if planned && readable {
		if set.Commitments, err = said.planCoverage(ctx, tx, userID, start, end); err != nil {
			return err
		}
	}
	summary.FigureCoverage = set
	return nil
}

// mayReadPlans asks for weekly_plan read. A refused seat gets no commitments
// statement rather than a guess; any other error is real.
func mayReadPlans(ctx context.Context) (bool, error) {
	err := auth.Require(ctx, "weekly_plan", principal.ActionRead)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

// planCoverage dates a rep's commitments from their first plan week.
func (said teamCopy) planCoverage(
	ctx context.Context, tx pgx.Tx, owner ids.UUID, start, end time.Time,
) (*crmcontracts.WeeklyFigureCoverage, error) {
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	var since *time.Time
	if err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT (SELECT min(p.local_week_start) FROM weekly_plan p WHERE p.owner_id = $%d)::timestamp
		       AT TIME ZONE $%d`, arg(owner), arg(zone)), args...).Scan(&since); err != nil {
		return nil, fmt.Errorf("weekly: reading when the plan began: %w", err)
	}
	return said.figureCoverage(since, start, end, planNoun), nil
}

// memberPlans folds the commitments statements of the member weeks a team
// total sums.
type memberPlans struct {
	counted, stated int
	since           *time.Time
}

// add takes one counted member week's frozen statement, nil where it made none.
func (m *memberPlans) add(plans *crmcontracts.WeeklyFigureCoverage) {
	m.counted++
	if plans == nil {
		return
	}
	m.stated++
	if plans.RecordedSince != nil && (m.since == nil || plans.RecordedSince.Before(*m.since)) {
		m.since = plans.RecordedSince
	}
}

// coverage states the team's commitments only when every counted member week
// stated its own: one silent week makes the summed figure unmeasured.
func (m memberPlans) coverage(start, end time.Time, said teamCopy) *crmcontracts.WeeklyFigureCoverage {
	if m.counted == 0 || m.stated < m.counted {
		return nil
	}
	return said.figureCoverage(m.since, start, end, planNoun)
}

// figureCoverage judges one source's first record against the week. A record
// at the week's first instant measures all of it, so that is recorded, not
// partial.
func (said teamCopy) figureCoverage(since *time.Time, start, end time.Time, noun string) *crmcontracts.WeeklyFigureCoverage {
	var status crmcontracts.WeeklyFigureCoverageStatus
	var reason string
	if phrase, ok := coverageNouns[noun]; ok {
		noun = said.Say(phrase)
	}
	switch {
	case since == nil:
		status = crmcontracts.WeeklyFigureCoverageStatusNotRecorded
		reason = fmt.Sprintf(said.Say(coverageWords.None), noun)
	case !since.Before(end):
		status = crmcontracts.WeeklyFigureCoverageStatusNotRecorded
		reason = fmt.Sprintf(said.Say(coverageWords.After), noun)
	case since.After(start):
		status = crmcontracts.WeeklyFigureCoverageStatusPartial
		reason = fmt.Sprintf(said.Say(coverageWords.During), noun)
	default:
		return &crmcontracts.WeeklyFigureCoverage{Status: crmcontracts.WeeklyFigureCoverageStatusRecorded, RecordedSince: since}
	}
	return &crmcontracts.WeeklyFigureCoverage{Status: status, RecordedSince: since, Reason: &reason}
}

// readFigureStarts reads every record family's first record in one statement,
// each under the scope clause and owner rule its count uses.
//
// Tasks date by created_at alone because the task count admits only rows
// created before the week closed, so an imported task's older occurred_at
// cannot make a past week countable. Meetings take the earlier of created_at
// and occurred_at because their count dates by occurred_at, which a calendar
// import carries from the source. Imported leads start nothing: the lead count
// excludes them, having no response stamps to judge. Without lead read the lead
// arm is rendered FALSE, so the statement stays one and the lead table is not read.
func readFigureStarts(ctx context.Context, tx pgx.Tx, owners []ids.UUID, asOf time.Time) (figureStarts, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	owned := fmt.Sprintf("ANY($%d)", arg(owners))
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return figureStarts{}, err
	}
	var s figureStarts
	s.leadsRead = auth.ReadGranted(ctx, "lead")
	leadScope := "FALSE"
	if s.leadsRead {
		if leadScope, err = auth.ScopeClauseFor(ctx, "lead", "l", arg); err != nil {
			return figureStarts{}, err
		}
		leadScope = orUnbounded(leadScope)
	}
	taskScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return figureStarts{}, err
	}
	meetingScope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return figureStarts{}, err
	}
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return figureStarts{}, err
	}
	current := storekit.CurrentTaskClause("a", asOf, identities, arg)
	err = tx.QueryRow(ctx, fmt.Sprintf(`
 SELECT
		  (SELECT min(LEAST(d.created_at, d.closed_at,
		          (SELECT min(h.changed_at) FROM deal_stage_history h WHERE h.deal_id = d.id)))
		     FROM deal d WHERE d.owner_id = %[1]s AND (%[2]s)),
		  (SELECT min(a.created_at) FROM activity a
		    WHERE a.kind = 'task' AND a.archived_at IS NULL AND a.assignee_id = %[1]s AND (%[3]s) AND (a.is_done OR %[7]s)),
		  (SELECT min(LEAST(m.created_at, m.occurred_at)) FROM activity m
		    WHERE m.kind = 'meeting' AND m.archived_at IS NULL AND %[4]s AND (%[5]s)),
		  (SELECT min(COALESCE(l.routed_at, l.created_at)) FROM lead l
		    WHERE l.owner_id = %[1]s AND l.archived_at IS NULL AND `+NonImportedLeadSQL+` AND (%[6]s))`,
		owned, orUnbounded(dealScope), taskScope, meetingIsTheirsSQL(owned), meetingScope,
		leadScope, current), args...).
		Scan(&s.deals, &s.tasks, &s.meetings, &s.leads)
	if err != nil {
		return figureStarts{}, fmt.Errorf("weekly: reading when each figure's source began: %w", err)
	}
	return s, nil
}
