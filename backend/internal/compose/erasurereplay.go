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

// ReplayErasureSuppressions re-erases each live contact the suppression list
// or its exported journal names, and reports how many it took. A contact the
// eraser refuses, such as one under legal hold, does not stop the others. The
// refusals come back beside the count of what was done.
func ReplayErasureSuppressions(
	ctx context.Context, pool *pgxpool.Pool, blob blobstore.Store, vault keyvault.Vault,
) (int, error) {
	if blob == nil {
		return 0, ErrReplayNeedsObjectStore
	}
	workspace, err := InstallationDB(pool).Workspace(ctx)
	if err != nil {
		return 0, fmt.Errorf("replaying erasures: %w", err)
	}
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace.UUID), erasureReplayActor)
	return ComposedEraser(pool, blob, vault).ReapplySuppressions(ctx)
}
