// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Team prospecting: how a team's handoffs ended, what still waits, and each
// member's share, for a manager who coaches. It reads the handoff history and
// the held-meeting source the reporting metrics read, one statement per figure
// for the whole team.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// sdrTeamSmallSample is the fewest rounds a comparison of rates needs. Below
	// it the screen shows the count and its size, never a rate.
	sdrTeamSmallSample = 5
)

// sdrTeamMemberRows is how many member rows a review reads and returns. Only
// those members' figures are aggregated, whatever the team's size.
var sdrTeamMemberRows = 100

// GetSdrTeamReview implements GET /analytics/sdr-team.
func (h sdrSummaryHandlers) GetSdrTeamReview(w http.ResponseWriter, r *http.Request, params crmcontracts.GetSdrTeamReviewParams) {
	requested := RequestedScope{ID: (*ids.UUID)(params.ScopeId)}
	// My teams is the seat's own default, as the evaluator reads it.
	if params.ScopeKind != nil && *params.ScopeKind != crmcontracts.GetSdrTeamReviewParamsScopeKindManagedTeams {
		requested.Kind = string(*params.ScopeKind)
	}
	period := crmcontracts.ReportingSelectionPeriod(crmcontracts.GetSdrTeamReviewParamsPeriodThisWeek)
	if params.Period != nil {
		period = crmcontracts.ReportingSelectionPeriod(*params.Period)
	}
	ctx := r.Context()
	var out crmcontracts.SdrTeamReview
	// Every figure is read from one snapshot, so they agree with one as_of.
	err := h.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		var err error
		out, err = readSdrTeamReview(ctx, tx, requested, period, h.now().UTC())
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// sdrTeamWindow is the half-open span a review counts events in.
type sdrTeamWindow struct{ from, to time.Time }

func readSdrTeamReview(ctx context.Context, tx pgx.Tx, requested RequestedScope, period crmcontracts.ReportingSelectionPeriod, at time.Time) (crmcontracts.SdrTeamReview, error) {
	// No history of team membership is kept, so a team's figures follow its
	// members today, and the answer says so.
	out := crmcontracts.SdrTeamReview{
		AsOf: at, Workflow: crmcontracts.SdrTeamReviewWorkflowQualification, SmallSampleBelow: sdrTeamSmallSample,
		MembershipBasis: crmcontracts.SdrTeamReviewMembershipBasisCurrent, Members: []crmcontracts.SdrTeamMember{},
	}
	if err := auth.Require(ctx, contacts.HandoffObject, principal.ActionRead); err != nil {
		return out, err
	}
	resolved, err := ResolveAnalyticsScope(ctx, tx, requested)
	if err != nil {
		return out, err
	}
	out.Scope = scopeToWire(resolved)
	calendar, err := reportingCalendar(ctx, tx)
	if err != nil {
		return out, err
	}
	out.Interval, err = reporting.Interval(crmcontracts.ReportingSelection{Period: period}, calendar, at)
	if err != nil {
		return out, err
	}
	out.ObservedThrough = out.Interval.EndAt
	if at.Before(out.ObservedThrough) {
		out.ObservedThrough = at
	}
	window := sdrTeamWindow{from: out.Interval.StartAt, to: out.ObservedThrough}
	owners, err := readerMeasurableOwners(ctx, tx)
	if err != nil {
		return out, err
	}
	figures, err := readSdrTeamFigures(ctx, tx, requested, window, at)
	if err != nil {
		return out, err
	}
	if owners.IndividualOff {
		out.MembersWithheld = true
		// One figure a lone colleague makes up withholds the whole review.
		err := requireNoLoneColleague(ctx, tx, figures.contributors)
		if errors.As(err, new(auth.IndividualPerformanceOffError)) {
			out.Withheld = true
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
	out.Workflow, out.Meetings, out.Handoffs = figures.workflow, figures.meetings, figures.handoffs
	if owners.IndividualOff {
		return out, nil
	}
	out.Members, out.MembersTruncated, err = readSdrTeamMembers(ctx, tx, requested, owners, window, at)
	return out, err
}

// sdrTeamFigures are a review's team figures, with the seats behind each one
// so the lone-colleague floor can judge them.
type sdrTeamFigures struct {
	workflow     crmcontracts.SdrTeamReviewWorkflow
	meetings     *crmcontracts.SdrTeamMeetings
	handoffs     *crmcontracts.SdrTeamHandoffs
	contributors [][]ids.UUID
}

func readSdrTeamFigures(ctx context.Context, tx pgx.Tx, requested RequestedScope, window sdrTeamWindow, at time.Time) (sdrTeamFigures, error) {
	out := sdrTeamFigures{workflow: crmcontracts.SdrTeamReviewWorkflowQualification}
	if weekly.MayReadMeetings(ctx) {
		meetings, seats, err := readSdrTeamMeetings(ctx, tx, requested, window, at)
		if err != nil {
			return out, err
		}
		out.meetings, out.contributors = &meetings, append(out.contributors, seats)
	}
	template, err := reporting.FrameworkTemplate(ctx, tx)
	if err != nil || template != crmcontracts.ReportingFrameworkInputTemplateSdr {
		return out, err
	}
	out.workflow = crmcontracts.SdrTeamReviewWorkflowHandoff
	handoffs, seats, err := readSdrTeamHandoffs(ctx, tx, requested, window, at)
	if err != nil {
		return out, err
	}
	out.handoffs, out.contributors = &handoffs, append(out.contributors, seats...)
	return out, nil
}

// readSdrTeamMeetings counts the team's held meetings once each, by the
// population rule the held-meetings reading applies.
func readSdrTeamMeetings(ctx context.Context, tx pgx.Tx, requested RequestedScope, window sdrTeamWindow, at time.Time) (crmcontracts.SdrTeamMeetings, []ids.UUID, error) {
	var b reportingBindings
	_, credited, err := analyticsPopulationExpression(ctx, tx, requested, "credit.id", b.arg, unownedIsExcluded)
	if err != nil {
		return crmcontracts.SdrTeamMeetings{}, nil, err
	}
	held, _, err := heldMeetingSource(ctx, &b, at, window.from, window.to)
	if err != nil {
		return crmcontracts.SdrTeamMeetings{}, nil, err
	}
	population := sqlUnnarrowed
	if credited == "" {
		credited = sqlUnnarrowed
	} else {
		population = "EXISTS (SELECT 1 FROM " + held.seats + " WHERE " + credited + ")"
	}
	query := `SELECT count(DISTINCT a.id),COALESCE(bool_or(` + held.legacy + `),false),COALESCE(array_remove(array_agg(DISTINCT mc.id),NULL),'{}')
 FROM ` + held.from + `
 LEFT JOIN LATERAL (SELECT credit.id FROM ` + held.seats + ` WHERE ` + credited + `) mc ON true
 WHERE ` + held.where + ` AND ` + population
	var out crmcontracts.SdrTeamMeetings
	var seats []ids.UUID
	err = tx.QueryRow(ctx, query, b.values...).Scan(&out.Held, &out.LimitedHistory, &seats)
	return out, seats, err
}

// eventWindowSQL keeps the handoff events at column inside the window.
func eventWindowSQL(b *reportingBindings, column string, window sdrTeamWindow) string {
	return column + ">=" + b.add(window.from) + " AND " + column + "<" + b.add(window.to) + " -- occurred_at raw: a handoff event's instant, not an activity day\n"
}

// readSdrTeamHandoffs reads the handoff figures and the seats behind each.
func readSdrTeamHandoffs(ctx context.Context, tx pgx.Tx, requested RequestedScope, window sdrTeamWindow, at time.Time) (crmcontracts.SdrTeamHandoffs, [][]ids.UUID, error) {
	out := crmcontracts.SdrTeamHandoffs{Reasons: []crmcontracts.SdrHandoffReasonCount{}}
	var seats [][]ids.UUID
	outcomes, outcomeSeats, unattributed, err := readHandoffOutcomes(ctx, tx, requested, window)
	if err != nil {
		return out, nil, err
	}
	out.Outcomes, out.Unattributed, seats = outcomes, unattributed, append(seats, outcomeSeats...)
	cohort, cohortSeats, err := readHandoffCohort(ctx, tx, requested, window)
	if err != nil {
		return out, nil, err
	}
	out.Cohort, seats = cohort, append(seats, cohortSeats...)
	if out.Waiting, err = readHandoffTotals(ctx, tx, requested, at); err != nil {
		return out, nil, err
	}
	waitingSeats, err := readWaitingSubmitters(ctx, tx, requested, at)
	if err != nil {
		return out, nil, err
	}
	reasons, reasonSeats, err := readHandoffReasons(ctx, tx, requested, window)
	if err != nil {
		return out, nil, err
	}
	out.Reasons = reasons
	return out, append(append(seats, waitingSeats...), reasonSeats...), nil
}

// readHandoffOutcomes counts the decisions and submissions made in the window,
// each credited to the submitter its event names. Accepted counts each deal's
// first acceptance once.
func readHandoffOutcomes(ctx context.Context, tx pgx.Tx, requested RequestedScope, window sdrTeamWindow) (crmcontracts.SdrHandoffOutcomes, [][]ids.UUID, int, error) {
	var out crmcontracts.SdrHandoffOutcomes
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "e.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return out, nil, 0, err
	}
	in := orUnnarrowed(population)
	seats := func(status string) string {
		return "COALESCE(array_remove(array_agg(DISTINCT e.submitter_id_at_change) FILTER (WHERE e.to_status=" + b.add(status) + " AND " + in + "),NULL),'{}')"
	}
	count := func(status string) string {
		return "count(*) FILTER (WHERE e.to_status=" + b.add(status) + " AND " + in + ")"
	}
	query := `SELECT ` + count(contacts.HandoffSubmitted) + `,` + count(contacts.HandoffRejected) + `,` + count(contacts.HandoffRecycled) + `,
 count(*) FILTER (WHERE e.submitter_id_at_change IS NULL AND ` + unattributedIn(population) + `),` + seats(contacts.HandoffSubmitted) + `,` + seats(contacts.HandoffRejected) + `,` + seats(contacts.HandoffRecycled) + `
 FROM sdr_handoff_event e WHERE ` + eventWindowSQL(&b, "e.occurred_at", window)
	var unattributed int
	groups := make([][]ids.UUID, 4)
	if err := tx.QueryRow(ctx, query, b.values...).Scan(&out.Submitted, &out.Rejected, &out.Recycled, &unattributed, &groups[0], &groups[1], &groups[2]); err != nil {
		return out, nil, 0, err
	}
	var a reportingBindings
	_, credited, err := analyticsPopulationExpression(ctx, tx, requested, "credit.submitter_id_at_change", a.arg, unownedIsExcluded)
	if err != nil {
		return out, nil, 0, err
	}
	accepted := `SELECT count(*),COALESCE(array_remove(array_agg(DISTINCT credit.submitter_id_at_change),NULL),'{}') FROM ` + acceptedCreditSQL + `
 WHERE ` + orUnnarrowed(credited) + ` AND ` + eventWindowSQL(&a, "credit.occurred_at", window)
	err = tx.QueryRow(ctx, accepted, a.values...).Scan(&out.Accepted, &groups[3])
	return out, groups, unattributed, err
}

// unattributedIn admits an event that names no submitter only for the whole
// company: no team can tell whether it was its own.
func unattributedIn(population string) string {
	if population == "" {
		return sqlUnnarrowed
	}
	return "false"
}

// readHandoffCohort follows the rounds submitted in the window to the event
// that ended each, up to the window's end.
func readHandoffCohort(ctx context.Context, tx pgx.Tx, requested RequestedScope, window sdrTeamWindow) (crmcontracts.SdrHandoffCohort, [][]ids.UUID, error) {
	var out crmcontracts.SdrHandoffCohort
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "s.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return out, nil, err
	}
	// Each figure is read with the seats behind it, so the lone-colleague
	// floor judges every figure on its own.
	seats := func(filter string) string {
		return "COALESCE(array_remove(array_agg(DISTINCT s.submitter_id_at_change) FILTER (WHERE " + filter + "),NULL),'{}')"
	}
	ended := func(status string) (string, string) {
		filter := "outcome.to_status=" + b.add(status)
		return "count(*) FILTER (WHERE " + filter + ")", seats(filter)
	}
	accepted, acceptedSeats := ended(contacts.HandoffAccepted)
	rejected, rejectedSeats := ended(contacts.HandoffRejected)
	recycled, recycledSeats := ended(contacts.HandoffRecycled)
	query := `SELECT count(*),` + accepted + `,` + rejected + `,` + recycled + `,
 count(*) FILTER (WHERE outcome.to_status IS NULL),
 percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM outcome.occurred_at-s.occurred_at)/3600) FILTER (WHERE outcome.to_status IS NOT NULL),
 ` + seats(sqlUnnarrowed) + `,` + acceptedSeats + `,` + rejectedSeats + `,` + recycledSeats + `,` + seats("outcome.to_status IS NULL") + `,` + seats("outcome.to_status IS NOT NULL") + `
 FROM sdr_handoff_event s
 LEFT JOIN LATERAL (SELECT n.to_status,n.occurred_at FROM sdr_handoff_event n WHERE n.handoff_id=s.handoff_id
 AND (n.occurred_at,n.id)>(s.occurred_at,s.id) AND n.occurred_at<` + b.add(window.to) + ` -- occurred_at raw: a handoff event's instant, not an activity day
 ORDER BY n.occurred_at,n.id LIMIT 1) outcome ON true
 WHERE s.to_status=` + b.add(contacts.HandoffSubmitted) + ` AND ` + orUnnarrowed(population) + ` AND ` + eventWindowSQL(&b, "s.occurred_at", window)
	var median *float64
	groups := make([][]ids.UUID, 6)
	if err := tx.QueryRow(ctx, query, b.values...).Scan(&out.Rounds, &out.Accepted, &out.Rejected, &out.Recycled, &out.Waiting, &median,
		&groups[0], &groups[1], &groups[2], &groups[3], &groups[4], &groups[5]); err != nil {
		return out, nil, err
	}
	if median != nil {
		hours := float32(*median)
		out.MedianDecisionHours = &hours
	}
	return out, groups, nil
}

// readWaitingSubmitters lists the seats behind the open handoffs
// readHandoffTotals counts: waiting on a decision, then returned, unassigned
// and blocked. The oldest wait is one of the waiting group's.
func readWaitingSubmitters(ctx context.Context, tx pgx.Tx, requested RequestedScope, at time.Time) ([][]ids.UUID, error) {
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "cycle.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, err
	}
	seats := func(filter string) string {
		return "COALESCE(array_remove(array_agg(DISTINCT cycle.submitter_id_at_change) FILTER (WHERE " + filter + "),NULL),'{}')"
	}
	waiting := "h.status=" + b.add(contacts.HandoffSubmitted)
	query := `SELECT ` + seats(waiting) + `,` + seats("h.status="+b.add(contacts.HandoffRecycled)) + `,
 ` + seats(waiting+" AND h.assigned_to IS NULL") + `,
 ` + seats(waiting+" AND h.assigned_to IS NOT NULL AND NOT ("+identity.LiveMemberSQL("r")+")") + `
 ` + handoffCycleSQL + `
 WHERE h.status IN (` + b.add(contacts.HandoffSubmitted) + `,` + b.add(contacts.HandoffRecycled) + `) AND ` + cycleByAsOf(&b, at) + ` AND ` + orUnnarrowed(population)
	groups := make([][]ids.UUID, 4)
	err = tx.QueryRow(ctx, query, b.values...).Scan(&groups[0], &groups[1], &groups[2], &groups[3])
	return groups, err
}

// readHandoffReasons groups the window's rejections and returns by the reason
// given, most often first.
func readHandoffReasons(ctx context.Context, tx pgx.Tx, requested RequestedScope, window sdrTeamWindow) ([]crmcontracts.SdrHandoffReasonCount, [][]ids.UUID, error) {
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "e.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, nil, err
	}
	query := `SELECT e.to_status,r.label,count(*),COALESCE(array_remove(array_agg(DISTINCT e.submitter_id_at_change),NULL),'{}')
 FROM sdr_handoff_event e JOIN sdr_handoff_reason r ON r.id=e.reason_id
 WHERE e.to_status IN (` + b.add(contacts.HandoffRejected) + `,` + b.add(contacts.HandoffRecycled) + `) AND ` + orUnnarrowed(population) + ` AND ` + eventWindowSQL(&b, "e.occurred_at", window) + `
 GROUP BY e.to_status,r.id,r.label ORDER BY count(*) DESC,r.label,r.id LIMIT 50`
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []crmcontracts.SdrHandoffReasonCount{}
	var seats [][]ids.UUID
	for rows.Next() {
		var reason crmcontracts.SdrHandoffReasonCount
		var behind []ids.UUID
		if err := rows.Scan(&reason.Outcome, &reason.Label, &reason.Count, &behind); err != nil {
			return nil, nil, err
		}
		out, seats = append(out, reason), append(seats, behind)
	}
	return out, seats, rows.Err()
}

// readSdrTeamMembers reads one row per member the reader may measure, in one
// statement for the whole team. A member's meetings count every meeting they
// are credited with, so the rows can add up to more than the team.
func readSdrTeamMembers(ctx context.Context, tx pgx.Tx, requested RequestedScope, owners reporting.MeasurableOwners, window sdrTeamWindow, at time.Time) ([]crmcontracts.SdrTeamMember, bool, error) {
	members, truncated, err := sdrTeamMemberPage(ctx, tx, requested, owners)
	out := []crmcontracts.SdrTeamMember{}
	if err != nil || len(members) == 0 {
		return out, truncated, err
	}
	var b reportingBindings
	seats := b.add(members)
	// A seat that may not read activities gets no meetings column, as it gets
	// no team meeting count.
	met, meetings, joinMet := "", "NULL::bigint", ""
	if weekly.MayReadMeetings(ctx) {
		held, _, err := heldMeetingSource(ctx, &b, at, window.from, window.to)
		if err != nil {
			return nil, false, err
		}
		met = `met AS (SELECT credit.id seat,count(DISTINCT a.id) n FROM ` + held.from + ` CROSS JOIN LATERAL ` + held.seats + `
 WHERE ` + held.where + ` AND credit.id=ANY(` + seats + `) GROUP BY credit.id),
 `
		meetings, joinMet = "COALESCE(met.n,0)", " LEFT JOIN met ON met.seat=u.id"
	}
	submitted, rejected, recycled := b.add(contacts.HandoffSubmitted), b.add(contacts.HandoffRejected), b.add(contacts.HandoffRecycled)
	query := `WITH ` + met + `ev AS (SELECT e.submitter_id_at_change seat,count(*) FILTER (WHERE e.to_status=` + submitted + `) submitted,
 count(*) FILTER (WHERE e.to_status=` + rejected + `) rejected,count(*) FILTER (WHERE e.to_status=` + recycled + `) recycled
 FROM sdr_handoff_event e WHERE e.submitter_id_at_change=ANY(` + seats + `) AND ` + eventWindowSQL(&b, "e.occurred_at", window) + ` GROUP BY 1),
 won AS (SELECT credit.submitter_id_at_change seat,count(*) n FROM ` + acceptedCreditSQL + `
 WHERE credit.submitter_id_at_change=ANY(` + seats + `) AND ` + eventWindowSQL(&b, "credit.occurred_at", window) + ` GROUP BY 1),
 waits AS (SELECT cycle.submitter_id_at_change seat,count(*) n,min(cycle.occurred_at) since ` + handoffCycleSQL + `
 WHERE h.status=` + submitted + ` AND ` + cycleByAsOf(&b, at) + ` AND cycle.submitter_id_at_change=ANY(` + seats + `) GROUP BY 1)
 SELECT u.id,COALESCE(u.display_name,''),` + meetings + `,COALESCE(ev.submitted,0),COALESCE(won.n,0),COALESCE(ev.rejected,0),
 COALESCE(ev.recycled,0),COALESCE(waits.n,0),waits.since
 FROM app_user u` + joinMet + ` LEFT JOIN ev ON ev.seat=u.id LEFT JOIN won ON won.seat=u.id LEFT JOIN waits ON waits.seat=u.id
 WHERE u.id=ANY(` + seats + `) ORDER BY lower(COALESCE(u.display_name,'')),u.id`
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, false, err
	}
	out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.SdrTeamMember, error) {
		var member crmcontracts.SdrTeamMember
		err := row.Scan(&member.Id, &member.Label, &member.MeetingsHeld, &member.Submitted, &member.Accepted, &member.Rejected,
			&member.Recycled, &member.Waiting, &member.OldestWaitingSince)
		return member, err
	})
	return out, truncated, err
}

// sdrTeamMemberPage is the first sdrTeamMemberRows members of the scope the
// reader may measure one by one, in name order, and whether more exist.
func sdrTeamMemberPage(ctx context.Context, tx pgx.Tx, requested RequestedScope, owners reporting.MeasurableOwners) ([]ids.UUID, bool, error) {
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "u.id", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, false, err
	}
	measurable := sqlUnnarrowed
	if !owners.Every {
		measurable = "u.id=ANY(" + b.add(owners.Members) + ")"
	}
	rows, err := tx.Query(ctx, "SELECT u.id FROM app_user u WHERE "+identity.LiveMemberSQL("u")+" AND NOT u.is_agent AND "+
		orUnnarrowed(population)+" AND "+measurable+" ORDER BY lower(COALESCE(u.display_name,'')),u.id LIMIT "+b.add(sdrTeamMemberRows+1), b.values...)
	if err != nil {
		return nil, false, err
	}
	members, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(members) <= sdrTeamMemberRows {
		return members, false, err
	}
	return members[:sdrTeamMemberRows], true, nil
}
