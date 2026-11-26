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

	"github.com/margince/margince/backend/internal/compose/storedobjectreap"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
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

// storedObjectReapBatch bounds one read of the ledger. A run takes batch after
// batch, oldest keys first, so the batch size does not cap how much one run
// deletes.
const storedObjectReapBatch = 500

// storedObjectReapDrainBudget bounds how long one run keeps taking batches,
// over every workspace. It is under the job's 5-minute timeout, which must also
// cover the batch still running when the budget is spent. It is checked
// between batches only.
const storedObjectReapDrainBudget = 4 * time.Minute

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
	// batch is storedObjectReapBatch outside tests.
	batch int
}

func (w *storedObjectReapWorker) Work(ctx context.Context, _ *river.Job[StoredObjectReapArgs]) error {
	stop := w.now().Add(storedObjectReapDrainBudget)
	return jobs.FaultContext(ctx, runPerEveryWorkspace(ctx, w.pool, func(ctx context.Context, workspace ids.UUID) error {
		return w.reapWorkspace(ctx, workspace, stop)
	}))
}

func (w *storedObjectReapWorker) reapWorkspace(ctx context.Context, workspace ids.UUID, stop time.Time) error {
	return storedobjectreap.Drain(ctx, func(ctx context.Context) (int, int, error) {
		return w.reapBatch(ctx, workspace)
	}, w.batch, stop, w.now)
}

// reapBatch answers how many keys it read and how many it settled: bytes
// deleted, or an intent retired because a row names the key.
func (w *storedObjectReapWorker) reapBatch(ctx context.Context, workspace ids.UUID) (listed, settled int, err error) {
	// The system principal, bound here rather than inherited: a queue carries
	// no actor, and every read and write below is RBAC-gated. The pass is the
	// installation's own bookkeeping, not any seat's.
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "stored_object_reap_worker")
	db := InstallationDB(w.pool)
	ledger := storedobjects.NewLedger(db)
	before := w.now().UTC().Add(-storedobjects.ProvisionalObjectGrace)
	provisional, err := ledger.ListProvisional(ctx, before, w.batch)
	if err != nil {
		return 0, 0, err
	}
	// The fault from an unowned kind is held until the rest of the pass has run: the
	// orphans already adjudicated are still orphans, and a kind nobody owns must not
	// keep every other kind's bytes alive.
	verdict, adjudicateErr := adjudicate(ctx, db, provisional)
	pendingByKey := make(map[string]storedobjects.Provisional, len(provisional))
	for _, pending := range provisional {
		pendingByKey[pending.Key] = pending
	}
	// A key an owning row DOES carry is a clear that never ran. Its bytes stay and
	// its intent goes, so it stops occupying a slot in every later pass's limit.
	for _, adopted := range verdict.Adopted {
		if err := ledger.Retire(ctx, pendingByKey[adopted]); err != nil {
			return len(provisional), settled, err
		}
		settled++
	}
	for _, orphan := range verdict.Orphans {
		done, err := w.reapOrphan(ctx, ledger, db, pendingByKey[orphan], before, workspace)
		if err != nil {
			return len(provisional), settled, err
		}
		if done {
			settled++
		}
	}
	return len(provisional), settled, adjudicateErr
}

// reapOrphan answers whether it settled the key.
func (w *storedObjectReapWorker) reapOrphan(ctx context.Context, ledger *storedobjects.Ledger, db *database.DB, pending storedobjects.Provisional, before time.Time, workspace ids.UUID) (bool, error) {
	claimed, err := ledger.Condemn(ctx, pending.Key, before, w.now())
	if err != nil || !claimed {
		return false, err
	}
	current, err := adjudicate(ctx, db, []storedobjects.Provisional{pending})
	if err != nil {
		return false, err
	}
	if len(current.Adopted) != 0 {
		return true, ledger.Retire(ctx, pending)
	}
	orphan := pending.Key
	if err := w.blob.Delete(ctx, orphan); err != nil {
		// The claim stays retryable; one unavailable object must not stop the batch.
		w.log.ErrorContext(ctx, "an orphaned object could not be deleted",
			"workspace", workspace, "err", err)
		return false, nil
	}
	return true, ledger.CompleteReap(ctx, orphan, w.now())
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
	return &storedObjectReapWorker{pool: pool, blob: blob, log: log, now: time.Now, batch: storedObjectReapBatch}
}
