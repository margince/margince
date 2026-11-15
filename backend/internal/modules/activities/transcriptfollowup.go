// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TranscriptFollowUpExistsTx locks the source so typed and extracted next steps cannot race.
// An archived meeting can still anchor a readable transcript.
func (s *Store) TranscriptFollowUpExistsTx(ctx context.Context, tx pgx.Tx, transcript ids.ActivityID, subject string, assignee *ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	if assignee == nil {
		return false, nil
	}
	source, err := readActivityContent(ctx, tx, transcript, storekit.LiveOnly)
	if err != nil {
		return false, err
	}
	meeting := ids.UUID(source.Id)
	if source.SourceActivityId != nil {
		meeting = ids.UUID(*source.SourceActivityId)
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", meeting, storekit.IncludeArchived); err != nil {
		return false, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	visibility, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return false, err
	}
	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM activity a
        WHERE a.kind = 'task' AND a.archived_at IS NULL AND coalesce(a.source_system, '') = ''
          AND a.source_activity_id = $%d
          AND a.assignee_id = $%d
          AND lower(trim(regexp_replace(a.subject, '\s+', ' ', 'g'))) = lower(trim(regexp_replace($%d::text, '\s+', ' ', 'g')))
          AND (%s))`, arg(meeting), arg(*assignee), arg(subject), visibility)
	var exists bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
		return false, fmt.Errorf("read the meeting's filed next step: %w", err)
	}
	return exists, nil
}
