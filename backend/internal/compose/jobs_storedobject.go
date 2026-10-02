// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// River wiring for the orphaned-object reap: one installation-wide pass that
// deletes the bytes of every stored object whose row never arrived.
//
// Storing a file is two writes to two systems that cannot be made atomic, and
// every writer puts the bytes first. When the transaction after the put fails,
// the object stays and nothing references it — which matters past the wasted
// storage, because an Art. 17 erasure finds an object by reading its key off
// the row. An object with no row is one the erasure cannot reach or even
// enumerate by owner.
//
// The ledger (platform/storedobject) is what makes the delete safe: this pass
// can only ever reach a key a writer declared provisional, of a kind a module
// declared its referencing columns for, so it cannot walk a bucket and decide
// for itself what is unreferenced.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/storedobject"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StoredObjectReapArgs schedules one pass over the provisional-object ledger.
type StoredObjectReapArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (StoredObjectReapArgs) Kind() string { return "stored_object_reap" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace (jobs.FleetWide, ADR-0103).
func (StoredObjectReapArgs) FleetWide() {}

// storedObjectReapBatch bounds one pass.
//
// A pass that could run unbounded over a large backlog holds a worker for as
// long as the backlog is deep, and the cadence is hourly — so a capped pass
// that leaves work behind is picked up an hour later rather than never. The
// longest-waiting keys go first, and a key whose delete failed waits again
// from that attempt, so a capped pass still reaches the ones behind it.
const storedObjectReapBatch = 500

// StoredObjectReferences is every module's declaration of where it records the
// keys of its stored objects. A kind missing here is never reaped, which keeps
// its orphans rather than deleting live bytes. Exported for the schema census
// that holds every key-shaped column to one of these declarations.
//
// Held by: TestEveryModulesStoredObjectReferenceIsReaped (backend/internal/compose/jobs_storedobject_test.go)
// and TestEveryStoredObjectKeyColumnIsDeclaredToTheReap (backend/gates/storedobjectcolumns_test.go)
func StoredObjectReferences() []storedobject.Reference {
	return []storedobject.Reference{
		activities.StoredObjectReference(),
		contacts.StoredObjectReference(),
		deals.StoredObjectReference(),
		knowledge.StoredObjectReference(),
		migration.StoredObjectReference(),
	}
}

// storedObjectReapWorker deletes the bytes of provisional objects past their
// kind's grace period.
//
// ONE pass for the installation: the ledger carries no workspace column — the
// key's own prefix names it — so the query has no tenant to scope by. Archived
// workspaces' objects are reached like any other, since archiving un-stores
// nothing.
type storedObjectReapWorker struct {
	pool *pgxpool.Pool
	blob blobstore.Store
	log  *slog.Logger
	now  func() time.Time
}

func (w *storedObjectReapWorker) Work(ctx context.Context, _ *river.Job[StoredObjectReapArgs]) error {
	return jobs.FaultContext(ctx, w.reap(ctx))
}

func (w *storedObjectReapWorker) reap(ctx context.Context) error {
	// The system principal, bound here rather than inherited: a queue carries
	// no actor, and every read and write below is RBAC-gated. The pass is the
	// installation's own bookkeeping, not any seat's.
	ctx = principal.SystemActing(ctx, "stored_object_reap_worker")
	ledger, err := storedobject.NewLedger(InstallationDB(w.pool), StoredObjectReferences()...)
	if err != nil {
		return err
	}
	now := w.now().UTC()
	orphans, err := ledger.Orphans(ctx, now, storedObjectReapBatch)
	if err != nil {
		return err
	}
	for _, orphan := range orphans {
		// Condemned first, in its own short transaction, because the listing
		// is a snapshot and the delete holds no lock: a key claimed or named
		// since the listing is passed over here rather than deleted live.
		condemned, err := ledger.Condemn(ctx, now, orphan.StorageKey)
		if err != nil {
			return err
		}
		if !condemned {
			continue
		}
		// The BYTES next, then the ledger row. The other order would forget an
		// object that is still there; a failed delete leaves the key condemned
		// and the next pass tries again.
		if err := w.blob.Delete(ctx, orphan.StorageKey); err != nil {
			// One key that will not delete must not stop the rest: a bucket
			// that refuses one object still holds the others, and every one of
			// them is a subject's file an erasure cannot reach.
			w.log.ErrorContext(ctx, "an orphaned object could not be deleted",
				"storage_key", orphan.StorageKey, "err", err)
			continue
		}
		if err := ledger.Retire(ctx, orphan.StorageKey); err != nil {
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
