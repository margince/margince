// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The one piece of a departure that reaches into a sibling's table.
//
// Everything else deactivation ends is a credential this product issued to its
// own clients. A capture connection is not: it holds a credential a PROVIDER
// issued — a Gmail refresh token, a Graph grant, an IMAP password — and the
// poller that spends it asks nothing about the seat it belongs to. So a seat
// could be deactivated and their mailbox go on being read, which is the one
// thing an operator who deactivates a departing colleague is most entitled to
// assume stopped.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// captureConnectionObject is capture's own audit object name, spelled here
// because identity cannot import the module that owns it. A departure's rows
// have to read the same as a withdrawal's in one trail.
const captureConnectionObject = "capture_connection"

// evidenceKeyReason names why a mutation happened, beside the field images
// that say what it changed. Spelled as privacy spells it, so one query reads
// the reason off any audited write that carries one.
const evidenceKeyReason = "reason"

// withdrawCaptureConnections disconnects every capture connection this seat
// owns, inside the caller's transaction.
//
// It is capture's own withdrawal, minus the half that cannot be transactional.
// `Disconnect` runs three phases: flip the row, delete the vault secret, null
// the pointer. Only the first is database work — the second is a call to an
// external secret store, which no transaction can hold — so this does phase
// one and deliberately leaves `credential_ref` set. That is the same state a
// crash between capture's own phases leaves, which capture already treats as
// recoverable, and capture's sweep finishes it.
//
// The flip is what actually stops the reading: DueConnections selects on
// status IN ('connected','error'), so a disconnected row is not polled. The
// generation bump fences the cycles already out at the provider, so a sync
// that read a connected row minutes ago cannot commit a watermark against a
// seat that has since left. `auth` is nulled because a legacy row keeps its
// credential there rather than in the vault, and for that row this IS the
// destruction.
func withdrawCaptureConnections(ctx context.Context, tx pgx.Tx, userID ids.UserID) error {
	// The rows are read FOR UPDATE before the flip, the way capture's own
	// withdrawal does and for the same two reasons: the audit before-image has
	// to be the status the connection is actually leaving (an UPDATE's
	// RETURNING hands back the row it just wrote, which would make both images
	// say "disconnected" and record a change that reads as no change), and a
	// concurrent reconnect serializes behind this transaction rather than
	// racing it.
	rows, err := tx.Query(ctx, `
		SELECT id, provider, status, account_label FROM capture_connection
		 WHERE user_id = $1 AND archived_at IS NULL AND status <> 'disconnected'
		 FOR UPDATE`, userID)
	if err != nil {
		return err
	}
	type withdrawn struct {
		id       ids.UUID
		provider string
		prior    string
		label    *string
	}
	// Collected before the writes: they go to the same table and would
	// interleave with an open cursor on it.
	gone, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (withdrawn, error) {
		var w withdrawn
		return w, row.Scan(&w.id, &w.provider, &w.prior, &w.label)
	})
	if err != nil {
		return err
	}
	for _, w := range gone {
		// rls-store-path: identity writes capture's table under the workspace
		// GUC the deactivation transaction already holds; ratified in
		// crossStoreWrites.
		if _, err := tx.Exec(ctx, `
			UPDATE capture_connection
			   SET status = 'disconnected', auth = NULL, generation = generation + 1
			 WHERE id = $1`, w.id); err != nil {
			return err
		}
	}
	for _, w := range gone {
		// One row per connection, naming the status it was withdrawn FROM, so
		// the trail says which mailboxes a departure actually stopped rather
		// than that a departure happened.
		// The reason rides EVIDENCE rather than the images: the images are the
		// row's own fields, and a withdrawal by departure and a withdrawal by
		// hand leave the same ones. Why it happened is context about the
		// mutation, which is exactly what evidence is for — and it is the
		// difference between "this mailbox was disconnected" and "this mailbox
		// was disconnected because the colleague left".
		if _, err := storekit.AuditWithEvidence(ctx, tx, "archive", captureConnectionObject, w.id,
			captureConnectionImage(w.provider, w.prior, w.label),
			captureConnectionImage(w.provider, "disconnected", w.label),
			map[string]any{evidenceKeyReason: deactivatedUserRevokeReason}); err != nil {
			return err
		}
	}
	return nil
}

// captureConnectionImage mirrors capture's own audit image for this object, so
// a departure's rows and a withdrawal's rows carry the same shape.
func captureConnectionImage(provider, status string, accountLabel *string) map[string]any {
	return map[string]any{
		"provider":      provider,
		"status":        status,
		"account_label": accountLabel,
	}
}
