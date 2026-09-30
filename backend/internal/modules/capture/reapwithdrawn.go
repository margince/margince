// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Phases 2 and 3 of a withdrawal somebody else performed.
//
// A withdrawal is three phases and only the first is database work: flip the
// row, destroy the vault secret, clear the pointer. Disconnect runs all three
// because it owns the whole act. Deactivation cannot — the flip has to commit
// with the deactivation itself, and no transaction can hold a call to an
// external secret store — so it runs phase one and leaves the rest here.

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReapWithdrawnCredentials destroys the vaulted secret of every connection
// this seat has already had withdrawn, and clears the pointer to it.
//
// Called AFTER the withdrawing transaction commits, which is the only order
// that is safe: a secret destroyed before the flip commits would be gone from
// a connection that is still live if the transaction then rolled back.
//
// It is idempotent and holds nothing — a row whose ref it has already cleared
// simply does not match — so a run that dies halfway costs nothing but the
// work it had not reached, and the next one finishes it.
func (r *Registry) ReapWithdrawnCredentials(ctx context.Context, userID ids.UserID) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("capture: reaping withdrawn credentials without a workspace in context")
	}
	// Every statement below runs on the post-commit cleanup budget, detached
	// from the caller, the way Disconnect's own phases 2 and 3 are: the
	// withdrawal has already committed, so a client that hangs up the moment
	// it has its answer must not be the reason a revoked credential stays
	// decryptable — or, worse, the reason a secret is destroyed while the row
	// still names it, which is what cancelling between the delete and the
	// clear would leave.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), keyvault.CleanupTimeout)
	defer cancel()
	type stranded struct {
		provider string
		ref      string
	}
	var rows []stranded
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, `
			SELECT provider, credential_ref FROM capture_connection
			 WHERE user_id = $1 AND status = 'disconnected'
			   AND credential_ref IS NOT NULL AND archived_at IS NULL`, userID)
		if err != nil {
			return err
		}
		rows, err = pgx.CollectRows(cursor, func(row pgx.CollectableRow) (stranded, error) {
			var s stranded
			return s, row.Scan(&s.provider, &s.ref)
		})
		return err
	}); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	// A ref with no vault configured is a wiring fault rather than something to
	// skip past: clearing the pointer would leave the only name for a secret
	// nobody deleted.
	if r.vault == nil {
		return errors.New("capture: connections carry credential refs but no keyvault is configured to delete them")
	}
	for _, row := range rows {
		keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws, keyvault.Ref(row.ref), "deactivation")
		// Only the ref THIS pass resolved and deleted (credential_ref = $3).
		// Without that guard a reconnect landing in between — Connect writes a
		// NEW ref onto the same row — would have its live, still-vaulted ref
		// nulled from under it: a connected row with no credential, and the
		// fresh secret orphaned.
		if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				UPDATE capture_connection SET credential_ref = NULL
				 WHERE user_id = $1 AND provider = $2 AND credential_ref = $3`,
				userID, row.provider, row.ref)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
