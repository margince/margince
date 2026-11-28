// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The context a boot-time ledger write runs under.
//
// A boot step that records an operational fact — which extensions this binary
// composed, which release it was published as — writes to system_log like every
// other ledger row, which means it needs the three things every ledger write
// needs: the workspace the row belongs to, an actor to attribute it to, and a
// correlation scope. No request supplied any of them, because no request caused
// the write, so the boot has to bind them itself.
//
// Spelled once because the pre-bootstrap case is the subtle half: an
// installation with no company yet has no workspace to record against, so
// there is nothing to write and nothing to compare — a boot step must treat that
// as "not yet", never as an error and never as an empty answer it then acts on.
// The integration lane covers that arm for both facts.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bootLedgerScope binds the actor and correlation scope a system_log write
// needs, answering whether the installation is bootstrapped at all.
//
// A false second return is NOT an error: it is a pre-bootstrap installation, and
// the caller decides what "not yet" means for its own fact. In that case the
// returned context is the input one. A caller that ignores the flag therefore
// writes against no company, and the transaction answers ErrNoWorkspace.
func bootLedgerScope(ctx context.Context, pool *pgxpool.Pool, actor string) (context.Context, bool, error) {
	_, err := identity.NewService(pool).InstallationWorkspace(ctx)
	if errors.Is(err, identity.ErrNotBootstrapped) {
		return ctx, false, nil
	}
	if err != nil {
		return ctx, false, err
	}
	ctx = principal.SystemActing(ctx, actor)
	return ctx, true, nil
}

// bootLedgerLock is the transaction-scoped advisory lock a boot observation takes
// before it reads the last one and decides whether to write. Every process role
// boots concurrently, and several api replicas boot at once during a rollout;
// without it each transaction reads the same previous observation and every one
// of them writes the change.
//
// ONE statement for both facts, parameterized by the fact's name. The workspace
// this key used to carry went with ADR-0091 §5: an installation serves one
// company (ADR-0061), so it distinguished nothing.
//
// It keeps a NAMESPACE prefix rather than hashing the bare fact name, because
// the workspace suffix was doing that job too. hashtext over an unqualified
// caller-chosen string shares one key space with every other such lock in this
// tree, and a collision there serializes two unrelated boot paths for the
// length of a transaction.
//
// It reads no GUC, so there is no unset-GUC case to guard: the argument is a
// compile-time literal joined to a caller-supplied fact name, and neither can be
// NULL. That matters because pg_advisory_xact_lock is STRICT — a NULL argument
// takes NO LOCK and returns NULL rather than raising, which is a guard reporting
// success while holding nothing. The workspace-qualified form needed a COALESCE
// to stay out of that state; with no GUC to read there is nothing to guard.
const bootLedgerLock = `
	SELECT pg_advisory_xact_lock(hashtext('margince:boot-ledger:' || $1)::bigint)`

// installationMarker identifies THIS installation inside a boot observation's
// detail payload.
//
// The ledgers lost their tenant column in ADR-0091 §8 phase D, and the boot
// facts are read back with "the newest row wins". That is right for an
// installation reading its own history and wrong for one carrying an archived
// predecessor's: the residue gate exempts the ledgers by name, because their
// immutability trigger makes clearing them impossible, so those rows are still
// there and the read can no longer tell them apart.
//
// So the observation says which installation made it. A row without the marker
// is not assumed to be ours — it predates this and may be a predecessor's — and
// the reader treats "no marked row" the same way it treats "nothing recorded":
// the comparison is disabled, which is the posture buildinfo already takes for
// an unstamped binary.
func installationMarker(ctx context.Context, tx pgx.Tx) (string, error) {
	ws, err := storekit.Workspace(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("compose: naming this installation in a boot observation: %w", err)
	}
	return ws.String(), nil
}
