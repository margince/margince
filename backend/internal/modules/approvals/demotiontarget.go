// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// demotionTargetsDecidable answers, by position, whether each staged reversal
// names a lead the caller may change that is still in the promoted state.
func demotionTargetsDecidable(ctx context.Context, tx pgx.Tx, refs []targetRef) ([]bool, error) {
	out := make([]bool, len(refs))
	var leads []ids.UUID
	for _, ref := range refs {
		if ref.Type != nil && *ref.Type == tableLead && ref.ID != nil {
			leads = append(leads, *ref.ID)
		}
	}
	if len(leads) == 0 {
		return out, nil
	}
	readable, err := probeAnswer(auth.Require(ctx, tableLead, principal.ActionRead))
	if err != nil || !readable {
		return out, err
	}
	// Not live: promotion archives the lead, and only that state admits this action.
	writable, err := auth.PassingSubset(ctx, tx, tableLead, leads, auth.RuleWritable)
	if err != nil || len(writable) == 0 {
		return out, err
	}
	promoted, err := idsAmong(ctx, tx, `SELECT id FROM lead WHERE id = ANY($1) AND status = 'promoted'
		AND archived_at IS NOT NULL AND promoted_contact_id IS NOT NULL`, leads)
	if err != nil {
		return nil, err
	}
	for i, ref := range refs {
		out[i] = ref.Type != nil && *ref.Type == tableLead && ref.ID != nil && writable[*ref.ID] && promoted[*ref.ID]
	}
	return out, nil
}
