// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Delivery proves that we replied, not that we fulfilled a request. Confirmed
// requests survive acknowledgements until a linked task is completed or a human
// dismisses the conversation. Unclassified mail keeps the conversation fallback.
func unansweredConversationSQL(asOf string) string {
	return unansweredConversation(asOf, false)
}

// unansweredConversationAdmittingThreadless is the same question asked by the
// WAITING QUEUE, which additionally admits a message carrying no thread key.
//
// The two differ because their callers differ in what stands between the
// predicate and a reader. The queue applies the machine-sender and colleague
// rules above its own scan cap, so a threadless notification is dropped before
// it can reach anybody. The deal card applies neither: dealstatus/move.go takes
// the first email in the list and offers it as the reply somebody owes. Passing
// the wider form there turned a hand-logged note, and any threadless noreply
// mail linked to a deal, into a standing obligation on that deal.
//
// The narrow form is therefore the default and this one is asked for by name,
// so a new caller gets the safe reading unless it has said otherwise.
//
// Held by TestOnlyTheWaitingFamilyAdmitsThreadlessMail.
func unansweredConversationAdmittingThreadless(asOf string) string {
	return unansweredConversation(asOf, true)
}

// unansweredConversation spells the question once for both readings.
//
// A threadless message is admitted by NO reply evidence rather than by loose
// evidence: the anti-join below compares thread keys with plain equality, which
// a NULL never satisfies, so the row finds no reply and stays waiting. Matching
// them with IS NOT DISTINCT FROM instead would join every NULL to every other,
// and one unthreaded outbound would silence every unthreaded question in the
// workspace.
func unansweredConversation(asOf string, admitThreadless bool) string {
	threadless := ""
	if admitThreadless {
		// The evidence the narrow form asks of such a row is evidence the
		// waiting queue PRODUCES: the owed-verdict pass reads its backlog from
		// that queue, so a row excluded there is never judged, never gains a
		// verdict, and is excluded again on the next pass.
		threadless = ` OR a.thread_key IS NULL`
	}
	return `(` + requestCandidateSQL + threadless + ` OR (a.thread_key IS NOT NULL AND NOT EXISTS (SELECT 1 FROM activity later
     WHERE later.thread_key = a.thread_key AND later.kind = a.kind
       AND later.channel_provider IS NOT DISTINCT FROM a.channel_provider
       AND later.direction = 'outbound' AND later.archived_at IS NULL
       AND later.occurred_at <= ` + asOf + `
       AND (later.occurred_at, later.id) > (a.occurred_at, a.id))))`
}

// EmailRowState keeps obligation and delivery facts in one page-batched read.
type EmailRowState struct {
	RequestHasReminder bool
	Delivery           DeliveryState
	Move               crmcontracts.EmailSummaryMove
}

// EmailStatesFor reads confirmed obligations and delivery after the caller's
// content gate. A captured reminder's completion settles its source request.
func EmailStatesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]EmailRowState, error) {
	if len(activityIDs) == 0 {
		return map[ids.UUID]EmailRowState{}, nil
	}
	args := []any{activityIDs}
	wanted := fmt.Sprintf("$%d", len(args))
	rows, err := tx.Query(ctx, `SELECT a.id, coalesce(delivery.status, ''), delivery.reason,
 delivery.sent_at, delivery.bounced_at, delivery.bounce_reason, coalesce(delivery.attachments, '[]'::jsonb),
 coalesce(`+outstandingRequestSQL+`, false), `+openRequestReminderSQL+`
 FROM activity a LEFT JOIN comms_outbound delivery ON delivery.activity_id = a.id
 WHERE a.id = ANY(`+wanted+`) AND a.restricted_at IS NULL`, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading email states: %w", err)
	}
	defer rows.Close()
	out := make(map[ids.UUID]EmailRowState, len(activityIDs))
	for rows.Next() {
		var id ids.UUID
		var state EmailRowState
		var owed bool
		if err := rows.Scan(&id, &state.Delivery.Status, &state.Delivery.Reason, &state.Delivery.SentAt,
			&state.Delivery.BouncedAt, &state.Delivery.BounceReason, &state.Delivery.Files, &owed, &state.RequestHasReminder); err != nil {
			return nil, err
		}
		state.Move = crmcontracts.EmailSummaryMoveNone
		if owed {
			state.Move = crmcontracts.EmailSummaryMoveNeedsReply
		}
		out[id] = state
	}
	return out, rows.Err()
}
