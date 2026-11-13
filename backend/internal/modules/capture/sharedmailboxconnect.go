// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SharedConnect is what an admin hands over to connect a shared mailbox.
type SharedConnect struct {
	Provider   string
	Address    string
	ReaderTeam ids.UUID
	// Posture is held or shared. Empty starts a new mailbox held and keeps a
	// reconnected one as it is.
	Posture string
	// BackupOwner holds the work of mail whose sender no reader owns. Nil
	// names none on a new mailbox and keeps a reconnected one's.
	BackupOwner *ids.UUID
	Auth        connector.Auth
}

// SharedReconnectError refuses a reconnect that would also move the mailbox.
// Readers and visibility change only through ChangeSharedMailbox, which asks
// for a confirmation first.
type SharedReconnectError struct{ visibility bool }

func (e *SharedReconnectError) Error() string {
	if e.visibility {
		return "this shared mailbox is already connected with another mail visibility; change it on the mailbox first, then reconnect"
	}
	return "this shared mailbox is already connected with other readers; change them on the mailbox first, then reconnect"
}

// MessageFault answers the refusal as the conflict it unwraps to.
func (e *SharedReconnectError) MessageFault() (code, message string) {
	if e.visibility {
		return "shared_mailbox_visibility_differs", e.Error()
	}
	return "shared_mailbox_readers_differ", e.Error()
}

func (e *SharedReconnectError) Unwrap() error { return apperrors.ErrConflict }

// ConnectShared connects a shared mailbox, or reconnects the one already
// stored for this provider and address. Only a signed-in admin may. A
// provider with its own shared grant probes it first. The credential is
// sealed in the vault, and a refusal deletes it again.
func (r *Registry) ConnectShared(ctx context.Context, in SharedConnect) (ids.UUID, error) {
	c, err := r.connector(in.Provider)
	if err != nil {
		return ids.Nil, err
	}
	actor, err := RequireSharedMailboxAdmin(ctx)
	if err != nil {
		return ids.Nil, err
	}
	if in.Posture != "" {
		if err := refuseSharedVisibility(in.Posture); err != nil {
			return ids.Nil, err
		}
	}
	scopes, err := grantedScopes(c, actor, in.Provider)
	if err != nil {
		return ids.Nil, err
	}
	if in.Auth, err = probeShared(ctx, c, in.Auth); err != nil {
		return ids.Nil, fmt.Errorf("capture: probing the shared mailbox: %w", err)
	}
	ws, err := r.vaultWorkspace(ctx)
	if err != nil {
		return ids.Nil, err
	}
	ref, err := r.vault.Put(ctx, ids.From[ids.WorkspaceKind](ws), []byte(in.Auth))
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: sealing the shared mailbox credential: %w", err)
	}
	row := sharedUpsert{SharedConnect: in, address: bareAddress(in.Address), by: actor.UserID, scopes: scopes, ref: ref}
	var id ids.UUID
	var priorRef *string
	if err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		id, priorRef, err = upsertSharedConnectionTx(ctx, tx, row)
		return err
	}); err != nil {
		// Any failure after the seal deletes it, whether the write was refused
		// or the transaction never began. A commit that failed yet landed then
		// names a deleted ref, and the next sync asks for a reconnect.
		keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws, ref, "failed shared connect")
		return ids.Nil, fmt.Errorf("capture: storing the shared mailbox: %w", err)
	}
	if priorRef != nil {
		keyvault.DeleteDetached(ctx, r.vault, slog.Default(), ws, keyvault.Ref(*priorRef), "shared reconnect")
	}
	return id, nil
}

// vaultWorkspace is the workspace a credential is sealed under, once a vault
// is configured to seal it.
func (r *Registry) vaultWorkspace(ctx context.Context) (ids.UUID, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.Nil, errors.New("capture: a shared mailbox connect outside workspace context")
	}
	if r.vault == nil {
		return ids.Nil, errors.New("capture: no keyvault configured, so a mailbox credential cannot be sealed")
	}
	return ws, nil
}

type sharedUpsert struct {
	SharedConnect
	address string
	by      ids.UUID
	scopes  []string
	ref     keyvault.Ref
}

// sharedPrior is the row a reconnect finds.
type sharedPrior struct {
	id      ids.UUID
	ref     *string
	status  string
	readers ids.UUID
	posture string
	backup  *ids.UUID
}

// upsertSharedConnectionTx writes the shared row, or points the stored one at
// the fresh credential. A reconnect raises the generation, so a cycle still out
// under the old credential commits nothing.
func upsertSharedConnectionTx(ctx context.Context, tx pgx.Tx, in sharedUpsert) (ids.UUID, *string, error) {
	var prior sharedPrior
	err := tx.QueryRow(ctx, `
		SELECT id, credential_ref, status, reader_team_id, mail_posture, backup_owner_id FROM capture_connection
		 WHERE user_id IS NULL AND provider = $1 AND lower(account_label) = lower($2)
		   FOR UPDATE`, in.Provider, in.address).Scan(&prior.id, &prior.ref, &prior.status, &prior.readers, &prior.posture, &prior.backup)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil, err
	}
	posture, err := sharedConnectPosture(in, prior, found)
	if err != nil {
		return ids.Nil, nil, err
	}
	if err := refuseUnreadableTeamTx(ctx, tx, in.ReaderTeam); err != nil {
		return ids.Nil, nil, err
	}
	if err := refuseSharedWithoutOptInTx(ctx, tx, posture); err != nil {
		return ids.Nil, nil, err
	}
	backup := in.BackupOwner
	if backup == nil {
		backup = prior.backup
	}
	if err := refuseBackupOutsideTeamTx(ctx, tx, in.ReaderTeam, in.BackupOwner); err != nil {
		return ids.Nil, nil, err
	}
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO capture_connection (provider, user_id, connected_by, reader_team_id, account_label, mail_posture,
		                                scopes, credential_ref, status, share_acknowledged_at, backup_owner_id)
		VALUES (@provider, NULL, @by, @readers, @address, @visibility, @scopes, @ref, 'connected', now(), @backup)
		ON CONFLICT (provider, lower(account_label)) WHERE user_id IS NULL
		DO UPDATE SET credential_ref = EXCLUDED.credential_ref, auth = NULL, status = 'connected', archived_at = NULL,
		              connected_at = now(), connected_by = EXCLUDED.connected_by, scopes = EXCLUDED.scopes,
		              backup_owner_id = EXCLUDED.backup_owner_id, generation = capture_connection.generation + 1
		RETURNING id`, pgx.StrictNamedArgs{
		sqlArgProvider: in.Provider, "by": in.by, "readers": in.ReaderTeam, "address": in.address,
		"visibility": posture, "scopes": in.scopes, "ref": string(in.ref), "backup": backup,
	}).Scan(&id); err != nil {
		return ids.Nil, nil, err
	}
	database.Forget(tx, ownerIdentitiesMemo)
	view := SharedMailboxView{Provider: in.Provider, Address: in.address, Status: "connected", ReaderTeam: in.ReaderTeam, Posture: posture, BackupOwner: backup}
	after := sharedMailboxImage(view)
	verb, change, before := auditVerbCreate, sharedChangeConnected, map[string]any(nil)
	if found {
		verb, change = auditVerbUpdate, sharedChangeReconnected
		view.Status, view.ReaderTeam, view.Posture, view.BackupOwner = prior.status, prior.readers, prior.posture, prior.backup
		before = sharedMailboxImage(view)
	}
	if err := announceSharedMailboxTx(ctx, tx, verb, change, id, before, after); err != nil {
		return ids.Nil, nil, err
	}
	return id, prior.ref, resetConnectionSchedule(ctx, tx, id, false)
}

// sharedConnectPosture is the visibility a connect writes. A reconnect keeps
// the stored readers and visibility, and refuses a request naming others.
func sharedConnectPosture(in sharedUpsert, prior sharedPrior, found bool) (string, error) {
	if !found {
		if in.Posture == "" {
			return PostureHeld, nil
		}
		return in.Posture, nil
	}
	if prior.readers != in.ReaderTeam {
		return "", &SharedReconnectError{}
	}
	if in.Posture != "" && in.Posture != prior.posture {
		return "", &SharedReconnectError{visibility: true}
	}
	return prior.posture, nil
}
