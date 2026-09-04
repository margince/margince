// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The one-time relocation of connector credentials off the legacy
// capture_connection.auth bytea column and into the keyvault, run on every
// boot until the column can be dropped.
//
// It is a raw relocation of bytes already in the tenant's own store, not a
// domain mutation: no record changes meaning, so it files no audit row and no
// event. That posture stays, and it is why "who set this credential" is still
// answered by the connect row that set it.
//
// What it DOES file is a system_log entry, because the posture was being read
// as "and therefore nothing is recorded" — and it left the one path that
// repoints a live connection at new ciphertext as the only writer of
// credential_ref with no trace of when it happened. Every sibling records
// something: connect and reconnect, channelconn, integrations, overlay and the
// two settings seals all write audit rows, and extsecrets writes system_log for
// exactly this shape. An operator debugging a connection that broke across a
// deploy had no ledger to read. margince/margince#2552.
//
// system_log rather than audit_log, matching extsecrets: a timestamped record
// that bytes moved, without filing a domain act nobody performed.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CredentialBackfillActor names this pass on its ledger rows. A system id
// rather than a person, for the reason the rows exist: an operator reading the
// trail has to be able to tell bytes the BOOT relocated from a credential a
// colleague connected, and those are the two things that write this column.
const CredentialBackfillActor = "system:capture-credential-backfill"

// detailConnectionID is the ledger key naming which connection moved. A
// constant because three writers in this package spell it — the rotation
// ledger, the sync-state log and this one — and an operator correlating them
// matches on it.
const detailConnectionID = "connection_id"

// actionCredentialRelocated is the system_log action this pass files. Named as
// a constant because the ledger is read by matching on it, and a literal at the
// one call site is a string a reader has to find twice to trust.
const actionCredentialRelocated = "capture_credential_relocated"

// BackfillCredentials migrates every legacy capture_connection row whose
// credential still lives in the auth bytea column onto the vault: it seals the
// bytes, records the credential_ref, and clears auth. It is idempotent — a row
// that already carries a ref is skipped — so a re-run or a crash-retry is
// safe, which is what lets it run on every boot. It walks every live workspace
// under that workspace's own GUC, which is what capture_connection's own
// workspace predicates read.
// One workspace's failure must not starve the rest of the fleet (the same
// invariant retention and the close-date sweep hold): the walk continues past
// a failing workspace and returns the count migrated plus the joined errors.
func (r *Registry) BackfillCredentials(ctx context.Context) (int, error) {
	if r.vault == nil {
		return 0, errors.New("capture: cannot backfill connector credentials without a keyvault")
	}
	// rls-exempt: fleet enumeration — the workspace table is not workspace-scoped; this reads every tenant before entering each workspace's own GUC.
	rows, err := r.db.Pool().Query(ctx, `SELECT id FROM workspace WHERE archived_at IS NULL ORDER BY created_at`)
	if err != nil {
		return 0, fmt.Errorf("capture: listing workspaces for credential backfill: %w", err)
	}
	workspaces, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	total := 0
	var errs error
	for _, wsID := range workspaces {
		// The pass names ITSELF as the actor. Nobody asked for this — it is
		// the boot noticing a legacy row — so binding a human would put
		// somebody's name on a move they did not make, and the ledger's whole
		// value is telling a relocation apart from a connect.
		//
		// A bound actor is also storekit.LogSystem's precondition: it refuses
		// rather than guessing, which is what made this the one credential_ref
		// writer with no ledger row until the actor arrived with it.
		//
		// The correlation id groups one boot's relocations as the single pass
		// they are, exactly as the expiry sweep's does for one tick.
		wsCtx := principal.WithWorkspaceID(ctx, wsID)
		wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
		wsCtx = principal.WithActor(wsCtx, principal.Principal{
			Type: principal.PrincipalSystem, ID: CredentialBackfillActor,
		})
		migrated, err := r.backfillWorkspace(wsCtx, ids.From[ids.WorkspaceKind](wsID))
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("capture: backfilling workspace %s: %w", wsID, err))
			continue
		}
		total += migrated
	}
	return total, errs
}

// backfillWorkspace migrates one workspace's legacy rows. Each secret is
// sealed OUTSIDE the update tx (put-then-commit); the update then claims the
// row only if it still has no ref, so a concurrent backfill (two worker pods
// at boot) cannot double-migrate — the loser's sealed secret is a harmless
// orphan, never a corrupted row.
func (r *Registry) backfillWorkspace(ctx context.Context, ws ids.WorkspaceID) (int, error) {
	type legacyRow struct {
		id   ids.UUID
		auth []byte
	}
	var pending []legacyRow
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, auth FROM capture_connection
			WHERE credential_ref IS NULL AND auth IS NOT NULL`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l legacyRow
			if err := rows.Scan(&l.id, &l.auth); err != nil {
				return err
			}
			pending = append(pending, l)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}

	migrated := 0
	for _, l := range pending {
		ref, err := r.vault.Put(ctx, ws, l.auth)
		if err != nil {
			return migrated, err
		}
		var claimed bool
		err = r.db.Tx(ctx, func(tx pgx.Tx) error {
			ct, err := tx.Exec(ctx, `
				UPDATE capture_connection SET credential_ref = $2, auth = NULL
				WHERE id = $1 AND credential_ref IS NULL`, l.id, string(ref))
			if err != nil {
				return err
			}
			claimed = ct.RowsAffected() == 1
			if !claimed {
				// Another boot won the row: the UPDATE carries `AND
				// credential_ref IS NULL` and matched nothing. Nothing moved
				// here, so nothing is recorded — a line per losing racer would
				// make the ledger count boots rather than relocations.
				//
				// NOT PROVEN BY A TEST, and worth saying so rather than
				// implying otherwise. TestTwoBootsRelocateOneCredentialOnce
				// runs two passes concurrently and holds the pair to one
				// relocation and one row, which is the claim that matters — but
				// it cannot force the interleaving that reaches THIS branch,
				// because the window is between the work-list read and the
				// claim, and forcing it would mean a hook in this function
				// whose only caller is a test. So the branch is reasoned from
				// the UPDATE's own predicate rather than demonstrated.
				return nil
			}
			// INSIDE the same transaction as the UPDATE, so a recorded
			// relocation is one that actually committed — the reason
			// extsecrets logs inside its caller's transaction too. The detail
			// names the connection and the workspace and never the material:
			// the ciphertext ref is the thing an operator needs to correlate
			// with the vault, and the bytes are what must not appear in a
			// ledger anybody can read.
			_, err = storekit.LogSystem(ctx, tx, actionCredentialRelocated, map[string]any{
				detailConnectionID: l.id.String(),
				"workspace_id":     ws.String(),
				"credential_ref":   string(ref),
			})
			return err
		})
		if err != nil {
			return migrated, err
		}
		if claimed {
			migrated++
		}
	}
	return migrated, nil
}
