// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The repair of mail captured or sent before the masking rules existed.
//
// A credential that arrived in clear stays readable in the row, in the
// embeddings built from it and in the search index. Nothing the live writers do
// reaches it.
//
// The rules that mask a new mail mask an old one, so this writer calls
// credentialmask and never a pattern of its own.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EvidenceKeyMaskedStoredText flags the audit row of a mail whose stored text
// MaskStoredMailTextTx masked. Only this writer sets it, so a later step finds
// the mail it masked here and never a mail whose sender typed the marker.
const EvidenceKeyMaskedStoredText = "stored_credentials_masked"

// LockUnheldActivitiesTx locks the named activities that are under no statutory
// hold and answers their ids. A hold is placed by an UPDATE of the same row.
// So a hold placed after the caller selected its page either commits first and
// leaves the row out, or waits for this transaction.
func LockUnheldActivitiesTx(ctx context.Context, tx pgx.Tx, want []ids.UUID) (map[ids.UUID]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM activity WHERE id = ANY($1) AND restricted_at IS NULL
		 ORDER BY id FOR UPDATE`, want)
	if err != nil {
		return nil, fmt.Errorf("activities: locking the rows under no hold: %w", err)
	}
	locked, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: locking the rows under no hold: %w", err)
	}
	unheld := make(map[ids.UUID]bool, len(locked))
	for _, id := range locked {
		unheld[id] = true
	}
	return unheld, nil
}

// MaskStoredMailTextTx masks the credentials left in the subject and body of
// the named rows, and answers the ones it changed.
//
// A kind that keeps its text as typed is skipped, not masked: the marker would
// then stand where a member wrote a word. Masked text masks to itself, so a row
// this has already repaired produces no write and a replay changes nothing.
func MaskStoredMailTextTx(
	ctx context.Context, tx pgx.Tx, want []ids.ActivityID,
) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	asked := make([]ids.UUID, 0, len(want))
	for _, id := range ids.Ascending(want) {
		asked = append(asked, id.UUID)
	}
	unheld, err := LockUnheldActivitiesTx(ctx, tx, asked)
	if err != nil {
		return nil, err
	}
	changed := make([]ids.UUID, 0, len(want))
	for _, id := range ids.Ascending(want) {
		if !unheld[id.UUID] {
			continue
		}
		var kind crmcontracts.ActivityKind
		var subject, body *string
		var archived *time.Time
		if err := tx.QueryRow(ctx, `SELECT kind, subject, body, archived_at FROM activity WHERE id = $1`,
			id).Scan(&kind, &subject, &body, &archived); err != nil {
			return nil, fmt.Errorf("activities: reading the stored text of %s: %w", id, err)
		}
		if keepsTypedText(kind) {
			continue
		}
		wrote, err := maskRowTextTx(ctx, tx, id, subject, body)
		if err != nil {
			return nil, err
		}
		if !wrote {
			continue
		}
		changed = append(changed, id.UUID)
		if archived == nil {
			continue
		}
		// An archived row is not embeddable, so the update this mask emits
		// rebuilds no vector. The one built while it was live holds the
		// unmasked text, so it is dropped in the same transaction.
		if _, err := tx.Exec(ctx, `
			DELETE FROM embedding WHERE entity_type = 'activity' AND entity_id = $1`,
			id); err != nil {
			return nil, fmt.Errorf("activities: dropping the masked activity's embedding: %w", err)
		}
	}
	return changed, nil
}

// maskRowTextTx writes the masked subject and body of one row, and answers
// whether anything moved. The old text pins the update, so a live edit landing
// between the read and the write leaves this row to the next pass.
func maskRowTextTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, subject, body *string,
) (bool, error) {
	maskedSubject, maskedBody := maskedText(subject), maskedText(body)
	if same(subject, maskedSubject) && same(body, maskedBody) {
		return false, nil
	}
	before := map[string]any{fieldSubject: deref(subject), fieldBody: deref(body)}
	// The caller holds the row lock; the old text guards against a writer that
	// does not take it.
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET subject = $2, body = $3
		 WHERE id = $1
		   AND subject IS NOT DISTINCT FROM $4 AND body IS NOT DISTINCT FROM $5`,
		id, maskedSubject, maskedBody, subject, body)
	if err != nil {
		return false, fmt.Errorf("activities: masking the stored text of %s: %w", id, err)
	}
	if tag.RowsAffected() != 1 {
		// The text moved under the lock, so the next pass reads it again. An
		// audit row here would describe a change that did not happen.
		return false, nil
	}
	after := map[string]any{fieldSubject: deref(maskedSubject), fieldBody: deref(maskedBody)}
	changedBefore, changedAfter := storekit.ChangedColumns(before, after)
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", id.UUID,
		changedBefore, changedAfter, map[string]any{EvidenceKeyMaskedStoredText: true})
	if err != nil {
		return false, err
	}
	// The same event an edit emits, because the embeddings are rebuilt from it.
	// `search_tsv` is generated, so the index follows the row without being
	// told.
	//
	// Only the columns that moved are named. A subscriber acting on a field
	// this pass left alone would rebuild from a change that did not happen.
	changed := crmcontracts.PublicEventActivityChangedFields{}
	if !same(subject, maskedSubject) {
		changed.Subject = maskedSubject
	}
	if bodyChanged := !same(body, maskedBody); bodyChanged {
		changed.Body = &bodyChanged
	}
	return true, storekit.EmitEvent(ctx, tx, auditID, id.UUID,
		crmcontracts.PublicEventActivityUpdated{ChangedFields: changed})
}

func same(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
