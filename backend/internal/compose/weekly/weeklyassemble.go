// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Measuring one rep's week.
//
// Every count here is written ONCE, when the week closes, and read back
// unchanged forever after. A retrospective that recomputes on read answers
// differently depending on when you open it — the deal you closed on Friday
// gets reclassified in March because somebody edited its stage — and a record
// of a past week that does that is not a record.
//
// Each read runs under the acting rep's own principal and row scope. The job
// that drives this binds that principal per rep; nothing here widens it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealLineCap bounds how many deal lines one week records.
//
// A retrospective is read in a few minutes; a hundred lines is an export, not
// a review. The counts stay complete either way — the cap bounds what is
// LISTED, never what is counted, so a busy week reads "12 deals moved" beside
// the most recent of them rather than a truncated number.
const dealLineCap = 20

// WeekStartOf is the Monday of the local week containing now, in the
// installation's reporting zone.
//
// Monday because the review is about a working week and the product's own
// language says so. Derived through briefs.LocalDayAt rather than a second
// spelling of the zone lookup: two answers to "what day is it here" is how a
// Sunday-night job files its work under the wrong week.
func WeekStartOf(ctx context.Context, tx pgx.Tx, now time.Time) (time.Time, error) {
	day, _, err := briefs.LocalDayAt(ctx, tx, now)
	if err != nil {
		return time.Time{}, err
	}
	// Go's Weekday has Sunday at 0; the offset back to Monday is 6 for Sunday
	// and weekday-1 otherwise.
	back := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -back), nil
}

// AssembleFor measures the week that just closed for the acting rep and writes
// it, unless they already have one for that week.
//
// created=false means the week already had a review. That is the constraint
// doing its job rather than a failure: the dispatcher ticks more than once
// inside a week so that a worker which was down still backfills.
func (e *Engine) AssembleFor(ctx context.Context, now time.Time) (Review, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return Review{}, false, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return Review{}, false, err
	}

	var review Review
	var created bool
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		// The week under review is the one that just CLOSED, not the one in
		// progress: a retrospective of a week still being lived would be
		// rewritten every day it ran.
		thisWeek, err := WeekStartOf(ctx, tx, now)
		if err != nil {
			return err
		}
		weekStart := thisWeek.AddDate(0, 0, -7)

		// The window is the LOCAL week, resolved to real instants.
		//
		// WeekStartOf returns a calendar date carried as midnight UTC — the
		// same shape brief_run.local_day uses, which is right for a date
		// column and wrong for a range. Comparing timestamptz against it
		// measures a week offset by the installation's UTC offset, and in a
		// DST zone a fixed 168 hours rather than the week people lived.
		start, end, err := localWeekWindow(ctx, tx, weekStart)
		if err != nil {
			return err
		}

		review = Review{UserID: userID, LocalWeekStart: weekStart, AsOf: now.UTC()}
		if review.Counts, err = countWeek(ctx, tx, userID, start, end); err != nil {
			return err
		}
		// Leads and meetings read separately from the tallies above: different
		// tables, different scope clauses, and each dated by a rule that takes
		// a paragraph to justify. They fold onto the same Counts because a
		// reader of a week wants one set of figures.
		c := &review.Counts
		if c.LeadsRouted, c.LeadsAnsweredInTarget, c.LeadsBreached, err =
			countWeekLeads(ctx, tx, userID, start, end); err != nil {
			return err
		}
		if c.MeetingsHeld, c.MeetingsWithNextStep, err =
			countWeekMeetings(ctx, tx, userID, start, end); err != nil {
			return err
		}
		if review.Money, err = countWeekMoney(ctx, tx, userID, start, end); err != nil {
			return err
		}
		if review.Deals, err = readWeekDeals(ctx, tx, userID, start, end); err != nil {
			return err
		}
		// The week this one is measured against: the rep's most recent EARLIER
		// review, whenever it was.
		//
		// Their previous review rather than "last week" by arithmetic. A rep
		// with a gap — a leave, a worker outage — has a prior week that is not
		// seven days back, and looking for one would find nothing and report
		// every count as new.
		if review.PriorReviewID, err = priorReview(ctx, tx, userID, weekStart); err != nil {
			return err
		}

		id, wrote, err := insertReview(ctx, tx, review)
		if err != nil {
			return err
		}
		created = wrote
		if !wrote {
			// Somebody already wrote this week. Read theirs rather than
			// reporting a failure — the rep gets one review either way.
			return nil
		}
		review.ID = id
		return insertDealLines(ctx, tx, id, review.Deals)
	})
	if err != nil {
		return Review{}, false, err
	}
	if !created {
		existing, err := e.LatestReview(ctx, &review.LocalWeekStart)
		return existing, false, err
	}
	return review, true, nil
}

// localWeekWindow turns a local week's calendar start into the two instants
// that bound it, in the installation's own zone.
//
// It asks Postgres rather than doing the arithmetic in Go so the zone lookup
// and the conversion are one answer: a DST week is 167 or 169 hours, and
// adding 7*24h to a start would measure an hour of the wrong week twice a year.
func localWeekWindow(ctx context.Context, tx pgx.Tx, weekStart time.Time) (time.Time, time.Time, error) {
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	var start, end time.Time
	if err := tx.QueryRow(ctx, `
		SELECT ($1::date)::timestamp AT TIME ZONE $2,
		       ($1::date + 7)::timestamp AT TIME ZONE $2`, weekStart, zone).
		Scan(&start, &end); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("weekly: bounding the local week: %w", err)
	}
	return start, end, nil
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
	if dealScope == "" {
		dealScope = "true"
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
		scope = "true"
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
// Attributed by captured_by, because a meeting has no owner: the
// activity_task_fields CHECK reserves assignee_id for tasks, so filtering
// meetings by it would match nothing and report every rep as having held none.
//
// "Left a next step" is a task raised AFTER the meeting against a record the
// meeting was also filed under — through the shared record rather than a direct
// pointer, because there is no meeting_id on a task.
func countWeekMeetings(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (held, withNextStep int, err error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos := arg(start), arg(end)
	capturedPos := arg("human:" + userID.String())
	scope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return 0, 0, err
	}
	const heldByRep = `m.kind = 'meeting' AND m.archived_at IS NULL
		      AND m.meeting_status = 'held' AND m.captured_by = $%[3]d
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
		            AND tl.organization_id IS NOT DISTINCT FROM ml.organization_id
		            AND tl.deal_id IS NOT DISTINCT FROM ml.deal_id
		            AND tl.lead_id IS NOT DISTINCT FROM ml.lead_id
		            AND tl.project_id IS NOT DISTINCT FROM ml.project_id
		          JOIN activity task ON task.id = tl.activity_id
		         WHERE ml.activity_id = m.id AND task.kind = 'task'
		           AND task.archived_at IS NULL
		           AND task.created_at >= m.occurred_at))`,
		startPos, endPos, capturedPos, scope), args...).
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
		dealScope = "true"
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
