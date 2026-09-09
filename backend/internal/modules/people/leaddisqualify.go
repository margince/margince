// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Retiring a lead: the one path that enforces "disqualified ⇒ archived".
//
// Split from lead.go for size, along the seam that was already there — that
// file creates, reads and lists leads, and this one closes them.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// DisqualifyLeadInput is why the lead is closed. Both fields are optional on
// the wire so the governed agent path still works; the UI always sends a
// reason. Disqualifying is the one path enforcing "disqualified ⇒ archived"
// (DELETE /leads/{id} in the contract).
type DisqualifyLeadInput struct {
	ReasonID *ids.UUID
	Note     *string
}

// ensureDisqualifyReasonIfNamed checks a reason the caller supplied and stands
// down for one they did not: the field is optional, and an absent reason is a
// disqualification nobody explained rather than one naming a retired code.
func ensureDisqualifyReasonIfNamed(ctx context.Context, tx pgx.Tx, reasonID *ids.UUID) error {
	if reasonID == nil {
		return nil
	}
	return ensureActiveDisqualifyReason(ctx, tx, *reasonID)
}

// DisqualifyLead retires a lead in place: the status, the reason and the
// archive instant, with the row surviving so it stays fetchable by id.
//
// An optional precondition refuses the write when a human has touched the lead
// since a caller-named instant; untouchedguard.go says why it is asked inside
// this transaction.
func (s *Store) DisqualifyLead(
	ctx context.Context, id ids.LeadID, in DisqualifyLeadInput, opts ...WriteOption,
) (crmcontracts.Lead, error) {
	options := collectWriteOptions(opts)
	if err := auth.Require(ctx, entityLead, principal.ActionDelete); err != nil {
		return crmcontracts.Lead{}, err
	}
	return s.leadWrite(ctx, id,
		func(tx pgx.Tx, active []fieldcatalog.Column) (crmcontracts.Lead, error) {
			var out crmcontracts.Lead
			// The row lock makes the status read and the update below one
			// race-free unit.
			if _, err := storekit.LockRow(ctx, tx, entityLead, id.UUID, storekit.LiveOnly); err != nil {
				return out, err
			}
			// The precondition, under the row lock the write takes: a caller that
			// asked for this write only while nobody had touched the record gets
			// that answered HERE rather than in a read that already committed.
			if err := refuseIfHumanTouched(ctx, tx, entityLead, id.UUID, options); err != nil {
				return out, err
			}
			current, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
			if err != nil {
				return out, err
			}
			// Under the lock, against the row this transaction will write — the
			// version an agent's released approval was granted against, which
			// its redemption verified in a transaction that has since committed.
			if err := refuseIfVersionMoved(entityLead, current.Version, options); err != nil {
				return out, err
			}
			if err := ensureDisqualifyReasonIfNamed(ctx, tx, in.ReasonID); err != nil {
				return out, err
			}
			setBy, err := statusSetByFor(ctx)
			if err != nil {
				return out, err
			}
			if _, err := tx.Exec(ctx,
				`UPDATE lead SET status = 'disqualified', status_set_by = $4, archived_at = now(), disqualify_reason_id = $2, disqualify_note = $3, `+
					firstResponseSet+` WHERE id = $1 AND archived_at IS NULL`,
				id, in.ReasonID, in.Note, setBy); err != nil {
				return out, err
			}
			// A retired record carries no tags, the same rule the company and person
			// archive paths hold. It matters here because an import files what it
			// creates under one word: an undone run archives the lead, and a lead
			// left tagged still answers a filter for the batch that was reversed.
			if _, err := tx.Exec(ctx,
				`DELETE FROM taggable WHERE entity_type = 'lead' AND entity_id = $1`, id); err != nil {
				return out, fmt.Errorf("drop the lead's tags: %w", err)
			}
			after := map[string]any{leadStatusColumn: "disqualified"}
			if in.ReasonID != nil {
				after["disqualify_reason_id"] = *in.ReasonID
			}
			if in.Note != nil {
				after["disqualify_note"] = *in.Note
			}
			auditID, err := storekit.Audit(ctx, tx, "archive", entityLead, id.UUID,
				map[string]any{leadStatusColumn: current.Status}, after)
			if err != nil {
				return out, err
			}
			if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadDisqualified{}); err != nil {
				return out, err
			}
			return readLead(ctx, tx, id, storekit.IncludeArchived, active)
		})
}
