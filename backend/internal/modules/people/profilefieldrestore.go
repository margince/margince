// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Putting back the value a newer statement replaced.
//
// The undo half of recency. A signature or a card stating something newer
// replaces what the record holds and keeps the replaced value on the row, and
// this is what turns that buffer into an action a reader can take.
//
// The test is "is this still ours", asked of the record AS IT IS NOW rather
// than of the history — the same test providerclaimrevert.go makes, for the
// same reason. A restore that reached past a value somebody has since typed
// would undo their answer in order to undo the machine's, which is the one
// thing an undo must not do.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// restoredEvidence is the receipt a restored value carries: the value itself,
// and who had set it before it was replaced.
//
// The evidence column is NOT NULL and means "the text this value was read
// from". A restore reads it from the record's own history rather than from a
// signature or a page, so that is what it says — a quote borrowed from the
// replacement would be a receipt for the wrong value.
func restoredEvidence(value string, original *string) string {
	if original != nil && *original != "" {
		return fmt.Sprintf("Restored %q, set by %s before a later statement replaced it.", value, *original)
	}
	return fmt.Sprintf("Restored %q, replaced by a later statement.", value)
}

// restoreSource marks a value a human put back, distinct from the pass that
// wrote over it: the row must not go on claiming it was read from a signature.
const restoreSource = "human_restore"

// RestoreProfileField puts back the value a newer statement replaced.
//
// Refuses rather than reaching: ErrNotFound when the field holds nothing to
// restore, ErrConflict when the record has moved on since the replacement.
// Those are different answers on purpose — "there was no undo to make" and
// "there was, but somebody has answered since" send a reader to different
// places.
func (s *Store) RestoreProfileField(ctx context.Context, personID ids.PersonID, field string) error {
	if err := auth.Require(ctx, entityPerson, principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		// The subject first, and writable: an undo is a write to the person's
		// own record, and the eraser takes this row before what hangs off it.
		if err := auth.HoldWritableLive(ctx, tx, entityPerson, personID.UUID); err != nil {
			return err
		}
		// FOR UPDATE: the row is held from the moment it is read until the
		// restore lands, so a statement arriving mid-undo cannot slip between
		// the "is this still ours" test and the write it authorises.
		var current, superseded, evidence, sourceRef string
		var supersededBy *string
		var confidence *float64
		if err := tx.QueryRow(ctx, `
			SELECT value, superseded_value, superseded_captured_by, evidence_snippet,
			       source_ref, confidence
			  FROM person_profile_field
			 WHERE person_id = $1 AND field = $2 AND superseded_value IS NOT NULL
			 FOR UPDATE`,
			personID, field).Scan(&current, &superseded, &supersededBy, &evidence,
			&sourceRef, &confidence); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Either the field was never replaced or it never existed. One
				// answer for both, deliberately: telling them apart would
				// report whether a contact this caller may not read carries a
				// particular field.
				return apperrors.ErrNotFound
			}
			return fmt.Errorf("people: reading the field to restore: %w", err)
		}

		// Is this still ours? The column carries the display value for a
		// mirrored field, so it has to agree too — a title somebody retyped
		// leaves the sidecar untouched, and restoring on the sidecar alone
		// would put the old value back under their answer.
		// A human's CORRECTION blocks the undo, and it has to be asked for
		// separately: a correction is recorded in the ai_feedback ledger and
		// touches neither the column nor this row, so the CAS below cannot see
		// one. Restoring over it would put back a value the reader looked at
		// and rejected — and would move this row's updated_at, which is the
		// date their verdict is judged against, retiring the correction as
		// well as overruling it.
		//
		// Matched on the ledger's own key, which is a hash of the claim path:
		// spelling the path here rather than taking the hash apart is what
		// keeps the two sides agreeing, and ai.ProfileFieldClaimPath is the one
		// spelling. The store cannot import that module, so the caller-facing
		// half lives in the handler's package and this asks the table directly.
		var corrected bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM ai_feedback
				 WHERE subject_type = 'person' AND subject_id = $1
				   AND claim_kind = 'profile_field' AND verdict = 'corrected'
				   AND claim_key = encode(sha256(('profile_field:' || $2)::bytea), 'hex'))`,
			personID, field).Scan(&corrected); err != nil {
			return fmt.Errorf("people: looking for a correction on %s: %w", field, err)
		}
		if corrected {
			return apperrors.ErrConflict
		}

		// The comparison is IN the statement, not before it: reading the column
		// and then writing it leaves a window in which somebody types their own
		// answer and this overwrites it. RowsAffected is the CAS — zero means
		// the column no longer holds what was written, so the undo is refused.
		column, mirrored := observedFieldColumn(field)
		if mirrored {
			tag, err := tx.Exec(ctx,
				`UPDATE person SET `+column+` = $2
				  WHERE id = $1 AND archived_at IS NULL AND `+column+` = $3`,
				personID, superseded, current)
			if err != nil {
				return fmt.Errorf("people: restoring the displayed %s: %w", field, err)
			}
			if tag.RowsAffected() == 0 {
				return apperrors.ErrConflict
			}
		}

		// Through the one writer, under the precedence a restore IS: a human
		// choosing this value over the one that replaced it. Its clause also
		// clears the undo buffer, which is what stops the same undo being
		// offered a second time and undoing itself.
		// The evidence is REPLACED, not inherited. The snippet on the row
		// belongs to the value being undone — restoring "CFO" while keeping
		// the card's "VP Finance" quote would print a receipt for a value the
		// record no longer holds, and this table's whole promise is that a
		// value can be shown the text it was read from. The same for
		// confidence: nothing measured the restored value, and a score carried
		// over from the replacement would say a model had.
		landed, err := writePersonProfileField(ctx, tx, personID, personProfileFieldRow{
			Field: field, Value: superseded,
			EvidenceSnippet: restoredEvidence(superseded, supersededBy),
			SourceRef:       sourceRef,
			// captured_by is the authenticated restorer, like every other
			// mutation's. Who ORIGINALLY said this is a fact about the past and
			// belongs in the evidence line, not in the provenance stamp of a
			// row this person just wrote.
			Source: restoreSource, CapturedBy: by,
		}, replaceOnAcceptance)
		if err != nil {
			return err
		}
		if !landed {
			// The subject went inside the window the writer serializes.
			return apperrors.ErrNotFound
		}

		// The write shape. Named, not quoted, on both sides: audit_log is
		// append-only and outlives the erasure that clears the record, and this
		// field can be a phone number or a postal address.
		auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityPerson, personID.UUID,
			map[string]any{field: nil}, map[string]any{field: "restored"},
			map[string]any{auditKeySource: restoreSource, auditKeyFields: []string{field}})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, personID.UUID, crmcontracts.PublicEventPersonUpdated{
			ChangedFields: map[string]any{auditKeyFields: []string{field}, auditKeySource: restoreSource},
		})
	})
}

// RestorePersonProfileField implements POST /people/{id}/profile-fields/{field}/restore.
func (h Handlers) RestorePersonProfileField(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, field crmcontracts.PersonProfileFieldKey) {
	// No body. The field's own read overlays a human's verdict onto the stored
	// value (person360's readProfileFields), and answering with the row this
	// write just made would serve the value UNDER that overlay — the one
	// surface a reader would trust most, showing a claim they may already have
	// overridden. The page re-reads through the door that consults the ledger.
	if err := h.store.RestoreProfileField(r.Context(),
		ids.From[ids.PersonKind](ids.UUID(id)), string(field)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
