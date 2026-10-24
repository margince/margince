// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MembersOnlyHoldable is the condition, on the activity aliased a, for stored
// mail the members_only repair may hold. The row is captured, open and under
// no statutory hold, and no person decided its audience. A row a human set,
// and one a mailbox owner shared, record the decision the hold leaves alone.
const MembersOnlyHoldable = `a.kind = 'email' AND a.audience = 'workspace' AND a.restricted_at IS NULL
   AND a.audience_reason IS DISTINCT FROM 'manual'
   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id)
   AND NOT EXISTS (SELECT 1 FROM capture_import ci
                    WHERE ci.activity_id = a.id AND ci.verdict_status = 'shared_by_owner')`

// HoldMembersOnlyTx places the members_only hold on stored mail that capture
// opened before it knew the rule. The caller judges membership; this writes
// through the derivation's own writer, so the audit row and activity.updated
// event match a hold placed at capture. It answers how many rows it held.
func HoldMembersOnlyTx(ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	held := 0
	for _, id := range ids.Ascending(activityIDs) {
		var stored string
		var storedReason *string
		err := tx.QueryRow(ctx, `
			SELECT a.audience, a.audience_reason FROM activity a
			 WHERE a.id = $1 AND `+MembersOnlyHoldable+` FOR UPDATE OF a`, id).Scan(&stored, &storedReason)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("activities: reading mail to hold as members-only: %w", err)
		}
		if err := writeDerivedAudienceTx(ctx, tx, id, stored, storedReason, audienceParticipants, ReasonMembersOnly); err != nil {
			return 0, err
		}
		held++
	}
	return held, nil
}
