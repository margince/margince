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
	// marked not theirs, or judged not sales.
	Disposed int
	// DisposedNotSales is how many of those were the workspace-wide judgement.
	// Its own figure because it is the one that costs everybody: the other two
	// hide a row from one reader, this one hides the conversation from all of
	// them and does not lift.
	DisposedNotSales int
}

// firstResponseSQL measures the wait on threads that WERE answered.
//
// The mirror of waitingRepliesSQL, which finds the newest inbound with no later
// outbound. This finds the newest inbound that HAS one, and how long it took —
// so the two together cover every sales thread: answered, and not yet.
//
// Deliberately NOT sharing that constant. The anti-join is the whole of what
// this differs by, and forcing one statement to express both questions would
// mean a flag deciding whether a NOT EXISTS is an EXISTS — the shape where a
// reader can no longer tell what either caller runs. What the two DO share is
// the definition of a sales thread, and that is stated here as the same three
// clauses rather than inherited: see the comment on the sales link below.
//
// The window is bounded by the caller. An unbounded read would answer "how fast
// have we ever been", which no reader is asking and which grows without limit.
const firstResponseSQL = `
	SELECT count(*),
	       COALESCE(
	         percentile_cont(0.5) WITHIN GROUP (
	           ORDER BY EXTRACT(EPOCH FROM (reply.occurred_at - inbound.occurred_at)) / 60
	         )::bigint, 0)
	  FROM activity inbound
	  -- The FIRST reply after it, not the newest: what a customer waited is the
	  -- time to the answer they actually got, and taking the latest outbound on
	  -- the thread would report the wait as the length of the whole conversation.
	  JOIN LATERAL (
	         SELECT a.occurred_at
	           FROM activity a
	          WHERE a.thread_key = inbound.thread_key
	            AND a.direction = 'outbound'
	            AND a.archived_at IS NULL
	            AND a.occurred_at > inbound.occurred_at
	          ORDER BY a.occurred_at
	          LIMIT 1) reply ON TRUE
	 WHERE inbound.kind IN ('email', 'message')
	   AND inbound.direction = 'inbound'
	   AND inbound.archived_at IS NULL
	   AND inbound.thread_key IS NOT NULL
	   AND inbound.occurred_at >= $1
	   AND inbound.occurred_at < $2
	   AND %[1]s
	   -- A SALES thread, by the same three arms waitingRepliesSQL qualifies on.
	   -- Restated rather than shared because this query has no wl join to hang
	   -- them off; what must not drift is the DEFINITION, and a test feeding
	   -- both readers one timeline holds that better than a shared fragment
	   -- neither could read.
	   AND EXISTS (
	         SELECT 1 FROM activity_link sales
	          WHERE sales.activity_id = inbound.id
	            AND (sales.person_id IS NOT NULL
	              OR sales.organization_id IS NOT NULL
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
const dispositionsSQL = `
	SELECT count(*) FILTER (WHERE a.after->>'disposition' IS NOT NULL),
	       count(*) FILTER (WHERE a.after->>'disposition' = 'not_sales')
	  FROM audit_log a
	 WHERE a.entity_type = 'activity'
	   AND a.action = 'update'
	   AND a.occurred_at >= $1
	   AND a.occurred_at < $2`

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
		content, err := auth.ActivityContentClause(ctx, "inbound", arg)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, fmt.Sprintf(firstResponseSQL,
			content,
			liveRecord(openDealPredicate, "d"),
			liveRecord(workingLeadPredicate, "ld")),
			args...).Scan(&out.Answered, &out.MedianMinutes); err != nil {
			return fmt.Errorf("activities: reading first-response times: %w", err)
		}
		// The audit read carries no content clause, and that is deliberate: an
		// audit row records that a JUDGEMENT was made, not what the message
		// said, and the count is over the workspace's own bookkeeping. The
		// workspace binding is the transaction's.
		if err := tx.QueryRow(ctx, dispositionsSQL, from, to).
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
