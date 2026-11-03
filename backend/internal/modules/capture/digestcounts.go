// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Re-read legacy payloads through the same personal window used by new builds.
// Membership in a workspace grants access, not responsibility for its imports.
func readDigestCounts(ctx context.Context, tx pgx.Tx, userID ids.UUID, since, until time.Time, p *DigestPayload) error {
	args := []any{}
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	start, end, reader := arg(since), arg(until), arg(userID)
	imported := " AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.audience = 'workspace' AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = " + reader + ")"
	created := "created_at >= " + start + " AND created_at <= " + end
	labeled := "capture_labeled_at >= " + start + " AND capture_labeled_at <= " + end
	err := tx.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM activity a WHERE a.captured_by LIKE 'connector:%' AND a.kind = 'email' AND `+created+imported+`),
  (SELECT count(*) FROM contact WHERE captured_by LIKE 'connector:%' AND `+created+` AND owner_id = `+reader+`),
  (SELECT count(*) FROM company WHERE (captured_by LIKE 'connector:%' OR source LIKE 'domain\_triage:%') AND `+created+` AND owner_id = `+reader+`),
  (SELECT count(*) FROM activity a WHERE capture_label = 'commitment' AND `+labeled+imported+`),
  (SELECT count(*) FROM activity a WHERE capture_label = 'meeting' AND `+labeled+imported+`),
  (SELECT count(*) FROM activity a WHERE capture_label = 'noise' AND `+labeled+imported+`)`, args...).Scan(
		&p.Capture.ActivitiesCreated, &p.Capture.ContactsCreated, &p.Capture.CompaniesCreated,
		&p.Review.Classify.Commitments, &p.Review.Classify.Meetings, &p.Review.Classify.Noise)
	if err != nil {
		return fmt.Errorf("capture: digest counts: %w", err)
	}
	p.Capture.MessagesSynced = p.Capture.ActivitiesCreated
	return nil
}
