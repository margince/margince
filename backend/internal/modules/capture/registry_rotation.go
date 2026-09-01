// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Persisting a credential the provider replaced mid-sync.
//
// Its own file because it is a WRITE to the connection's secret, and every
// other write in this module is a lifecycle event a human asked for —
// connect, reconnect, disconnect. This one happens because a provider chose
// to, during a background pull nobody is watching, and the rules it has to
// obey are its own: never fail the sync over it, never leave the row pointing
// at a blob that was not written, and never retire the old secret until the
// row that replaced it has committed.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rotationSink re-seals one connection's credential. Built per sync, so the
// connection id and the generation it was read at are fixed for the life of the
// sink and cannot be confused with another connection's.
type rotationSink struct {
	registry     *Registry
	connectionID ids.UUID
	// generation is the same fence SyncOnce reads before the pull. A lifecycle
	// change since then has moved it, and the re-seal must then land nowhere:
	// a disconnect that destroyed the credential must not be undone by a
	// rotation racing behind it.
	generation int
	// readRef is the credential this sync actually read. It fences the write
	// alongside the generation because a SAME-ACCOUNT reconnect does not bump
	// the generation — only a rebind does — so a reconnect committing mid-pull
	// would otherwise have its fresh credential overwritten by a replacement
	// this sync derived from the one it replaced.
	readRef *string
	log     *slog.Logger
}

// Rotated seals the replacement and points the connection at it.
//
// The order is seal → record → retire, the same one the Google-app store uses
// and for the same reason. Sealing first means a crash before the record leaves
// an orphan blob, which costs bytes; recording first would leave the row
// naming a secret that was never written, which costs the mailbox. Retiring the
// superseded blob only after the record commits means a crash between them
// leaves the OLD secret readable, which is the direction that still works.
func (s rotationSink) Rotated(ctx context.Context, auth connector.Auth) error {
	if s.registry.vault == nil {
		// A role that seals nothing has nowhere to put this. Not an error: the
		// old credential is still valid, and refusing here would turn a
		// vault-less deployment's every sync into a failure.
		return nil
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return fmt.Errorf("capture: rotating a credential outside workspace context")
	}
	wsID := ids.From[ids.WorkspaceKind](ws)

	ref, err := s.registry.vault.Put(ctx, wsID, []byte(auth))
	if err != nil {
		return fmt.Errorf("capture: sealing the rotated credential: %w", err)
	}

	superseded, previous, err := s.record(ctx, ref)
	if err != nil {
		// The blob just written is deliberately LEFT. A commit can fail
		// ambiguously — the connection drops after Postgres committed and
		// before the client hears so — and deleting on that path would destroy
		// the credential the row now points at, leaving a live connection
		// naming a secret nothing can resolve. An orphan costs bytes; this
		// costs the mailbox, so the uncertainty is resolved in the direction
		// that still works.
		return err
	}
	if superseded {
		// The connection changed under this sync — disconnected, reconnected,
		// or rotated by a cycle that got there first. Its credential is not
		// this sink's to move, and the blob just sealed belongs to nobody.
		keyvault.DeleteDetached(ctx, s.registry.vault, s.log, ws, ref, "rotation-superseded")
		return nil
	}
	if previous != "" && previous != string(ref) {
		keyvault.DeleteDetached(ctx, s.registry.vault, s.log, ws, keyvault.Ref(previous), "rotation")
	}
	return nil
}

// record points the connection at the new ref and reports the one it replaced.
//
// TWO fences, matched in the same statement as the write rather than checked
// before it. The generation catches a disconnect or a rebind, whose
// credential-destroying update must not be overwritten by this one. The
// credential ref catches what the generation cannot: a same-account reconnect
// leaves the generation alone, so only "the credential I read is still the
// credential on the row" distinguishes a rotation of the live grant from one
// derived out of a grant that has since been replaced.
//
// `auth` is cleared in the same statement. A legacy row that never reached the
// vault carries its credential in that column, and making the ref authoritative
// while leaving the column populated would retire nothing — the superseded
// secret would sit in the clear for the life of the installation.
func (s rotationSink) record(ctx context.Context, ref keyvault.Ref) (superseded bool, previous string, err error) {
	err = s.registry.db.Tx(ctx, func(tx pgx.Tx) error {
		var prior *string
		// The self-join is what makes RETURNING report the ref being REPLACED
		// rather than the one just written: a plain RETURNING credential_ref
		// yields the new value, and retiring that would destroy the secret the
		// row now points at.
		scanErr := tx.QueryRow(ctx, `
			UPDATE capture_connection AS c SET credential_ref = $2, auth = NULL
			FROM capture_connection AS before
			WHERE c.id = before.id AND c.id = $1 AND c.generation = $3
			  AND c.credential_ref IS NOT DISTINCT FROM $4
			  AND c.status IN ('connected','error')
			RETURNING before.credential_ref`,
			s.connectionID, string(ref), s.generation, s.readRef).Scan(&prior)
		if scanErr == pgx.ErrNoRows {
			superseded = true
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		if prior != nil {
			previous = *prior
		}
		// Recorded in the SAME transaction as the ref it describes, so the
		// ledger cannot claim a rotation that was rolled back.
		//
		// system_log rather than audit_log — the posture platform/extsecrets
		// already declares for this shape: bytes moved and nothing changed
		// meaning. Nobody DECIDED this; a provider replaced a secret during a
		// background pull, so there is no domain change to audit. What an
		// operator wants from it is the operational fact, and it is what makes
		// an aged-out mailbox diagnosable — either the rotations stopped
		// appearing, or they never did.
		//
		// The secret is never a detail field. The refs are opaque addresses,
		// and they are what a reader needs to follow the chain.
		_, logErr := storekit.LogSystem(ctx, tx, "capture.credential_rotated", map[string]any{
			"connection_id":  s.connectionID.String(),
			"credential_ref": string(ref),
			"replaced_ref":   previous,
		})
		return logErr
	})
	return superseded, previous, err
}
