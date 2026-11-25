// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// carryLeadWorkToLead moves the merged-away lead's hand-offs, the factors a
// rep set by hand, and the earlier merges that point at it. Left behind, each
// hangs off an archived lead that no read of the survivor returns.
func carryLeadWorkToLead(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.LeadID) error {
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID}
	for _, step := range []struct{ what, stmt string }{
		{"hand-offs", `UPDATE sdr_handoff SET lead_id = @target WHERE lead_id = @source`},
		// One live factor per lead: the survivor's own answer stands, as every
		// other merge collision is settled, and the retired copy goes.
		{"colliding manual factors", `DELETE FROM lead_manual_signal a
		  WHERE a.lead_id = @source AND a.superseded_at IS NULL AND EXISTS (
		    SELECT 1 FROM lead_manual_signal b
		     WHERE b.lead_id = @target AND b.factor = a.factor AND b.superseded_at IS NULL)`},
		{"manual factors", `UPDATE lead_manual_signal SET lead_id = @target WHERE lead_id = @source`},
		// The redirect chain stays one hop deep, as the contact merge keeps it.
		{"earlier merges", `UPDATE lead SET merged_into_id = @target WHERE merged_into_id = @source`},
	} {
		if _, err := tx.Exec(ctx, step.stmt, args); err != nil {
			return fmt.Errorf("carry the merged-away lead's %s: %w", step.what, err)
		}
	}
	return nil
}
