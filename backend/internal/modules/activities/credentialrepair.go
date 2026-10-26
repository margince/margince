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
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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
	changed := make([]ids.UUID, 0, len(want))
	for _, id := range ids.Ascending(want) {
		var kind crmcontracts.ActivityKind
		var subject, body *string
		err := tx.QueryRow(ctx, `
			SELECT kind, subject, body FROM activity
			 WHERE id = $1 AND restricted_at IS NULL FOR UPDATE`,
			id).Scan(&kind, &subject, &body)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("activities: reading the stored text of %s: %w", id, err)
		}
		if keepsTypedText(kind) {
			continue
		}
		wrote, err := maskRowTextTx(ctx, tx, id, subject, body)
		if err != nil {
			return nil, err
		}
		if wrote {
			changed = append(changed, id.UUID)
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
	maskedSubject, maskedBody := maskedPtr(subject), maskedPtr(body)
	if same(subject, maskedSubject) && same(body, maskedBody) {
		return false, nil
	}
	before := map[string]any{fieldSubject: deref(subject), fieldBody: deref(body)}
	// The old text is the concurrency guard, and the hold is re-stated under it.
	// A row restricted between the read and the write is not this pass's to
	// rewrite, and the row count below says so.
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET subject = $2, body = $3
		 WHERE id = $1 AND restricted_at IS NULL
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
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, changedBefore, changedAfter)
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

// maskedPtr is text with its credentials masked, keeping a nil as a nil: an
// absent subject is not an empty one.
func maskedPtr(text *string) *string {
	if text == nil {
		return nil
	}
	masked := credentialmask.MaskText(*text)
	return &masked
}

func same(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
