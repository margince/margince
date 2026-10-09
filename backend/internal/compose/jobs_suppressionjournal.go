// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Keeping the suppression journal in the object store.
//
// An erasure writes a suppression entry so the subject cannot be re-imported.
// That entry lives in the database, and a restore to a point before the
// erasure brings the subject back with nothing left to refuse them.
//
// privacy.ExportSuppressions copies the list to the object store, where a
// restore cannot reach it, and its own comment asks to be run on a schedule.
// Nothing ran it, so the journal a restore would need was never written.
//
// The export carries the identifier's kind and the date, never the identifier.
// An export carrying the address would reconstitute what the erasure
// destroyed.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// suppressionJournalActor is the system principal this pass acts as.
//
// A constant, because the frontend-label census reads the argument at the call
// site. A principal it cannot see is one History cannot name.
const suppressionJournalActor = "suppression_journal_worker"

// SuppressionJournalArgs exports the suppression list to the object store.
type SuppressionJournalArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (SuppressionJournalArgs) Kind() string { return "suppression_journal_export" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace and walks them itself (jobs.FleetWide, ADR-0103).
func (SuppressionJournalArgs) FleetWide() {}

// suppressionJournalExporterFor answers nil for a role that composed no object
// store.
//
// The journal sits where a database restore cannot reach it, so without a store
// there is nowhere to write it. The schedule is withheld separately by the
// kind's own declaration, which names the same dependency.
func suppressionJournalExporterFor(
	pool *pgxpool.Pool, blob blobstore.Store, log *slog.Logger,
) *suppressionJournalWorker {
	if blob == nil {
		return nil
	}
	return &suppressionJournalWorker{pool: pool, blob: blob, log: log}
}

// addSuppressionJournalJobs registers the export worker when there is somewhere
// to write to.
func addSuppressionJournalJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) {
	exporter := suppressionJournalExporterFor(pool, cfg.Blobstore, log)
	if exporter == nil {
		return
	}
	addDeclaredWorker[SuppressionJournalArgs](reg, exporter)
}

// suppressionJournalWorker writes one installation's journal.
type suppressionJournalWorker struct {
	pool *pgxpool.Pool
	blob blobstore.Store
	log  *slog.Logger
}

func (w *suppressionJournalWorker) Work(ctx context.Context, _ *river.Job[SuppressionJournalArgs]) error {
	return jobs.FaultContext(ctx, runPerEveryWorkspace(ctx, w.pool, w.exportWorkspace))
}

func (w *suppressionJournalWorker) exportWorkspace(ctx context.Context, workspace ids.UUID) error {
	// The system principal, bound here rather than inherited: a queue carries
	// no actor, and ExportSuppressions is gated on the contact delete grant.
	ctx = principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), suppressionJournalActor)
	eraser := privacy.NewEraser(InstallationDB(w.pool)).WithBlobstore(w.blob)
	written, err := eraser.ExportSuppressions(ctx)
	if err != nil {
		return err
	}
	// Logged rather than counted into a metric: the number only matters against
	// the list's own size.
	//
	// A journal that stops growing while erasures continue is the failure this
	// job exists to prevent.
	w.log.InfoContext(ctx, "suppression journal exported", "entries", written)
	return nil
}
