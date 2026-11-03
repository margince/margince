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
		if lead, ok := leadOf(ref); ok {
			leads = append(leads, lead)
		}
	}
	leads = distinct(leads)
	if len(leads) == 0 {
		return out, nil
	}
	reversible, err := reversibleLeads(ctx, tx, leads)
	if err != nil {
		return nil, err
	}
	for i, ref := range refs {
		lead, ok := leadOf(ref)
		out[i] = ok && reversible[lead]
	}
	return out, nil
}

// leadOf is the lead a reversal names; any other target is not reversible.
func leadOf(ref targetRef) (ids.UUID, bool) {
	if ref.Type == nil || *ref.Type != tableLead || ref.ID == nil {
		return ids.Nil, false
	}
	return *ref.ID, true
}

// reversibleLeads is the leads the caller may read and change that are still
// promoted. It takes one statement for write authority and one for the state.
func reversibleLeads(ctx context.Context, tx pgx.Tx, leads []ids.UUID) (map[ids.UUID]bool, error) {
	readable, err := probeAnswer(auth.Require(ctx, tableLead, principal.ActionRead))
	if err != nil || !readable {
		return map[ids.UUID]bool{}, err
	}
	// Not live: promotion archives the lead, and only that state admits this action.
	writable, err := auth.PassingSubset(ctx, tx, tableLead, leads, auth.RuleWritable)
	if err != nil || len(writable) == 0 {
		return writable, err
	}
	promoted, err := idsAmong(ctx, tx, `SELECT id FROM lead WHERE id = ANY($1) AND status = 'promoted'
		AND archived_at IS NOT NULL AND promoted_contact_id IS NOT NULL`, leads)
	if err != nil {
		return nil, err
	}
	for id := range writable {
		writable[id] = promoted[id]
	}
	return writable, nil
}
