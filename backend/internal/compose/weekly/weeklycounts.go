// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What a rep's week came to, counted.
//
// Split from the assembly beside it: that decides WHICH week is under review
// and writes the row, and these are the queries that answer what happened in
// it. Each carries its own scope clause and its own dating rule, and several
// take a paragraph to justify.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// sqlUnbounded is what an unnarrowed reader's scope renders to.
//
// A named constant rather than the literal at each site: the occurrences here
// are one idea — this caller may read every row — and spelling it repeatedly
// invites one that says something subtly different.
const sqlUnbounded = "true"

// orUnbounded renders an empty scope clause as the unnarrowed predicate.
func orUnbounded(clause string) string {
	if clause == "" {
		return sqlUnbounded
	}
	return clause
}

// countWeek tallies the week. One statement, because ten round trips for ten
// integers is ten chances for the numbers to describe different moments.
func countWeek(ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time) (Counts, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos := arg(start), arg(end), arg(userID)
	// Activities carry no owner, so their visibility comes from the records
	// they link to and the audience the writer set — never hand-rolled.
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return Counts{}, err
	}
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return Counts{}, err
	}
	dealScope = orUnbounded(dealScope)

	verbsPos := arg(privacy.ScrubVerbs())
	completion := taskCompletionAtSQL("a", fmt.Sprintf("$%d", endPos), fmt.Sprintf("$%d", verbsPos))
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return Counts{}, err
	}
	current := storekit.CurrentTaskClause("a", end, identities, arg)
	var c Counts
	err = tx.QueryRow(ctx, fmt.Sprintf(`
        WITH tasks AS (
          SELECT `+storekit.TaskDueSQL("a")+` AS due_at, %[6]s AS completed_at FROM activity a
           WHERE a.kind='task' AND a.archived_at IS NULL AND (a.is_done OR %[7]s)
             AND a.created_at < $%[2]d AND a.assignee_id=$%[3]d AND (%[4]s)
             AND ((`+storekit.TaskDueSQL("a")+` >= $%[1]d AND `+storekit.TaskDueSQL("a")+` < $%[2]d)
               OR a.done_at >= $%[1]d OR (`+storekit.TaskDueSQL("a")+` < $%[2]d AND NOT a.is_done)
               OR a.updated_at >= $%[2]d))
        SELECT
          (SELECT count(*) FROM tasks WHERE completed_at >= $%[1]d AND completed_at < $%[2]d),
          (SELECT count(*) FROM tasks WHERE due_at >= $%[1]d AND due_at < $%[2]d),
          (SELECT count(*) FROM tasks WHERE due_at >= $%[1]d AND due_at < $%[2]d AND completed_at < $%[2]d),
          (SELECT count(*) FROM tasks WHERE due_at < $%[2]d AND (completed_at IS NULL OR completed_at >= $%[2]d)),
		  -- Moved EXCLUDES deals that closed this week. The stage change that
		  -- closed a deal is the same event as the win, and counting both tells
		  -- the rep one thing twice — and the deal lines below already exclude
		  -- them, so counting them here made the number and the list disagree.
		  (SELECT count(DISTINCT h.deal_id) FROM deal_stage_history h
		     JOIN deal d ON d.id = h.deal_id
		    WHERE h.changed_at >= $%[1]d AND h.changed_at < $%[2]d
		      AND d.owner_id = $%[3]d AND (%[5]s)
		      AND NOT (d.status IN ('won', 'lost')
		               AND d.closed_at >= $%[1]d AND d.closed_at < $%[2]d)),
		  (SELECT count(*) FROM deal d
		    WHERE d.status = 'won' AND d.closed_at >= $%[1]d AND d.closed_at < $%[2]d
		      AND d.owner_id = $%[3]d AND (%[5]s)),
		  (SELECT count(*) FROM deal d
		    WHERE d.status = 'lost' AND d.closed_at >= $%[1]d AND d.closed_at < $%[2]d
		      AND d.owner_id = $%[3]d AND (%[5]s)),
		  (SELECT count(*) FROM approval ap
		    WHERE ap.status = 'approved' AND ap.decided_by = $%[3]d
		      AND ap.decided_at >= $%[1]d AND ap.decided_at < $%[2]d),
		  (SELECT count(*) FROM approval ap
		    WHERE ap.status = 'rejected' AND ap.decided_by = $%[3]d
		      AND ap.decided_at >= $%[1]d AND ap.decided_at < $%[2]d),
		  (SELECT count(*) FROM brief_item bi
		     JOIN brief_run br ON br.id = bi.brief_run_id
		    WHERE br.user_id = $%[3]d AND bi.state = 'acted'
		      AND bi.state_at >= $%[1]d AND bi.state_at < $%[2]d),
		  (SELECT count(*) FROM brief_item bi
		     JOIN brief_run br ON br.id = bi.brief_run_id
		    WHERE br.user_id = $%[3]d AND bi.state = 'dismissed'
		      AND bi.state_at >= $%[1]d AND bi.state_at < $%[2]d)`,
		startPos, endPos, userPos, activityScope, dealScope, completion, current), args...).
		Scan(&c.TasksCompleted, &c.TasksDue, &c.TasksDone, &c.TasksCarriedOver,
			&c.DealsMoved, &c.DealsWon, &c.DealsLost,
			&c.ProposalsAccepted, &c.ProposalsRejected,
			&c.BriefItemsActed, &c.BriefItemsDismissed)
	if err != nil {
		return Counts{}, fmt.Errorf("weekly: counting the week: %w", err)
	}
	return c, nil
}

// NonImportedLeadSQL admits a lead under alias l that no import wrote. Response
// timing reads only those, here and in the reporting catalog.
const NonImportedLeadSQL = "NOT starts_with(COALESCE(l.source_system, ''), '" + provenance.ReservedSourceSystemPrefix + "')"

// leadArrivedInWeekSQL is the week's lead cohort under alias l, with the
// window bound at positions 1 and 2. A lead is in it when it arrived in the
// window, is not archived and no import wrote it. Both the weekly counts and
// the frozen scorecard read it, so they describe the same leads.
const leadArrivedInWeekSQL = `COALESCE(l.routed_at, l.created_at) >= $%[1]d
		      AND COALESCE(l.routed_at, l.created_at) < $%[2]d
		      AND l.archived_at IS NULL AND ` + NonImportedLeadSQL

// Both the weekly counts and the funnel scorecard use the arrival cohort and
// the same closing instant. A missing breach stamp does not prove an SLA target.
const (
	responseRecordedInWeekSQL = `l.first_response_at < $%[2]d AND ` + NonImportedLeadSQL
	breachRecordedInWeekSQL   = `l.sla_breached_at < $%[2]d AND ` + NonImportedLeadSQL
)

// countWeekLeads counts how the week's inbound leads were answered.
//
// Dated by COALESCE(routed_at, created_at), which is the rule the SLA writer
// itself applies (leadsla.go): a lead nobody routed still has a first-response
// target running from when it arrived. Keying on routed_at alone would drop
// every unrouted lead from the week — the ones most likely to have been missed.
//
// Answered and breached are read from the two stamps the SLA writer maintains,
// never recomputed from a policy that may have changed since: a week is judged
// by the target that applied to it.
func countWeekLeads(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (routed, answered, breached int, err error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos := arg(start), arg(end), arg(userID)
	// A rep's OWN leads, but the scope clause still applies: a rep whose grants
	// were narrowed after the week must not read back rows they can no longer
	// see, and the review is assembled under their own principal
	// (weeklyjobs.go binds it) precisely so this holds.
	scope, err := auth.ScopeClauseFor(ctx, "lead", "l", arg)
	if err != nil {
		return 0, 0, 0, err
	}
	scope = orUnbounded(scope)
	// One cohort, three counts over it.
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT count(*) FROM lead l
		    WHERE l.owner_id = $%[3]d
		      AND `+leadArrivedInWeekSQL+` AND (%[4]s)),
		  (SELECT count(*) FROM lead l
		    WHERE l.owner_id = $%[3]d
		      AND `+leadArrivedInWeekSQL+`
		      AND `+responseRecordedInWeekSQL+`
		      AND (%[4]s)),
		  (SELECT count(*) FROM lead l
		    WHERE l.owner_id = $%[3]d
		      AND `+leadArrivedInWeekSQL+`
		      AND `+breachRecordedInWeekSQL+` AND (%[4]s))`,
		startPos, endPos, userPos, scope), args...).
		Scan(&routed, &answered, &breached)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("weekly: counting the week's leads: %w", err)
	}
	return routed, answered, breached, nil
}

// countWeekMeetings counts the meetings the rep held and how many left a next
// step behind.
//
// A booking cancelled or no-showed is not a conversation the week can be
// credited with, so only `held` counts.
//
// Attributed by HOST, falling back to the capturer only where no host is
// recorded.
//
// assignee_id is reserved for tasks by the activity_task_fields CHECK, so it
// cannot answer here — but host_user_id can, and it names the contact whose
// meeting it was rather than the contact or connector that filed it. Read by
// capturer alone, a meeting a colleague minuted or a calendar connector
// imported counted for whoever recorded it and not for the rep who sat in it,
// so the same meeting moved between reps depending on how it reached the CRM.
//
// The fallback is bounded to rows with NO host: a meeting that names one is
// that contact's, and letting the capturer also claim it would count one meeting
// twice across two reps' weeks.
//
// A recorded next step must name this meeting as its source and be readable
// by the member. A task merely sharing an account does not prove an outcome.
// Only tasks created from the meeting before the week closed count.
//
// meetingIsTheirsSQL is "this meeting is that rep's" for the weekly figures:
// the meeting credit rule analytics counts by, over alias `m`. Filing a
// meeting, importing a copy, or an invitation nobody accepted credits nobody.
// A meeting two reps attended is in both their weeks: both spent the time.
//
// Three readers ask it: the headline count here, the funnel in
// weeklyscorecard.go and the coverage in weeklycoverage.go. Plan capacity asks
// the wider responsibility rule instead, because an unanswered invitation
// still holds the time.
//
// Held by: TestTheMeetingAttributionHasOneSpelling (meetingattribution_test.go)
func meetingIsTheirsSQL(seat string) string {
	return storekit.MeetingCreditSQL("m", seat)
}

// NextStepTaskSQL is "task is a next step recorded from meeting": a live task
// made from it once the meeting began (occurredday.StartSQL). Each caller adds
// its own upper bound and scope.
func NextStepTaskSQL(task, meeting string) string {
	return task + ".source_activity_id = " + meeting + ".id AND " + task + ".kind = 'task' AND " +
		task + ".archived_at IS NULL AND " + task + ".created_at >= " + occurredday.StartSQL(meeting)
}

func countWeekMeetings(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, now, start, end time.Time,
) (held, withNextStep int, err error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos := arg(start), arg(end)
	hostPos := arg(userID)
	// The reader Analytics counts a held meeting through. So a meeting redated
	// after it was held stays in its held week on both surfaces.
	//
	// Analytics also refuses a row whose history has a later entry, so an
	// evaluation reads the same way twice for its receipts. A week is
	// assembled once and its figure stored, so there is nothing to reproduce.
	// Refusing such a row here would drop a meeting whose status was recorded
	// after the assembly's clock.
	transition := storekit.MeetingHeldAsOfSQL("m", fmt.Sprintf("$%d", arg(now)))
	scope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return 0, 0, err
	}
	taskScope, err := auth.ActivityContentClause(ctx, "task", arg)
	if err != nil {
		return 0, 0, err
	}
	// NULL admitted, because this window is a week that has already passed: a
	// meeting nothing said was off, whose time has come and gone, is one that
	// happened. A calendar connector records no status at all, so the strict
	// form counted none of a rep's synced meetings in their own review.
	// The transition's status when it recorded one, so a meeting marked held
	// and later reopened is not still counted.
	status := "COALESCE(held.status,m.meeting_status)"
	// A start whose day cannot be settled is read under either day, as the
	// Analytics reader reads it.
	within := "(" + occurredday.WithinAtSQL(transition.Start, transition.Precision, "$%[1]d", "$%[2]d") +
		" OR (" + transition.UnknownDay + " AND (" +
		occurredday.WithinAtSQL(transition.Start, "'instant'", "$%[1]d", "$%[2]d") + " OR " +
		occurredday.WithinAtSQL(transition.Start, "'day'", "$%[1]d", "$%[2]d") + ")))"
	heldByRep := `m.kind = 'meeting' AND m.archived_at IS NULL
		      AND (` + status + ` IS NULL OR ` + status + ` = 'held')
		      AND ` + meetingIsTheirsSQL("$%[3]d") + `
		      AND ` + within + `
		      AND (%[4]s)`
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT count(*) FROM activity m `+transition.Join+` WHERE `+heldByRep+`),
		  (SELECT count(*) FROM activity m `+transition.Join+` WHERE `+heldByRep+`
		      AND EXISTS (
		        SELECT 1 FROM activity task
		         WHERE %[6]s
		           AND task.created_at < $%[2]d AND (%[5]s)))`,
		startPos, endPos, hostPos, scope, taskScope, NextStepTaskSQL("task", "m")), args...).
		Scan(&held, &withNextStep)
	if err != nil {
		return 0, 0, fmt.Errorf("weekly: counting the week's meetings: %w", err)
	}
	return held, withNextStep, nil
}

// readWeekDeals reads the deals this rep's week is about, LABELS AND ALL.
//
// The labels are read here so they can be frozen: the row written from this is
// the record of what the week was, and joining a name at read time would let a
// rename in March rewrite a review from January — or a deletion erase the line
// entirely.
//
// Won and lost lines come first because they are what a week is remembered by;
// within that, most recent first.
func readWeekDeals(ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time) ([]DealLine, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos, capPos := arg(start), arg(end), arg(userID), arg(dealLineCap)
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	dealScope = orUnbounded(dealScope)

	// The week's closed list prints each deal's figure beside its name, so the
	// mask reaches it here as it does the deal list. The `moved` leg below
	// already selects a typed NULL for this column, so a masked row lines up
	// with a shape this UNION carries anyway.
	amountSQL, err := auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH closed AS (
			SELECT d.id, d.name, d.status AS outcome, NULL::text AS to_stage,
			       %[6]s, d.currency, d.closed_at AS at
			  FROM deal d
			 WHERE d.status IN ('won', 'lost')
			   AND d.closed_at >= $%[1]d AND d.closed_at < $%[2]d
			   AND d.owner_id = $%[3]d AND (%[5]s)
		), moved AS (
			SELECT DISTINCT ON (h.deal_id)
			       d.id, d.name, 'moved'::text AS outcome, s.name AS to_stage,
			       NULL::bigint AS amount_minor, NULL::text AS currency,
			       h.changed_at AS at
			  FROM deal_stage_history h
			  JOIN deal d ON d.id = h.deal_id
			  LEFT JOIN stage s ON s.id = h.to_stage_id
			 WHERE h.changed_at >= $%[1]d AND h.changed_at < $%[2]d
			   AND d.owner_id = $%[3]d AND (%[5]s)
			   -- A deal that CLOSED this week is reported as won or lost, not
			   -- as a move: the stage change that closed it is the same event,
			   -- and listing both would tell the rep one thing twice.
			   AND d.id NOT IN (SELECT id FROM closed)
			 -- id breaks a tie, so two moves at one instant freeze the same
			 -- label every time rather than whichever the planner saw first.
			 ORDER BY h.deal_id, h.changed_at DESC, h.id DESC
		)
		SELECT id, name, outcome, to_stage, amount_minor, currency, at
		  FROM (SELECT * FROM closed UNION ALL SELECT * FROM moved) lines
		 ORDER BY (outcome = 'moved'), at DESC, id
		 LIMIT $%[4]d`,
		startPos, endPos, userPos, capPos, dealScope, amountSQL), args...)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the week's deals: %w", err)
	}
	defer rows.Close()

	var lines []DealLine
	for rows.Next() {
		var line DealLine
		var toStage, currency *string
		if err := rows.Scan(&line.DealID, &line.Label, &line.Outcome, &toStage,
			&line.AmountMinor, &currency, &line.OccurredAt); err != nil {
			return nil, err
		}
		if toStage != nil {
			line.ToStageLabel = *toStage
		}
		if currency != nil {
			line.Currency = *currency
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// insertDealLines freezes the week's deal lines onto the review.
func insertDealLines(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, lines []DealLine) error {
	for _, line := range lines {
		// Money is a pair or it is absent — the CHECK says so, and a bare
		// amount is a number nobody can read.
		var currency *string
		if line.AmountMinor != nil && line.Currency != "" {
			currency = &line.Currency
		}
		var toStage *string
		if line.ToStageLabel != "" {
			toStage = &line.ToStageLabel
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO weekly_review_deal (weekly_review_id, deal_id, deal_label,
			    outcome, to_stage_label, amount_minor_at_close, currency_at_close, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			reviewID, line.DealID, line.Label, line.Outcome, toStage,
			line.AmountMinor, currency, line.OccurredAt); err != nil {
			return fmt.Errorf("weekly: freezing a deal line: %w", err)
		}
	}
	return nil
}
