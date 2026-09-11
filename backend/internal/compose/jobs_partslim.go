// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// River wiring for the raw-capture part sweep: one worker draining a bounded
// batch per run. It registers itself — worker and schedule together — so
// jobs.go grows one line as this surface does.
//
// The cadence is the high-water mark of the duplication it removes. Capture
// writes the provider original with the attachment's octets still in it, and
// this takes them back out once the object store can vouch for them, so a
// backfill running at a gigabyte an hour leaves at most one cadence of that on
// disk.
//
// It registers only where an object store is configured. Without one the sweep
// can prove nothing, and a pass that read the table anyway would either stamp
// rows it proved nothing about — consuming the backlog a later-wired store
// would have worked — or read the whole table every cadence to no effect.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// capturePartSlimQueue keeps the sweep off the default queue: a batch is one
// object read per part plus an encode of those octets, so a full batch would
// hold a default worker for as long as the store takes to answer.
const capturePartSlimQueue = "capture_part_slim"

// capturePartSlimMaxWorkers is one. The sweep is bounded by the object store
// and by the pool every other job shares, so a second buys contention rather
// than throughput — and the backlog drains at a batch per cadence either way.
const capturePartSlimMaxWorkers = 1

// addCapturePartSlimJobs registers the worker and returns its schedule.
//
// The store is built here rather than handed in for the reason the retention
// pass's is: its object store is the runner's own, and the sweep's whole
// contract is that it can prove the bytes are durable before it removes them.
func addCapturePartSlimJobs(
	reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger,
) []*river.PeriodicJob {
	// The WORKER, not only the schedule. periodicFor already suppresses the
	// schedule without a store, but a registered worker still subscribes to the
	// queue — so on a fleet where one runner has the store and another does not,
	// the storeless runner can claim the job, do nothing, and complete it. The
	// rows are not lost (nothing is stamped without proof) but the pass makes no
	// progress whenever that runner wins the claim. api/jobs.yaml declares
	// `absent: registers_nothing`, and this is what makes that true.
	if cfg.Blobstore == nil {
		return nil
	}
	slim := func(db *database.DB) *capture.PartSlimStore {
		return capture.NewPartSlimStore(db, cfg.Blobstore)
	}
	addDeclaredWorker[CapturePartSlimArgs](reg, &capturePartSlimWorker{
		pool: pool, slim: slim, log: log,
	})
	return periodicFor(cfg, CapturePartSlimArgs{})
}

// CapturePartSlimArgs drains one batch of unconsidered provider originals.
type CapturePartSlimArgs struct{}

// Kind names the job in the census.
func (CapturePartSlimArgs) Kind() string { return "capture_part_slim" }

type capturePartSlimWorker struct {
	river.WorkerDefaults[CapturePartSlimArgs]
	pool *pgxpool.Pool
	slim func(*database.DB) *capture.PartSlimStore
	log  *slog.Logger
}

// partSlimJobActor binds the principal the sweep's writes run under.
//
// A queue carries no principal to inherit, so a worker reaching a store has to
// name one or its first gated write is refused for having no actor. The sweep's
// writes are not gated today — it touches raw_capture directly, which capture
// owns — but the actor is bound anyway: the day one of them grows a gate, the
// failure would be a refused write inside a background pass, which is the
// hardest kind to notice.
func partSlimJobActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "capture_part_slim_worker",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// Work drains one batch.
//
// The counts are logged rather than kept silent because this is the only place
// an operator can watch the backlog drain, and "considered but not slimmed" is
// the number that distinguishes a store that is not answering from a corpus
// whose encodings simply cannot be located.
func (w *capturePartSlimWorker) Work(ctx context.Context, _ *river.Job[CapturePartSlimArgs]) error {
	ctx = partSlimJobActor(ctx)
	got, err := w.slim(InstallationDB(w.pool)).SlimBatch(ctx, capture.PartSlimBatch)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("capture_part_slim: slimming a batch: %w", err))
	}
	if got.Considered == 0 {
		// The backlog is drained. Said at debug so a quiet installation does
		// not print a line every cadence forever.
		w.log.DebugContext(ctx, "raw capture part sweep found nothing to consider")
		return nil
	}
	w.log.InfoContext(ctx, "raw capture part sweep",
		"considered", got.Considered, "slimmed", got.Slimmed,
		"unproved_parts", got.Unproved, "payload_bytes_freed", got.Freed)
	return nil
}
