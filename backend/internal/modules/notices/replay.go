// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A replay returns the original record, including its text and target.
// A new delivery payload cannot change an already delivered notice.
func storedNotice(ctx context.Context, tx pgx.Tx, recipient ids.UserID, dedupeKey string) (Notice, error) {
	var stored Notice
	var storedType *string
	var storedID *ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id, kind, subject, body, target_type, target_id, created_at, origin, system_text FROM notice
		 WHERE recipient_user_id = $1 AND dedupe_key = $2`,
		recipient, dedupeKey,
	).Scan(&stored.ID, &stored.Kind, &stored.Subject, &stored.Body,
		&storedType, &storedID, &stored.CreatedAt, &stored.Origin, &stored.SystemText); err != nil {
		return Notice{}, fmt.Errorf("notices: reading the notice that already stands: %w", err)
	}
	if storedType != nil && storedID != nil {
		stored.Target = Target{Type: *storedType, ID: *storedID}
	}
	stored.Target = resolvedNoticeTarget(stored.Target, stored.Kind, stored.SystemText)
	stored.SystemDisplay = noticeSystemDisplay(ctx, stored.SystemText, stored.Subject, stored.Body)
	return stored, nil
}

// Read and delivery paths exclude the recipient's own stage moves.
const stageMoveNotice = `kind = 'automation' AND target_type = 'deal'
 AND (origin ? 'stage_change' OR starts_with(dedupe_key, 'stage_change_notify:'))`

const notTheReadersOwnStageMove = `NOT coalesce(` + stageMoveNotice + `
	AND origin->>'actor_type' = 'human'
	AND lower(origin->>'actor_id') IN
	  (recipient_user_id::text, 'human:' || recipient_user_id::text), false)`
