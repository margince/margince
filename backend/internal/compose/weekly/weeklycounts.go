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

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sqlUnbounded is what an unnarrowed reader's scope renders to.
//
// A named constant rather than the literal at each site: the occurrences here
// are one idea — this caller may read every row — and spelling it repeatedly
// invites one that says something subtly different.
const sqlUnbounded = "true"

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
	if dealScope == "" {
		dealScope = sqlUnbounded
	}

	var c Counts
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT count(*) FROM activity a
		    WHERE a.kind = 'task' AND a.archived_at IS NULL
		      AND a.assignee_id = $%[3]d
		      AND a.due_at >= $%[1]d AND a.due_at < $%[2]d AND (%[4]s)),
		  -- Delivered: of the tasks that fell due in the week, the ones
		  -- finished. Scoped to the SAME due window as the line above, because
		  -- the two are read as a ratio — counting everything closed in the
		  -- week against everything due in it can print "4 of 2", which is not
		  -- a number anybody can act on.
		  (SELECT count(*) FROM activity a
		    WHERE a.kind = 'task' AND a.archived_at IS NULL
		      AND a.assignee_id = $%[3]d AND a.is_done
		      AND a.due_at >= $%[1]d AND a.due_at < $%[2]d
		      AND a.done_at IS NOT NULL AND (%[4]s)),
		  -- Carried over: fell due BEFORE the week and is still open. A task
		  -- merely created earlier and not yet due has not been postponed at
		  -- all, and reporting it as carried over tells a rep they are behind
		  -- on work nobody has asked for yet.
		  (SELECT count(*) FROM activity a
		    WHERE a.kind = 'task' AND a.archived_at IS NULL
		      AND a.assignee_id = $%[3]d AND NOT a.is_done
		      AND a.due_at IS NOT NULL AND a.due_at < $%[1]d AND (%[4]s)),
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
		startPos, endPos, userPos, activityScope, dealScope), args...).
		Scan(&c.TasksDue, &c.TasksDone, &c.TasksCarriedOver,
			&c.DealsMoved, &c.DealsWon, &c.DealsLost,
			&c.ProposalsAccepted, &c.ProposalsRejected,
			&c.BriefItemsActed, &c.BriefItemsDismissed)
	if err != nil {
		return Counts{}, fmt.Errorf("weekly: counting the week: %w", err)
	}
	return c, nil
}

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
	if scope == "" {
		scope = sqlUnbounded
	}
	// One window expression, three counts over it.
	const arrived = `COALESCE(l.routed_at, l.created_at) >= $%[1]d
		      AND COALESCE(l.routed_at, l.created_at) < $%[2]d`
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT count(*) FROM lead l
		    WHERE l.owner_id = $%[3]d AND l.archived_at IS NULL
		      AND `+arrived+` AND (%[4]s)),
		  (SELECT count(*) FROM lead l
		    WHERE l.owner_id = $%[3]d AND l.archived_at IS NULL
		      AND `+arrived+`
		      AND l.first_response_at IS NOT NULL AND l.sla_breached_at IS NULL
		      AND (%[4]s)),
		  (SELECT count(*) FROM lead l
		    WHERE l.owner_id = $%[3]d AND l.archived_at IS NULL
		      AND `+arrived+`
		      AND l.sla_breached_at IS NOT NULL AND (%[4]s))`,
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
// cannot answer here — but host_user_id can, and it names the person whose
// meeting it was rather than the person or connector that filed it. Read by
// capturer alone, a meeting a colleague minuted or a calendar connector
// imported counted for whoever recorded it and not for the rep who sat in it,
// so the same meeting moved between reps depending on how it reached the CRM.
//
// The fallback is bounded to rows with NO host: a meeting that names one is
// that person's, and letting the capturer also claim it would count one meeting
// twice across two reps' weeks.
//
// "Left a next step" is a task raised AFTER the meeting against a record the
// meeting was also filed under. Through the SHARED RECORD rather than
// activity.source_activity_id, which does exist: only some writers populate it
// — the transcript accept path names the meeting it read, a task typed by hand
// names nothing — so joining on it would report a rep who writes their own
// follow-ups as having produced none.
//
// Bounded at BOTH ends. A task created months later on the same account is not
// something the meeting produced, and one created after the week closed belongs
// to the week it was created in — a frozen review that changed its answer every
// time somebody added a task would not be frozen.
//
// The heuristic is what the data supports, and it is a heuristic: a task raised
// the Monday after a Friday meeting is plausibly its outcome and is not counted
// here. Tightening that needs source_activity_id on every writer first.
// meetingIsTheirsSQL is the one spelling of "this meeting is that rep's":
// hosted by them, or — where no host was recorded — filed by them.
//
// TWO readers ask it, the headline count here and the funnel in
// weeklyscorecard.go, and they must not disagree: a meeting credited to
// different people by the two panels is one page contradicting itself about the
// same week. The caller supplies its own placeholders because the two queries
// number their arguments differently.
//
// The fallback is bounded to rows with NO host on purpose. A meeting naming one
// is that person's, and letting its recorder also claim it would count one
// meeting twice across two reps.
//
// Held by: TestTheMeetingAttributionHasOneSpelling (meetingattribution_test.go)
func meetingIsTheirsSQL(hostPos, capturedPos string) string {
	return fmt.Sprintf("(m.host_user_id = %s OR (m.host_user_id IS NULL AND m.captured_by = %s))",
		hostPos, capturedPos)
}

func countWeekMeetings(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (held, withNextStep int, err error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos := arg(start), arg(end)
	capturedPos := arg("human:" + userID.String())
	hostPos := arg(userID)
	scope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return 0, 0, err
	}
	heldByRep := `m.kind = 'meeting' AND m.archived_at IS NULL
		      AND m.meeting_status = 'held'
		      AND ` + meetingIsTheirsSQL("$%[5]d", "$%[3]d") + `
		      AND m.occurred_at >= $%[1]d AND m.occurred_at < $%[2]d
		      AND (%[4]s)`
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT
		  (SELECT count(*) FROM activity m WHERE `+heldByRep+`),
		  (SELECT count(*) FROM activity m WHERE `+heldByRep+`
		      AND EXISTS (
		        SELECT 1 FROM activity_link ml
		          JOIN activity_link tl ON tl.entity_type = ml.entity_type
		            AND tl.person_id IS NOT DISTINCT FROM ml.person_id
		            AND tl.company_id IS NOT DISTINCT FROM ml.company_id
		            AND tl.deal_id IS NOT DISTINCT FROM ml.deal_id
		            AND tl.lead_id IS NOT DISTINCT FROM ml.lead_id
		            AND tl.project_id IS NOT DISTINCT FROM ml.project_id
		          JOIN activity task ON task.id = tl.activity_id
		         WHERE ml.activity_id = m.id AND task.kind = 'task'
		           AND task.archived_at IS NULL
		           AND task.created_at >= m.occurred_at
		           AND task.created_at < $%[2]d))`,
		startPos, endPos, capturedPos, scope, hostPos), args...).
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
	if dealScope == "" {
		dealScope = sqlUnbounded
	}

	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH closed AS (
			SELECT d.id, d.name, d.status AS outcome, NULL::text AS to_stage,
			       d.amount_minor, d.currency, d.closed_at AS at
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
		startPos, endPos, userPos, capPos, dealScope), args...)
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
