// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const linkedInMatchNoLongerSuggested = "the connection no longer suggests this contact; the question is withdrawn"

// WithdrawStaleLinkedInMatchesInTx withdraws one member's pending proposals of
// kind whose connection is no longer suggested for the contact they name.
//
// A contact archived or merged away, or a connection decided elsewhere, leaves
// a card whose accept would fail on every retry. The staging pass calls this
// before it stages, so a live pair gets a fresh card instead of joining a dead
// one. It answers how many it withdrew.
func WithdrawStaleLinkedInMatchesInTx(ctx context.Context, tx pgx.Tx, kind string, owner ids.UUID) (int, error) {
	withdrawn, err := withdrawPending(ctx, tx, kind, linkedInMatchNoLongerSuggested, func(arg func(any) int) string {
		return fmt.Sprintf(`proposed_change->>'owner_user_id' = $%d::text
			AND NOT EXISTS (
			    SELECT 1 FROM linkedin_connection c
			     WHERE c.id::text = proposed_change->>'connection_id'
			       AND c.match_status = 'suggested' AND c.tombstoned_at IS NULL
			       AND c.matched_contact_id::text = proposed_change->>'contact_id')`, arg(owner.String()))
	})
	return len(withdrawn), err
}
