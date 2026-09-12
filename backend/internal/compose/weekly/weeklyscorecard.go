// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// How WELL the week went, as against what happened in it.
//
// The counts beside this say a rep was sent 40 leads and held 6 meetings. This
// says they answered 12 inside the target and 4 of those meetings left a next
// step behind. The two are different questions and a reader wants both, but
// only this one is a judgement, so only this one has to be careful about what
// it does when there is nothing to judge.
//
// A BLOCK IS PRESENT OR ABSENT, NEVER ZEROED. A rep who carried no leads at all
// did not score zero on the funnel — the funnel was not their work that week,
// and a row of zeros reads as failure at something nobody asked of them. Each
// block reports whether it applies, and the panel draws only what applies.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Scorecard is one week's judgement of one rep's work.
//
// Two independent blocks. Either may be absent, and absent is a different fact
// from every count being zero: no leads at all versus forty leads and nothing
// done with them.
type Scorecard struct {
	Lead *LeadBlock
	Deal *DealBlock
}

// LeadBlock is the funnel slice: how the rep's leads moved, and whether the
// meetings behind them happened.
type LeadBlock struct {
	// Movement, counted from the audit images rather than the lead row. The row
	// carries the CURRENT status and a few milestone stamps, so it cannot say
	// who moved from contacted to engaged inside a window.
	Advanced     int
	Disqualified int
	Promoted     int

	// The SLA slice, carried here as well as on Counts so a reader of the
	// scorecard has the whole funnel in one record.
	AnsweredInTarget int
	Breached         int

	// Meetings by what they BECAME this week. Never by activity.meeting_status:
	// a meeting booked Monday and held Friday reads `held` today, so counting
	// the column reports no bookings for the week it was booked in.
	Booked int
	Held   int
	NoShow int
	// Meetings whose only history row was invented by the backfill from current
	// state. They cannot say when they were booked, so they are reported as
	// partial coverage rather than counted as history.
	PartialHistory int
}

// DealBlock is the pipeline slice: whether deals moved forward, and whether
// they are in a state anybody could work.
type DealBlock struct {
	Advances    int
	Regressions int

	// Median whole days a deal sat in the stage it left this week. Absent when
	// no deal changed stage: a median of nothing is not zero.
	MedianDaysInStage *int

	// Counts and their denominator, never a rate. A reader who sees 3 of 5 can
	// judge the number; one who sees 60% cannot tell it from 300 of 500.
	WithNextStep   int
	Open           int
	MultiThreaded  int
	CloseDateSound int

	// Category moves, each deal counted ONCE however many times it was edited.
	ForecastUp   int
	ForecastDown int

	// Deals whose state at the closing instant could not be reconstructed,
	// because the audit images describing their week sit behind an erasure.
	//
	// Reported rather than absorbed. These deals are absent from every population
	// count above, so a reader who is not told would take a smaller Open than the
	// rep really had for the whole truth. Nonzero means the counts beside it are a
	// floor, and the panel says so.
	//
	// A POINTER, because nil is a third fact and not a zero. A scorecard frozen
	// before this reconstruction existed cannot say what it failed to rebuild —
	// its counts are the current-state figures they always were — and folding
	// that to 0 would claim those old weeks were fully reconstructed.
	Unreconstructible *int
}

// multiThreadWindow is how far back a deal's second contact may be and still
// count as coverage.
//
// Thirty days rather than the review's own week: a deal worked steadily for a
// month is multi-threaded whether or not the second contact happened to appear
// in the seven days under review, and a week-long window would report a rep as
// single-threaded for the ordinary reason that they spoke to one of the two
// this week.
const multiThreadWindow = 30 * 24 * time.Hour

// minStakeholders is how many distinct contacts a deal needs to count as
// multi-threaded. Two, because the risk being measured is the single point of
// failure: one contact who leaves takes the deal with them.
const minStakeholders = 2

// scoreWeek reads both blocks for one rep's closed week.
//
// Both are read even when one turns out absent, because "absent" is itself a
// finding the row records, and deciding not to ask would leave the reader
// unable to tell an empty funnel from an unmeasured one.
func scoreWeek(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (Scorecard, error) {
	lead, err := scoreLeads(ctx, tx, userID, start, end)
	if err != nil {
		return Scorecard{}, err
	}
	deal, err := scoreDeals(ctx, tx, userID, start, end)
	if err != nil {
		return Scorecard{}, err
	}
	return Scorecard{Lead: lead, Deal: deal}, nil
}

// ladderRungSQL ranks an open lead status by its position on the ladder, so a
// transition has a DIRECTION.
//
// The rungs are the generated contract's own constants, so a status renamed in
// crm.yaml stops compiling here rather than silently ranking -1. Their ORDER is
// this list — the contract's enum is alphabetical and says nothing about
// direction — and it is the SQL mirror of contacts.LeadStatus.Advances, which is
// the Go spelling of the same rule.
//
// A terminal or unknown status ranks -1 and so is never a step UP, which is
// right: promoted and disqualified LEAVE the ladder rather than climbing it,
// and each already has its own count.
func ladderRungSQL(column string) string {
	ladder := []crmcontracts.LeadStatus{
		crmcontracts.LeadStatusNew,
		crmcontracts.LeadStatusContacted,
		crmcontracts.LeadStatusEngaged,
	}
	var b strings.Builder
	b.WriteString("CASE " + column)
	for rung, status := range ladder {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", status, rung)
	}
	b.WriteString(" ELSE -1 END")
	return b.String()
}

// climbedTheLadderSQL is the predicate for a step UP: both ends on the ladder,
// and the destination above the origin. The Go spelling of this rule is
// contacts.LeadStatus.Advances, and the two must agree — a gate below holds it.
func climbedTheLadderSQL() string {
	from, to := ladderRungSQL("from_status"), ladderRungSQL("to_status")
	return "(" + from + ") >= 0 AND (" + to + ") > (" + from + ")"
}

// scoreLeads reads the funnel block, or reports it absent.
//
// ABSENT means the rep carried no lead in the window at all — not that nothing
// moved. A rep with forty untouched leads gets a present block full of zeros,
// which is a real and unflattering finding; a rep with no leads gets no block.
//
// The movement counts come from audit_log's before/after images. That is where
// a lead's movement lives: the lead row carries its current status and a few
// milestone stamps (routed_at, first_response_at, promoted_at) and cannot
// answer "who moved from contacted to engaged this week". All five writers of
// lead.status — promote, demote, disqualify, the SLA ladder's AdvanceLeadStatus
// and the ordinary PATCH — go through storekit.Audit, so the images are
// complete; a sixth writer that bypassed it would make this undercount, which
// is why the audit gate refuses one.
//
// Profiled before it was written, per this work package's own delivery gate: at
// 10M audit rows with 333k lead updates over a year, one week for every owner
// costs 277ms cold and 67ms warm — 5% of database.CallerPredicateBudget, on a
// path no user waits on. The index that serves it is
// idx_audit_entity_narrow (entity_type, entity_id, occurred_at): entity_type is
// an equality on the leading column, so occurred_at is usable as a range even
// with entity_id skipped between them, and the millions of rows belonging to
// other entity types are never visited.
func scoreLeads(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (*LeadBlock, error) {
	// Presence is asked FIRST and with its own argument list. The block query
	// below binds the window as well, and a statement that binds a parameter it
	// never mentions is rejected outright — so the two cannot share one list.
	present, err := hasLeads(ctx, tx, userID, end)
	if err != nil {
		return nil, err
	}
	if !present {
		//nolint:nilnil // an absent block IS the answer for a rep who carried no leads, not a missing one: zeros here would read as failure at work nobody asked of them.
		return nil, nil
	}

	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos := arg(start), arg(end), arg(userID)
	// The RECORDER, used only where a meeting names no host — the same
	// fallback countWeekMeetings applies. captured_by is a principal STRING
	// rather than a user id, which is why this is not simply userID: comparing
	// the text column to a uuid is a type error, and comparing it to the bare
	// uuid text would never match.
	capturedPos := arg("human:" + userID.String())
	scope, err := auth.ScopeClauseFor(ctx, "lead", "l", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlUnbounded
	}
	// Activities carry no owner, so their visibility comes from the records
	// they link to and the audience the writer set — never hand-rolled.
	meetingScope, err := auth.ActivityContentClause(ctx, "m", arg)
	if err != nil {
		return nil, err
	}
	// The rep's leads, scoped once and reused by every count below. Membership
	// is "owned by this rep and visible to them", with no window on it: a lead
	// routed last month that moved this week is this week's movement.
	mine := fmt.Sprintf(`SELECT l.id FROM lead l
		 WHERE l.owner_id = $%[1]d AND (%[2]s)`, userPos, scope)

	block := &LeadBlock{}
	// One statement for the whole block: eight counts read at eight moments
	// would describe eight slightly different weeks.
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		WITH mine AS (%[5]s),
		-- Every status transition on those leads inside the window, one row per
		-- audit entry. DISTINCT FROM rather than <>: a NULL on either side is a
		-- change, and <> would silently drop the first transition of a lead
		-- whose status was NULL before it.
		moved AS (
		  SELECT a.entity_id,
		         a.before->>'status' AS from_status,
		         a.after->>'status'  AS to_status
		    FROM audit_log a
		    JOIN mine ON mine.id = a.entity_id
		   WHERE a.entity_type = 'lead'
		     AND a.occurred_at >= $%[1]d AND a.occurred_at < $%[2]d
		     AND a.before->>'status' IS DISTINCT FROM a.after->>'status'),
		-- Meetings the rep's leads became something this week. Read from the
		-- history table, so a meeting booked Monday and held Friday counts in
		-- BOTH the booked and the held tally, which is what a funnel means.
		met AS (
		  SELECT h.status, h.partial_pre_history
		    FROM activity_meeting_history h
		    JOIN activity m ON m.id = h.activity_id
		   WHERE m.kind = 'meeting'
		     AND `+meetingIsTheirsSQL("$%[3]d", "$%[6]d")+`
		     AND m.archived_at IS NULL
		     AND h.effective_at >= $%[1]d AND h.effective_at < $%[2]d
		     AND (%[7]s))
		SELECT
		  (SELECT count(*) FROM moved WHERE %[8]s),
		  (SELECT count(*) FROM moved WHERE to_status = 'disqualified'),
		  (SELECT count(*) FROM moved WHERE to_status = 'promoted'),
		  (SELECT count(*) FROM lead l JOIN mine ON mine.id = l.id
		    WHERE COALESCE(l.routed_at, l.created_at) >= $%[1]d
		      AND COALESCE(l.routed_at, l.created_at) < $%[2]d
		      AND l.first_response_at IS NOT NULL AND l.sla_breached_at IS NULL),
		  (SELECT count(*) FROM lead l JOIN mine ON mine.id = l.id
		    WHERE COALESCE(l.routed_at, l.created_at) >= $%[1]d
		      AND COALESCE(l.routed_at, l.created_at) < $%[2]d
		      AND l.sla_breached_at IS NOT NULL),
		  (SELECT count(*) FROM met WHERE status = 'booked' AND NOT partial_pre_history),
		  (SELECT count(*) FROM met WHERE status = 'held' AND NOT partial_pre_history),
		  (SELECT count(*) FROM met WHERE status = 'no_show' AND NOT partial_pre_history),
		  (SELECT count(*) FROM met WHERE partial_pre_history)`,
		startPos, endPos, userPos, scope, mine, capturedPos, meetingScope,
		climbedTheLadderSQL()), args...).
		Scan(&block.Advanced, &block.Disqualified, &block.Promoted,
			&block.AnsweredInTarget, &block.Breached,
			&block.Booked, &block.Held, &block.NoShow, &block.PartialHistory)
	if err != nil {
		return nil, fmt.Errorf("weekly: scoring the week's funnel: %w", err)
	}
	return block, nil
}

// hasLeads reports whether this rep carries any lead they can see.
//
// Bounded at the END of the week and open at the start: a rep whose leads all
// arrived last month still has a funnel this week, so the question is "did they
// carry one by then" rather than "did one arrive in these seven days" — which
// would make a quiet week's block vanish. The upper bound is what stops a lead
// created AFTER the week from conjuring a block onto it.
//
// Its own scope clause and its own argument list, because it binds only this
// bound and a statement may not bind a parameter it does not mention.
func hasLeads(ctx context.Context, tx pgx.Tx, userID ids.UUID, by time.Time) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	userPos, byPos := arg(userID), arg(by)
	scope, err := auth.ScopeClauseFor(ctx, "lead", "l", arg)
	if err != nil {
		return false, err
	}
	if scope == "" {
		scope = sqlUnbounded
	}
	var present bool
	err = tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM lead l
		   WHERE l.owner_id = $%[1]d AND l.created_at < $%[2]d AND (%[3]s))`,
		userPos, byPos, scope), args...).Scan(&present)
	if err != nil {
		return false, fmt.Errorf("weekly: looking for the rep's leads: %w", err)
	}
	return present, nil
}

// forecastRankSQL orders the forecast categories so a move between two of them
// has a direction.
//
// The vocabulary is deal_forecast_category_check's, and the order is the
// confidence ladder the product already speaks: omitted is not in the number at
// all, pipeline is in it, best_case is likely and commit is promised. A
// category the CHECK does not allow, and a NULL, rank as -1 and so read as
// BELOW omitted — a deal that gained a category moved up, and one that lost it
// moved down, which is what happened.
//
// Spelled as SQL rather than compared in Go because the comparison happens
// inside the aggregate that reduces a deal's many edits to one move; pulling
// the rows out to rank them here would be a second pass over the same data to
// answer the same question.
func forecastRank(column string) string {
	return `CASE ` + column + `
		WHEN 'commit' THEN 3 WHEN 'best_case' THEN 2
		WHEN 'pipeline' THEN 1 WHEN 'omitted' THEN 0 ELSE -1 END`
}

// scoreDeals reads the pipeline block, or reports it absent.
//
// ABSENT means the rep had no open deal AND no stage change in the window —
// nothing to judge. A rep with open deals that did not move gets a present
// block of zeros, which is the finding.
//
// An advance is a move to a LATER stage of the same pipeline, a regression an
// earlier one, both read from stage.position. Position rather than
// win_probability: two stages may share a probability, and a move between them
// is still a move. A deal that crossed pipelines has no comparable position and
// counts as neither.
func scoreDeals(
	ctx context.Context, tx pgx.Tx, userID ids.UUID, start, end time.Time,
) (*DealBlock, error) {
	// The reporting zone, because expected_close_date is a DATE and the window
	// is an instant: cast without it and Postgres uses the session zone, which
	// puts a deal closing near midnight on the wrong side of the boundary.
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	startPos, endPos, userPos := arg(start), arg(end), arg(userID)
	sincePos := arg(end.Add(-multiThreadWindow))
	stakeholdersPos := arg(minStakeholders)
	zonePos := arg(zone)
	// The erasure boundary's vocabulary, bound once and read by the rewind.
	// privacy owns the list; a literal here would be a second copy of the one
	// rule where almost-the-same resurrects what was certified destroyed.
	verbsPos := arg(privacy.ScrubVerbs())
	scope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlUnbounded
	}

	block := &DealBlock{}
	var present bool
	// Nullable on its own: a present block whose deals all stayed put has no
	// median, and that is absent rather than zero days.
	var median *int
	err = tx.QueryRow(ctx, fmt.Sprintf(dealScoreSQL,
		startPos, endPos, userPos, sincePos, stakeholdersPos,
		forecastRank("last_cat"), forecastRank("first_cat"), scope, zonePos,
		weekEndDealsSQL(fmt.Sprintf("$%d", endPos), fmt.Sprintf("$%d", verbsPos))),
		args...).
		Scan(&present, &block.Advances, &block.Regressions, &median,
			&block.WithNextStep, &block.Open, &block.MultiThreaded,
			&block.CloseDateSound, &block.ForecastUp, &block.ForecastDown,
			&block.Unreconstructible)
	if err != nil {
		return nil, fmt.Errorf("weekly: scoring the week's deals: %w", err)
	}
	if !present {
		//nolint:nilnil // no open deal and no stage change means there is nothing to judge, which is a different fact from a block of zeros.
		return nil, nil
	}
	block.MedianDaysInStage = median
	return block, nil
}
