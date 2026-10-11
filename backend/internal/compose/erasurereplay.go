// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Re-erasing what a restore brought back.
//
// An operator runs this from a shell after restoring a database, before users
// return. It reads the suppression list and the journal the export job wrote to
// the object store, and erases every live contact either one names.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// erasureReplayActor is the system principal the replay acts as. A constant
// because the frontend-label census reads the argument at the call site.
const erasureReplayActor = "erasure_replay"

// ErrReplayNeedsObjectStore refuses a replay with no object store, where the
// journal lives. After a restore to a point before an erasure, the database's
// own list no longer names the subject. A run without the journal would report
// a clean pass it cannot vouch for.
var ErrReplayNeedsObjectStore = errors.New(
	"the erasure replay needs the object store: set MARGINCE_BLOBSTORE_ENDPOINT or MARGINCE_BLOBSTORE_PATH as the api has them")

// ErrReplayJournalEmpty refuses a replay whose journal holds no entry. That
// means a store pointed at the wrong place or an export that never ran. Only
// an installation that has erased nobody has a right to one.
var ErrReplayJournalEmpty = errors.New(
	"the erasure journal in the object store holds no entry: check that the object store settings point at the restored " +
		"store and that it holds the erasure-suppression/ prefix, or pass --empty-journal-ok if this installation has never erased anyone")

// ErasureReplay is what one replay did and what it could rely on.
type ErasureReplay struct {
	Taken int
	// Journal is the export the replay read. An erasure after
	// Journal.ExportedAt is neither in it nor, after a restore, in the
	// database, so the replay cannot cover it.
	Journal privacy.JournalManifest
	// JournalEmpty is set when the operator let the replay run on a journal
	// with no entry.
	JournalEmpty bool
}

// ReplayErasureSuppressions re-erases each live contact the suppression list
// or its exported journal names. A contact the eraser refuses, such as one
// under legal hold, does not stop the others. The refusals come back beside
// the count of what was done.
func ReplayErasureSuppressions(
	ctx context.Context, pool *pgxpool.Pool, blob blobstore.Store, vault keyvault.Vault, emptyJournalOK bool,
) (ErasureReplay, error) {
	if blob == nil {
		return ErasureReplay{}, ErrReplayNeedsObjectStore
	}
	workspace, err := InstallationDB(pool).Workspace(ctx)
	if err != nil {
		return ErasureReplay{}, fmt.Errorf("replaying erasures: %w", err)
	}
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace.UUID), erasureReplayActor)
	eraser := ComposedEraser(pool, blob, vault)

	var replay ErasureReplay
	replay.Journal, err = eraser.ReadJournalManifest(ctx)
	switch {
	case errors.Is(err, privacy.ErrNoJournalManifest) || (err == nil && replay.Journal.Entries == 0):
		if !emptyJournalOK {
			return replay, ErrReplayJournalEmpty
		}
		replay.JournalEmpty = true
	case err != nil:
		return replay, fmt.Errorf("replaying erasures: %w", err)
	}
	replay.Taken, err = eraser.ReapplySuppressions(ctx)
	return replay, err
}
