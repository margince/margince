// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// staleMailLabelScope is mail whose body was restored from its original after
// the classifier last judged it, or declined to. The label or decline was read
// from the text the restoration replaced. A pass that labelled the mail after
// the restoration read the new text, so it is left alone.
const staleMailLabelScope = `FROM activity a
 WHERE a.kind = 'email' AND a.restricted_at IS NULL AND a.archived_at IS NULL
   AND (a.capture_label IS NOT NULL OR a.capture_label_declined_at IS NOT NULL)
   AND EXISTS (SELECT 1 FROM audit_log restored
         WHERE restored.entity_type = 'activity' AND restored.entity_id = a.id
           AND restored.action = 'update' AND restored.after @> '{"body_restored": true}'
           AND restored.occurred_at > greatest(a.capture_labeled_at, a.capture_label_declined_at))`

// StaleMailLabelPage is one page of the label reset.
type StaleMailLabelPage struct {
	Scanned []ids.UUID
	Reset   int
}

// ResetStaleMailLabelsTx puts mail back in the classify backlog when its body
// was restored after the classifier read it. Like the label write beside it,
// the reset mints no audit entry and no event.
func ResetStaleMailLabelsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (StaleMailLabelPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return StaleMailLabelPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id `+staleMailLabelScope+` AND a.id > $1 ORDER BY a.id LIMIT $2 FOR UPDATE OF a`, after, limit)
	if err != nil {
		return StaleMailLabelPage{}, fmt.Errorf("activities: reading mail labelled before its body was restored: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return StaleMailLabelPage{}, fmt.Errorf("activities: reading mail labelled before its body was restored: %w", err)
	}
	if len(scanned) == 0 {
		return StaleMailLabelPage{}, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE activity SET capture_label = NULL, capture_labeled_at = NULL,
		       capture_label_declined_at = NULL, capture_label_declined_ruleset = NULL
		 WHERE id = ANY($1)`, scanned)
	if err != nil {
		return StaleMailLabelPage{}, fmt.Errorf("activities: resetting labels read from a body since restored: %w", err)
	}
	return StaleMailLabelPage{Scanned: scanned, Reset: int(tag.RowsAffected())}, nil
}

// CountStaleMailLabels counts, up to maximum+1, the mail ResetStaleMailLabelsTx
// would reset.
func CountStaleMailLabels(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64) (int64, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT a.id `+staleMailLabelScope+` LIMIT $1) stale`, maximum+1).Scan(&count)
	return count, err
}
