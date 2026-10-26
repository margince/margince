// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type commitmentTaskIdentity struct {
	Locator  string
	Assignee *ids.UUID
}

func commitmentTaskHistoryTx(ctx context.Context, tx pgx.Tx, source ids.UUID) ([]commitmentTaskIdentity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, source); err != nil {
		return nil, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	// Identity memory is bounded by the visible source, like CommitmentTaskWritten.
	// A private or held derivative must still prevent recreating the same work.
	// The first update's before-image retains the original actor even after
	// human edits. Current participants need not include that actor anymore.
	unscrubbed := privacy.UnscrubbedImageSQL("h", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs())))
	query := fmt.Sprintf(`SELECT a.source_id, COALESCE((SELECT m.holder FROM (%s) m), a.assignee_id)
 FROM activity a WHERE a.source_activity_id=$%d AND a.source_system=$%d`,
		firstMoveSQL("activity", "a", "assignee_id", unscrubbed), arg(source), arg(activities.CommitmentTaskSource))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading commitment task identities: %w", err)
	}
	defer rows.Close()
	var history []commitmentTaskIdentity
	for rows.Next() {
		var item commitmentTaskIdentity
		if err := rows.Scan(&item.Locator, &item.Assignee); err != nil {
			return nil, fmt.Errorf("activities: decoding commitment task identity: %w", err)
		}
		history = append(history, item)
	}
	return history, rows.Err()
}
