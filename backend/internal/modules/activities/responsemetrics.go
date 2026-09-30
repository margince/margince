// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// How fast the workspace answers, and how much of the queue is being worked
// rather than put down.
//
// The hidden-backlog guardrail says what the queue is NOT showing. These two say
// what happens to the work it does show, which is the other half of the same
// question: a queue can be honest about its contents and still be a queue nobody
// answers.
//
// Both are read over a WINDOW rather than at an instant, because neither is a
// fact about right now — "we answer in four hours" is a claim about a fortnight,
// and a figure taken from today would swing on one slow afternoon.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ResponseMetrics is what the workspace did with its waiting work over a window.
type ResponseMetrics struct {
	// Answered is how many inbound sales messages got a reply in the window.
	Answered int
	// MedianMinutes is how long the middle one waited, in minutes. The MEDIAN
	// rather than the mean, for the reason the material bar takes one: a single
	// message answered after three weeks drags an average past every figure a
	// reader would recognise, and the question here is "what does a customer
	// typically wait", not "what is the arithmetic mean of our failures".
	//
	// Zero when nothing was answered, which the Answered count tells apart from
	// a genuine zero-minute median.
	MedianMinutes int
	// Disposed is how many rows a reader put DOWN in the window — snoozed,
	// marked not theirs, or judged not sales. Over the conversations the CALLER
	// may open, like every figure beside it.
	Disposed int
	// DisposedNotSales is how many of those were the workspace-wide judgement.
	// Its own figure because it is the one that costs everybody: the other two
	// hide a row from one reader, this one hides the conversation from all of
	// them and does not lift.
	DisposedNotSales int
}

// firstResponseSQL measures the wait on messages that WERE answered.
//
// The answer is the first one firstAnswerAtSQL finds: our reply on the thread,
// our same-subject mail to the sender, or a call or held meeting with them. It
// is the same evidence that takes a message off the waiting lane, so the two
// together cover every sales message: answered, and not yet.
//
// The window is bounded by the caller. An unbounded read would answer "how fast
// have we ever been", which no reader is asking and which grows without limit.
//
// The PERCENTILE is the caller's too, and there are exactly two askers. The
// metrics window takes the median, which is what "how fast do we answer" means
// to a reader reading a number. The waiting horizon takes a high one, because
// its question is the opposite end: the point past which this installation
// essentially does not answer at all (waitinghorizon.go).
var firstResponseSQL = `
	SELECT count(*),
	       COALESCE(
	         percentile_cont(%[5]s) WITHIN GROUP (
	           ORDER BY EXTRACT(EPOCH FROM (reply.occurred_at - inbound.occurred_at)) / 60
	         )::bigint, 0)
	  FROM activity inbound
	  -- The FIRST answer after it, not the newest: what a customer waited is the
	  -- time to the answer they actually got. Unbounded above, because an answer
	  -- that came after the window closed still ends a wait the window opened.
	  CROSS JOIN LATERAL (
	         SELECT ` + firstAnswerAtSQL("inbound", "'infinity'::timestamptz") + ` AS occurred_at
	       ) reply
	 WHERE inbound.kind IN ('email', 'message')
	   AND inbound.direction = 'inbound'
	   AND inbound.archived_at IS NULL
	   AND reply.occurred_at IS NOT NULL
	   AND inbound.occurred_at >= $1
	   AND inbound.occurred_at < $2
	   AND %[1]s
	   -- A COLLEAGUE's message is not a customer waiting, and this reading
	   -- answers "how fast do we answer customers". Without the same exclusion
	   -- the queue applies, an internal thread nobody replies to would drag the
	   -- rate down over work the queue never showed anybody.
	   AND NOT %[4]s
	   -- A SALES thread, by the same four arms waitingRepliesSQL qualifies on.
	   -- Restated rather than shared because this query has no wl join to hang
	   -- them off; what must not drift is the DEFINITION, and a test feeding
	   -- both readers one timeline holds that better than a shared fragment
	   -- neither could read.
	   AND EXISTS (
	         SELECT 1 FROM activity_link sales
	          WHERE sales.activity_id = inbound.id
	            AND (sales.contact_id IS NOT NULL
	              OR sales.company_id IS NOT NULL
	              OR EXISTS (SELECT 1 FROM deal d
	                          WHERE d.id = sales.deal_id AND %[2]s)
	              OR EXISTS (SELECT 1 FROM lead ld
	                          WHERE ld.id = sales.lead_id AND %[3]s)))`

// dispositionsSQL counts what readers put down in the window.
//
// From audit_log rather than from the state tables, and that is the whole point
// of reading it here: activity_reader_state holds what is set aside NOW, so a
// snooze that lifted and a not_mine somebody withdrew have left no trace in it.
// The audit row is the only record that the judgement was ever made, which is
// what a rate over a window needs.
//
// The states are NAMED, and the earlier `IS NOT NULL` is the reason. Four verbs
// write this column and two of them are UNDO — `picked_up` takes back a
// not_mine, `sales_again` takes back a not_sales — so counting every non-null
// value scored a rep who set a row aside and then thought better of it at TWO
// dispositions rather than none. The figure ran backwards for exactly the
// behaviour it should reward, and read as more judgement the more of it was
// withdrawn.
//
// Counted under the CALLER's own visibility, like the median beside it. The
// audit row names the activity it judged, so the count joins to that activity
// and applies the same content clause: a judgement made on a conversation this
// reader may not open contributes to no figure here. Without the join the two
// halves of one response disagreed about whose workspace they described — the
// median spoke for what the caller can see and these two for everybody — while
// the endpoint's own prose promised caller visibility for all four.
//
// An INNER join, so an audit row whose activity is gone counts for nobody. A
// deleted conversation is not a judgement anyone can still check, and admitting
// it would be the one direction this figure must not fail in: reporting work
// under a reader who cannot reach it. Production does not delete activity rows
// — erasure anonymizes them in place — so this drops nothing a live workspace
// holds; it is the honest answer for the case where a row IS gone.
//
// ARCHIVED activities are kept, and that is the one place this query and the
// median deliberately differ. The median excludes them because an archived
// thread is not a wait anybody is still serving. A judgement is a different
// kind of fact: the reader made it, in the window, and archiving the thread
// afterwards does not unmake it. Counting only unarchived ones would make the
// figure fall as a workspace tidied up — the same defect reading from
// activity_reader_state had, taking a different route.
// Every placeholder is derived from the argument slice rather than typed, which
// the rulebook asks of any statement whose arguments are built beside it.
// Nothing checks that a hand-typed $N still names the value a caller appends,
// and the content clause below numbers itself from wherever this query's own
// arguments end — so a literal here would be a second place to keep in step.
const dispositionsSQL = `
	SELECT count(*) FILTER (WHERE a.after->>'disposition' = ANY($%[3]d)),
	       count(*) FILTER (WHERE a.after->>'disposition' = $%[4]d)
	  FROM audit_log a
	  JOIN activity act ON act.id = a.entity_id
	 WHERE a.entity_type = 'activity'
	   AND a.action = 'update'
	   AND a.occurred_at >= $%[1]d
	   AND a.occurred_at < $%[2]d
	   AND %[5]s`

// puttingDown is the set of verbs that PUT a row down, as against the two that
// pick one back up.
//
// A function rather than a package var so no caller can append to the answer
// another caller is about to read — the shape every enumerated set in this tree
// takes. `stateNotSales` is spelled beside its siblings here rather than only
// inside the writer, because a figure counting a state nothing writes is a
// silent zero and reads exactly like a quiet fortnight.
func puttingDown() []string {
	return []string{stateSnoozed, stateNotMine, stateNotSales}
}

// ResponseWindow reads both figures over one window.
//
// The window is half-open — from inclusive, to exclusive — so consecutive
// windows partition time and a message on a boundary is counted once.
func (s *Store) ResponseWindow(ctx context.Context, from, to time.Time) (ResponseMetrics, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ResponseMetrics{}, err
	}
	if !to.After(from) {
		// A window that ends before it starts is a caller mistake, and answering
		// zeros would let it read as a quiet fortnight.
		return ResponseMetrics{}, fmt.Errorf(
			"activities: response window ends at %s, before it starts at %s",
			to.Format(time.RFC3339), from.Format(time.RFC3339))
	}
	var out ResponseMetrics
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{from, to}
		arg := func(v any) int { args = append(args, v); return len(args) }
		// The same content gate the waiting lane composes. A message this reader
		// may not read contributes to no figure here: a median over rows they
		// cannot open would publish the timing of somebody else's conversations.
		// The INBOUND side only, and the asymmetry is deliberate rather than an
		// oversight. The waiting lane ignores the audience arm on its own reply
		// anti-join for a stated reason — a reply this reader may not see still
		// answered the customer — and gating the reply here would make the two
		// readers disagree about which threads were answered.
		//
		// What it costs is stated in the contract rather than hidden: a caller
		// who can read an inbound but not the limited reply to it learns when a
		// colleague answered, folded into the median. A timestamp, not content.
		content, err := auth.ActivityContentClause(ctx, "inbound", arg)
		if err != nil {
			return err
		}
		// The SAME colleague-domain snapshot the queue judges by, read in this
		// transaction. Two readings of one rule taken from two snapshots would
		// disagree the moment an administrator added a domain between them.
		ownDomains, err := s.ownDomainList(ctx, tx)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, fmt.Sprintf(firstResponseSQL,
			content,
			liveRecord(openDealPredicate, "d"),
			liveRecord(workingLeadPredicate, "ld"),
			ownDomainSenderSQL("inbound", arg(ownDomains)),
			medianPercentile),
			args...).Scan(&out.Answered, &out.MedianMinutes); err != nil {
			return fmt.Errorf("activities: reading first-response times: %w", err)
		}
		// A second statement, so a second argument list. Each value takes the
		// position `putArg` gives it and the statement is rendered against those
		// positions, so nothing here depends on how many arguments the median
		// above happened to take.
		var putArgs []any
		putArg := func(v any) int { putArgs = append(putArgs, v); return len(putArgs) }
		windowFrom, windowTo := putArg(from), putArg(to)
		down, wholeWorkspace := putArg(puttingDown()), putArg(stateNotSales)
		// The same gate the median above carries, on the activity the audit row
		// names. A judgement made on a conversation this reader may not open
		// counts for nobody here.
		putContent, err := auth.ActivityContentClause(ctx, "act", putArg)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, fmt.Sprintf(dispositionsSQL,
			windowFrom, windowTo, down, wholeWorkspace, putContent), putArgs...).
			Scan(&out.Disposed, &out.DisposedNotSales); err != nil {
			return fmt.Errorf("activities: reading disposition counts: %w", err)
		}
		return nil
	})
	if err != nil {
		return ResponseMetrics{}, err
	}
	return out, nil
}
