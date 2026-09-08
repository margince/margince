// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Redeeming the sealed credentials a reset orphans.
//
// Split from datareset.go because it answers a question the rest of the reset
// does not: vault_secret carries no workspace_id — the tenant lives inside the
// ref and inside the AES-256-GCM AAD — so it is operational infrastructure
// rather than a tenant table, and the sweep never sees it. Everything here
// exists because that one table cannot be swept, and has to be reasoned about
// on its own terms.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// purgeSealedCredentials redeems the credential handles the sweep collected
// before it deleted the rows naming them.
//
// It runs after the commit because the vault is a seam, not a table: the local
// provider happens to write Postgres, but a remote one has no transaction to
// join. The handles were captured inside the transaction instead, which is the
// half that has to be consistent — a sweep that rolled back leaves refs this
// never receives.
//
// A failure fails the request. The alternative is reporting an installation as
// reset while its sealed credentials are still resident, which is precisely
// the state this exists to prevent. Delete is idempotent, so re-running the
// reset finishes a partial purge.
func (h dataResetHandlers) purgeSealedCredentials(ctx context.Context, wsID ids.UUID, counts *resetCounts) error {
	if h.vault == nil {
		return nil
	}
	ws := ids.From[ids.WorkspaceKind](wsID)
	refs, err := h.sealedRefsToPurge(ctx, counts)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		// The ref is never logged or returned: it is the address of a secret,
		// and an error naming it would put that address in every log sink.
		if err := h.vault.Delete(ctx, ws, keyvault.Ref(ref)); err != nil {
			return fmt.Errorf("data reset: purging a sealed credential: %w", err)
		}
		counts.SecretsPurged++
	}
	return nil
}

// sealedRefsToPurge is what the vault purge deletes: the handles the sweep saw,
// plus the ones it could not.
//
// The snapshot half is unchanged and still necessary — those refs' rows are
// gone, so nothing can find them by joining any more. The second half asks the
// post-commit invariant (orphanedSecretRefs): a vault_secret row no live handle
// column names is unreachable however it got there, which is what catches a
// write that repointed a handle after the sweep read it and before the sweep
// deleted it.
//
// Deduplicated, because the two overlap by design: a ref in the snapshot is
// also orphaned once the row naming it is gone, and Delete is idempotent but
// SecretsPurged is a count somebody reads.
//
// A failure to enumerate the orphans FAILS the purge rather than falling back
// to the snapshot alone. Falling back would report a clean installation on the
// exact reading that could not be taken, which is the shape of every silent
// half-purge this endpoint has to avoid.
func (h dataResetHandlers) sealedRefsToPurge(ctx context.Context, counts *resetCounts) ([]string, error) {
	seen := make(map[string]bool, len(counts.secretRefs))
	refs := make([]string, 0, len(counts.secretRefs))
	for _, ref := range counts.secretRefs {
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	var orphans []string
	if err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		orphans, err = orphanedSecretRefs(ctx, tx)
		return err
	}); err != nil {
		return nil, err
	}
	for _, ref := range orphans {
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
