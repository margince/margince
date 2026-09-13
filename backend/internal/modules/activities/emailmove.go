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

// EmailMovesFor reads confirmed obligations after the caller's content gate.
// Completing a captured reminder settles it; direction alone proves no request.
func EmailMovesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]crmcontracts.EmailSummaryMove, error) {
	args := []any{activityIDs}
	wanted := fmt.Sprintf("$%d", len(args))
	rows, err := tx.Query(ctx, `SELECT a.id,
   a.direction = 'inbound' AND a.owed_verdict = 'asks_us'
   AND a.capture_label = 'commitment'
   AND `+unansweredConversationSQL("now()")+`
   AND NOT EXISTS (SELECT 1 FROM activity task WHERE task.source_system = 'email_request'
     AND task.source_activity_id = a.id AND task.is_done)
   FROM activity a WHERE a.id = ANY(`+wanted+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading email obligations: %w", err)
	}
	defer rows.Close()
	out := make(map[ids.UUID]crmcontracts.EmailSummaryMove, len(activityIDs))
	for rows.Next() {
		var id ids.UUID
		var owed *bool
		if err := rows.Scan(&id, &owed); err != nil {
			return nil, err
		}
		out[id] = crmcontracts.EmailSummaryMoveNone
		if owed != nil && *owed {
			out[id] = crmcontracts.EmailSummaryMoveNeedsReply
		}
	}
	return out, rows.Err()
}
