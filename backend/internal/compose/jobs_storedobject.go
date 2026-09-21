// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// River wiring for the orphaned-object reap: a fleet-wide pass that deletes the
// bytes of every stored object whose row never arrived.
//
// Storing a file is two writes to two systems that cannot be made atomic, and
// both writers put the bytes first. When the transaction after the put fails,
// the object stays and nothing references it — which matters past the wasted
// storage, because an Art. 17 erasure finds an attachment's bytes by reading
// storage_key off its row. An object with no row is one the erasure cannot
// reach or even enumerate by owner.
//
// The ledger (activities/storedobjectintent.go) is what makes the delete safe:
// this pass can only ever reach a key a writer declared provisional, so it
// cannot walk a bucket and decide for itself what is unreferenced.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StoredObjectReapArgs schedules one pass over the provisional-object ledger.
type StoredObjectReapArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (StoredObjectReapArgs) Kind() string { return "stored_object_reap" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace and walks them itself (jobs.FleetWide, ADR-0103).
func (StoredObjectReapArgs) FleetWide() {}

// storedObjectReapBatch bounds one workspace's pass.
//
// A pass that could run unbounded over a large backlog holds a worker for as
// long as the backlog is deep, and the cadence is hourly — so a capped pass
// that leaves work behind is picked up an hour later rather than never. The
// oldest keys go first, so a capped pass still makes progress on the ones that
// have waited longest.
const storedObjectReapBatch = 500

// storedObjectReapWorker deletes the bytes of provisional objects past the
// grace period, in every workspace.
//
// Archived workspaces included: archiving a workspace does not un-store the
// objects inside it, and an orphan there is exactly as unreachable to an
// erasure as one anywhere else.
type storedObjectReapWorker struct {
	pool *pgxpool.Pool
	blob blobstore.Store
	log  *slog.Logger
	now  func() time.Time
}

func (w *storedObjectReapWorker) Work(ctx context.Context, _ *river.Job[StoredObjectReapArgs]) error {
	return jobs.FaultContext(ctx, runPerEveryWorkspace(ctx, w.pool, w.reapWorkspace))
}

func (w *storedObjectReapWorker) reapWorkspace(ctx context.Context, workspace ids.UUID) error {
	// The system principal, bound here rather than inherited: a queue carries
	// no actor, and every read and write below is RBAC-gated. The pass is the
	// installation's own bookkeeping, not any seat's.
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "stored_object_reap_worker")
	store := activities.NewStore(InstallationDB(w.pool))
	before := w.now().UTC().Add(-activities.ProvisionalObjectGrace)
	orphans, err := store.ListOrphanedObjects(ctx, before, storedObjectReapBatch)
	if err != nil {
		return err
	}
	for _, orphan := range orphans {
		// The BYTES first, then the ledger row. The other order would forget an
		// object that is still there, which is the state this whole ledger
		// exists to make impossible — a failed delete leaves the key and the
		// next pass tries again.
		if err := w.blob.Delete(ctx, orphan.StorageKey); err != nil {
			// One key that will not delete must not stop the rest: a bucket
			// that refuses one object still holds the others, and every one of
			// them is a subject's file an erasure cannot reach. The key stays
			// in the ledger, so nothing is forgotten.
			w.log.ErrorContext(ctx, "an orphaned object could not be deleted",
				"workspace", workspace.String(), "err", err)
			continue
		}
		if err := store.RetireOrphanedObject(ctx, orphan.StorageKey); err != nil {
			return err
		}
	}
	return nil
}

// storedObjectReaperFor answers nil for a role that composed no object store.
//
// Nil is the honest answer rather than a degraded one, the same shape the
// capture purger takes: a pass that cannot reach the bytes would retire ledger
// rows for objects that are still there, and that turns a recoverable orphan
// into one nothing can find.
func storedObjectReaperFor(pool *pgxpool.Pool, blob blobstore.Store, log *slog.Logger) *storedObjectReapWorker {
	if blob == nil {
		return nil
	}
	return &storedObjectReapWorker{pool: pool, blob: blob, log: log, now: time.Now}
}
