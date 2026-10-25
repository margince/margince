// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SettledCitations answers whether each cited message's thread settled it by
// asOf, by the Worklist's rules. Their ask: a settled verdict, a done request
// task, or an answer when nobody confirmed the ask. Our promise: the follow-up
// the Worklist offers as "Done?". Other kinds never settle. An id the caller
// may not read is absent.
func SettledCitations(ctx context.Context, tx pgx.Tx, cited []ids.UUID, asOf time.Time) (map[ids.UUID]bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(cited))
	if len(cited) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	citedPos := arg(cited)
	until := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	keptReadable, err := auth.ActivityContentClause(ctx, "kept", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT a.id,
  CASE WHEN a.direction = 'inbound' AND a.kind IN ('email', 'message')
         THEN NOT (%[1]s AND ((%[2]s) OR NOT %[3]s))
       WHEN a.direction = 'outbound' AND a.kind = 'email'
         THEN EXISTS (SELECT 1 FROM activity kept
                WHERE %[4]s AND (%[5]s)
                  AND EXISTS (SELECT 1 FROM activity_participant promised
                       WHERE promised.activity_id = a.id AND promised.role IN ('to', 'cc')
                         AND promised.contact_id IS NOT NULL AND %[6]s))
       ELSE false END
  FROM activity a
 WHERE a.id = ANY($%[7]d) AND a.archived_at IS NULL AND (%[8]s)`,
		requestOpenSQL, confirmedRequestSQL, answeredSQL("a", until),
		promiseFollowUpSQL("kept", "a", until), orUnbounded(keptReadable),
		promiseRecipientAddressedSQL("kept", "promised.contact_id"), citedPos, orUnbounded(readable)), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading whether cited threads settled: %w", err)
	}
	var id ids.UUID
	var settled bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &settled}, func() error {
		out[id] = settled
		return nil
	}); err != nil {
		return nil, fmt.Errorf("activities: reading whether cited threads settled: %w", err)
	}
	return out, nil
}
