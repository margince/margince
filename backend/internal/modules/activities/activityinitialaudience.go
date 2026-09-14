// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Initial audience and members commit before the creation audit and event.
// Selected membership is explicit; capture's audience_reason vocabulary does
// not describe this new task's audience. Its source link records the derivation.
func recordInitialActivity(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in LogActivityInput) error {
	after := map[string]any{fieldKind: in.Kind, fieldSubject: in.Subject}
	if len(in.audienceMembers) > 0 {
		args := []any{id}
		if result, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE activity SET audience = 'selected' WHERE id = $%d AND archived_at IS NULL AND restricted_at IS NULL`, len(args)), args...); err != nil {
			return err
		} else if result.RowsAffected() != 1 {
			return fmt.Errorf("activities: initial audience row unavailable")
		}
		if err := replaceAudienceMembers(ctx, tx, id, in.audienceMembers); err != nil {
			return err
		}
		audience, err := readAudienceImage(ctx, tx, id)
		if err != nil {
			return err
		}
		for column, value := range audience {
			after[column] = value
		}
		after["source_system"], after["source_activity_id"], after["assignee_id"] = in.SourceSystem, in.SourceActivityID, in.AssigneeID
	}
	auditID, err := storekit.Audit(ctx, tx, "create", "activity", id.UUID, nil, after)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, activityCapturedPayload(in.Kind, in.ChannelProvider))
}
