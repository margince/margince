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
func (s *Store) RestoreProfileField(ctx context.Context, personID ids.PersonID, field string) (crmcontracts.PersonProfileField, error) {
	var out crmcontracts.PersonProfileField
	if err := auth.Require(ctx, entityPerson, principal.ActionUpdate); err != nil {
		return out, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return out, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The subject first, and writable: an undo is a write to the person's
		// own record, and the eraser takes this row before what hangs off it.
		if err := auth.HoldWritableLive(ctx, tx, entityPerson, personID.UUID); err != nil {
			return err
		}
		var current, superseded string
		var supersededBy *string
		if err := tx.QueryRow(ctx, `
			SELECT value, superseded_value, superseded_captured_by
			  FROM person_profile_field
			 WHERE person_id = $1 AND field = $2 AND superseded_value IS NOT NULL
			 FOR UPDATE`,
			personID, field).Scan(&current, &superseded, &supersededBy); err != nil {
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
		column, mirrored := observedFieldColumn(field)
		if mirrored {
			var displayed *string
			if err := tx.QueryRow(ctx,
				`SELECT `+column+` FROM person WHERE id = $1`, personID).Scan(&displayed); err != nil {
				return fmt.Errorf("people: reading the displayed %s: %w", field, err)
			}
			if displayed == nil || *displayed != current {
				return apperrors.ErrConflict
			}
			if _, err := tx.Exec(ctx,
				`UPDATE person SET `+column+` = $2 WHERE id = $1 AND archived_at IS NULL`,
				personID, superseded); err != nil {
				return fmt.Errorf("people: restoring the displayed %s: %w", field, err)
			}
		}

		// The buffer is one level deep, so restoring empties it. Left behind it
		// would offer the same undo a second time and put back a value the
		// record already carries.
		if _, err := tx.Exec(ctx, `
			UPDATE person_profile_field
			   SET value = superseded_value,
			       captured_by = COALESCE(superseded_captured_by, $3),
			       source = $4,
			       superseded_value = NULL,
			       superseded_captured_by = NULL,
			       superseded_observed_at = NULL
			 WHERE person_id = $1 AND field = $2`,
			personID, field, by, restoreSource); err != nil {
			return fmt.Errorf("people: restoring the %s field: %w", field, err)
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
		if err := storekit.EmitEvent(ctx, tx, auditID, personID.UUID, crmcontracts.PublicEventPersonUpdated{
			ChangedFields: map[string]any{auditKeyFields: []string{field}, auditKeySource: restoreSource},
		}); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT field, value, evidence_snippet, source_ref, confidence, source, captured_by,
			       updated_at, observed_at, superseded_value, superseded_observed_at
			  FROM person_profile_field WHERE person_id = $1 AND field = $2`,
			personID, field).Scan(&out.Field, &out.Value, &out.EvidenceSnippet, &out.SourceRef,
			&out.Confidence, &out.Source, &out.CapturedBy, &out.CapturedAt,
			&out.ObservedAt, &out.SupersededValue, &out.SupersededObservedAt)
	})
	if err != nil {
		return crmcontracts.PersonProfileField{}, err
	}
	return out, nil
}

// RestorePersonProfileField implements POST /people/{id}/profile-fields/{field}/restore.
func (h Handlers) RestorePersonProfileField(w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, field crmcontracts.PersonProfileFieldKey) {
	restored, err := h.store.RestoreProfileField(r.Context(),
		ids.From[ids.PersonKind](ids.UUID(id)), string(field))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, restored)
}
