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

// Unreadable replies still settle a conversation: displaying their content and
// preventing a duplicate reply are separate decisions. The queue uses this too.
func unansweredConversationSQL(asOf string) string {
	return `NOT EXISTS (SELECT 1 FROM activity later
     WHERE later.thread_key = a.thread_key AND later.kind = a.kind
       AND later.channel_provider IS NOT DISTINCT FROM a.channel_provider
       AND later.direction = 'outbound' AND later.archived_at IS NULL
       AND later.occurred_at <= ` + asOf + `
       AND (later.occurred_at, later.id) > (a.occurred_at, a.id))`
}

// EmailRowState keeps obligation and delivery facts in one page-batched read.
type EmailRowState struct {
	Delivery DeliveryState
	Move     crmcontracts.EmailSummaryMove
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
 coalesce(a.direction = 'inbound' AND a.owed_verdict = 'asks_us'
 AND a.capture_label = 'commitment' AND `+unansweredConversationSQL("now()")+`
 AND NOT EXISTS (SELECT 1 FROM activity task WHERE task.source_system = '`+EmailRequestTaskSource+`'
 AND task.source_activity_id = a.id AND task.is_done), false)
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
			&state.Delivery.BouncedAt, &state.Delivery.BounceReason, &state.Delivery.Files, &owed); err != nil {
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
