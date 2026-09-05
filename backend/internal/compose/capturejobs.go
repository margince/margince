// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture pipeline's overnight River trio (ADR-0063): the catch-up
// classify pass (§2.8), the signature-enrich pass (§2.9), and the
// morning-digest build (CAP-DDL-6). Job args and worker adapters only —
// the engines they delegate to (CaptureClassifier, CaptureEnricher, the
// capture registry's digest builder) stay River-agnostic; NewJobRunner
// (jobs.go) registers these on the shared periodic schedule.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CaptureClassifyArgs runs one catch-up classify pass (ADR-0063; §2.8).
type CaptureClassifyArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureClassifyArgs) Kind() string { return "capture_classify" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureClassifyArgs) FleetWide() {}

// captureClassifyWorker drives the batched label engine for every live
// workspace; the engine commits per model call, so a mid-pass crash or budget
// stop loses nothing and the next tick resumes from the shrunken backlog.
//
// One worker where there were two (ADR-0103).
type captureClassifyWorker struct {
	pool       *pgxpool.Pool
	classifier *CaptureClassifier
}

func (w *captureClassifyWorker) Work(ctx context.Context, _ *river.Job[CaptureClassifyArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.classifyWorkspace))
}

func (w *captureClassifyWorker) classifyWorkspace(ctx context.Context, workspace ids.UUID) error {
	return w.classifier.RunWorkspace(principal.WithWorkspaceID(ctx, workspace), 0)
}

// CaptureEnrichArgs runs one signature-enrich pass (ADR-0063; §2.9).
type CaptureEnrichArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureEnrichArgs) Kind() string { return "capture_enrich" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureEnrichArgs) FleetWide() {}

// captureEnrichWorker drives the evidence-gated signature pass for every live
// workspace; every accepted field is auditable back to its verbatim signature
// line.
//
// One worker where there were two (ADR-0103).
type captureEnrichWorker struct {
	pool     *pgxpool.Pool
	enricher *CaptureEnricher
	log      *slog.Logger
}

func (w *captureEnrichWorker) Work(ctx context.Context, _ *river.Job[CaptureEnrichArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.enrichWorkspace))
}

func (w *captureEnrichWorker) enrichWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	filled, err := w.enricher.RunWorkspace(wsCtx)
	if err != nil {
		return err
	}
	if filled {
		w.enqueueContinuation(wsCtx, workspace)
	}
	return nil
}

// enqueueContinuation queues the next slice of a workspace whose pass filled
// its limit.
//
// Best-effort, like the backfill's digest above and for the same reason: the
// work is already done and committed, and the nightly pass still reconciles
// whatever this fails to queue. Failing the job instead would re-run a pass
// that has already spent its model budget on the candidates it handled.
//
// The insert carries NO uniqueness. This runs inside the pass it continues, so
// the ByArgs/active-state uniqueness the trigger uses would dedupe the
// continuation against its own still-running parent and drop it silently —
// which is precisely the case this exists to fix.
// It queues the PASS, which the collapse made the only kind there is: the
// continuation used to name one workspace and now re-runs the walk. On an
// installation with a single workspace — the only shape the product supports
// (identity.InstallationWorkspace refuses more) — that is the same work. On a
// fleet it would re-visit the workspaces that did not fill their limit, and
// each of those is a cheap no-op read against a backlog it just emptied.
func (w *captureEnrichWorker) enqueueContinuation(ctx context.Context, workspace ids.UUID) {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		w.log.WarnContext(ctx, "signature enrich: no River client in context, so the continuation was not enqueued",
			"workspace", workspace.String(), "err", err)
		return
	}
	if _, err := client.Insert(ctx, CaptureEnrichArgs{},
		oneOffPassOpts(CaptureEnrichArgs{}.Kind())); err != nil {
		w.log.WarnContext(ctx, "signature enrich: continuation enqueue failed",
			"workspace", workspace.String(), "err", err)
	}
}

// OrgNamePromotionArgs runs one org-name promotion pass (PO-F-2a).
type OrgNamePromotionArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (OrgNamePromotionArgs) Kind() string { return "org_name_promotion" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (OrgNamePromotionArgs) FleetWide() {}

// orgNamePromotionWorker promotes names for every live workspace.
//
// One worker where there were two (ADR-0103).
type orgNamePromotionWorker struct {
	pool     *pgxpool.Pool
	promoter *OrgNamePromoter
}

func (w *orgNamePromotionWorker) Work(ctx context.Context, _ *river.Job[OrgNamePromotionArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.promoteWorkspace))
}

// orgNamePromotionWorkspaceWorker runs one workspace's pass: a database-only
// walk over the org_name evidence the enrich job collects.
func (w *orgNamePromotionWorker) promoteWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	return jobs.FaultContext(ctx, w.promoter.RunWorkspace(wsCtx, workspace))
}

// CaptureDigestArgs builds the morning digests (CAP-DDL-6; the nightly
// suite's last pass).
type CaptureDigestArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureDigestArgs) Kind() string { return "capture_digest" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureDigestArgs) FleetWide() {}

// captureDigestWorker assembles one digest per connected user per
// workspace; a re-run replaces the day's payload (as-of-now truths).
type captureDigestWorker struct {
	registry *capture.Registry
	pool     *pgxpool.Pool
	log      *slog.Logger
	// now is the injected clock (nil = wall clock). The digest day is
	// deliberately read at execution time, not enqueue time: the payload
	// is as-of-now truths and a re-run replaces the day, so a retry that
	// crosses midnight builds the morning actually being served.
	now func() time.Time
}

// Work fans out on the DEFAULT queue, not ai_capture, unlike its three
// siblings in this file: assembling a digest is a database-only pass — this
// worker holds no model lane at all — and ai_capture exists to keep long,
// model-bound work from evicting short jobs. Queueing the morning digest
// behind two model workers would delay it for no reason.
// The GRANULARITY returns to one row for the fleet (ADR-0103). A per-workspace
// child gave a retry that re-ran only the workspace that failed; the pass joins
// its failures again, as it did before that split, and a retry re-runs the
// walk. Assembling a digest for a workspace that already has today's is a
// re-build of the same day, which the payload's as-of-now truths make
// idempotent — that is why the split was affordable to undo.
func (w *captureDigestWorker) Work(ctx context.Context, _ *river.Job[CaptureDigestArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.digestWorkspace))
}

func (w *captureDigestWorker) digestWorkspace(ctx context.Context, workspace ids.UUID) error {
	clock := w.now
	if clock == nil {
		clock = time.Now
	}
	return w.registry.BuildDigests(principal.WithWorkspaceID(ctx, workspace), clock().UTC())
}

// CaptureBackfillArgs pages ONE bounded backfill run (ADR-0063). Unique by
// args while incomplete: start and any retry converge on one job.
type CaptureBackfillArgs struct {
	Workspace  ids.UUID `json:"workspace_id"`
	BackfillID string   `json:"backfill_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (CaptureBackfillArgs) Kind() string { return "capture_backfill" }

// WorkspaceID binds this backfill run to its tenant (jobs.WorkspaceScoped).
func (a CaptureBackfillArgs) WorkspaceID() ids.UUID { return a.Workspace }

// captureBackfillWorker pages a run to completion, yielding between pages
// (snooze) so a long mailbox never monopolizes a worker slot. A page error
// returns nil after the engine recorded it — the run row owns its state.
type captureBackfillWorker struct {
	registry *capture.Registry
	log      *slog.Logger
}

// backfillPagesPerTick bounds how many pages one Work invocation walks before
// yielding. ONE page: a page is up to 100 messages fetched serially, each a
// full RAW download of a real email (megabytes for a photo), so a page can
// take minutes on a live mailbox. Committing after every page and snoozing
// means each page runs under a FRESH job context — a slow page can never
// starve the next of its deadline — and the meter climbs per page.
const backfillPagesPerTick = 1

func (w *captureBackfillWorker) Work(ctx context.Context, job *river.Job[CaptureBackfillArgs]) error {
	bfID, err := ids.Parse(job.Args.BackfillID)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("capture_backfill: backfill id: %w", err))
	}
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	for i := 0; i < backfillPagesPerTick; i++ {
		done, completed, retryAfter, err := w.registry.RunBackfillStep(wsCtx, bfID)
		if retryAfter > 0 {
			// The provider asked us to wait, and the run is still live with its
			// cursor intact. The row classifies the fault and counts it toward its
			// own give-up cap; River owns the redelivery, so a snooze — not a
			// silent stop — is what keeps the import alive across an outage.
			w.log.WarnContext(ctx, "capture backfill page deferred",
				"backfill", job.Args.BackfillID, "retry_after", retryAfter, "err", err)
			return river.JobSnooze(retryAfter)
		}
		if err != nil {
			// A fault no delay repairs, so the job stops here: the engine has
			// ended the run and put the class on the row — on a context detached
			// from this job, because the job context dying mid-page is itself the
			// commonest fault — and the log carries the detail.
			w.log.WarnContext(ctx, "capture backfill page failed", "backfill", job.Args.BackfillID, "err", err)
			return nil
		}
		if completed {
			// The connect-time import just closed: build today's digest for
			// THIS workspace now, so the morning screen reflects the
			// freshly-imported history instead of waiting for the nightly
			// pass. Best-effort — a failed enqueue never fails the backfill,
			// which the nightly run still covers.
			w.enqueueDigest(ctx, job.Args)
		}
		if done {
			return nil
		}
	}
	return river.JobSnooze(time.Second)
}

// enqueueDigest offers a same-day digest build for THIS workspace through the
// ambient River client; the digest worker rebuilds the day idempotently
// (as-of-now truths).
//
// THE PASS, because there is no longer a child to prefer to it. This argued the
// opposite until ADR-0103 collapsed the digest pair: the intent here is local —
// one tenant just imported its history — and a fleet fan-out would have run
// every workspace in the installation for it, N times over if N tenants
// finished together. There is one kind now, so "local" is not something the
// enqueue can express any more.
//
// What that costs is bounded by the same invariant the collapse rests on: an
// installation holds one active workspace (identity.InstallationWorkspace
// refuses more), so the pass covers exactly the tenant that finished. On a
// fleet it would also re-walk the others, each a re-build of a day they already
// have — the digest is idempotent over as-of-now truths, which is what made the
// pair affordable to collapse in the first place.
//
// oneOffPassOpts carries the queue the contract declares for the kind, and
// states there why a one-off deliberately takes neither the sweep tag nor the
// active-state uniqueness a scheduled tick carries. The second matters here in
// particular: a digest already RUNNING may have snapshotted the workspace
// BEFORE this backfill's rows landed, and deduping against it would drop the
// freshly-imported history off the morning screen until the nightly pass.
//
// The Safely variant is deliberate: the plain ClientFromContext PANICS when
// there is no client, and a best-effort enqueue must degrade rather than crash
// the pager. River puts the client in every Work context, so its absence means
// this method was reached from somewhere that is not a running job — which is
// worth a line, because silently returning is indistinguishable from having
// enqueued. Neither branch fails the backfill: the nightly pass still covers
// the workspace, and a completed import is not undone by a missing digest.
func (w *captureBackfillWorker) enqueueDigest(ctx context.Context, args CaptureBackfillArgs) {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		w.log.WarnContext(ctx, "capture backfill: no River client in context, so the same-day digest was not enqueued",
			"backfill", args.BackfillID, "err", err)
		return
	}
	child := CaptureDigestArgs{}
	if _, err := client.Insert(ctx, child, oneOffPassOpts(child.Kind())); err != nil {
		w.log.WarnContext(ctx, "capture backfill: digest enqueue failed",
			"backfill", args.BackfillID, "err", err)
	}
}

// CounterpartyVerdictArgs runs one counterparty-verdict pass (ADR-0072/A118 §4).
type CounterpartyVerdictArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CounterpartyVerdictArgs) Kind() string { return "capture_counterparty_verdict" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CounterpartyVerdictArgs) FleetWide() {}

// counterpartyVerdictWorker drives the disposition ledger's stages in the order
// they depend on each other: judge what is due, offer to a human what
// judging could not settle, then redact the noise whose undo window has closed.
//
// Judging and staging are separate stages rather than one, so a staging failure
// never costs a verdict that was already paid for — the rows it missed are
// picked up by the next tick, because the backlog is a query, not a queue this
// worker holds.
// It runs ALL of one workspace's stages, in dependency order, inside ONE job.
// The stages are not independent — retiring puts a stranded row in front of the
// review queue, reconciling declines keeps staging from re-asking an answered
// question, and ageing out runs after staging so a row whose window closed this
// tick has had its last chance — so splitting them into separate jobs per
// workspace would break the ordering the pass depends on.
//
// One worker where there were two (ADR-0103).
type counterpartyVerdictWorker struct {
	pool   *pgxpool.Pool
	engine *CounterpartyVerdictEngine
	// purger destroys personal mail past its window. Nil in a role with no
	// object store, and the stage is then skipped rather than half-done.
	purger *CapturePurger
	// backlogNotice tells a seat their capture backlog stopped moving. Nil in a
	// role composed without notices, and the stage is then skipped.
	backlogNotice BacklogNotifier
	log           *slog.Logger
}

func (w *counterpartyVerdictWorker) Work(ctx context.Context, _ *river.Job[CounterpartyVerdictArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.judgeWorkspace))
}

func (w *counterpartyVerdictWorker) judgeWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// The judging pass runs whether or not a model is composed, because not all
	// of it needs one: an owner's own decision and a role mailbox are answered
	// from the ledger and the address, and skipping the whole stage left those
	// rows unanswerable. Every row that DOES need a model is retired to `unsure`
	// inside the pass, which is what puts it in front of a human — a pending row
	// nobody will ever judge is invisible, because it looks exactly like one
	// whose turn has not come.
	//
	// Every other stage runs for the same reason it always did: rows already on
	// the ledger must reach a human, declines must close, and mail already
	// hidden must be redacted on schedule — turning AI off is not consent to
	// retain the content of messages the workspace already decided were noise.
	if err := w.engine.RunWorkspace(wsCtx, 0); err != nil {
		return err
	}
	if err := w.engine.ReconcileLedgerWorkspace(wsCtx); err != nil {
		return err
	}
	if err := w.engine.StageReviewsWorkspace(wsCtx, 0); err != nil {
		return err
	}
	// After staging, not before: a row whose window closed this tick has had its
	// last chance to be offered, and closing it first would withdraw an offer
	// that was about to be re-staged in the same pass.
	if err := w.engine.AgeOutStaleReviewsWorkspace(wsCtx, capture.UnsureReviewWindow); err != nil {
		return err
	}
	if err := w.engine.HideNoiseStragglersWorkspace(wsCtx); err != nil {
		return err
	}
	if err := w.engine.RedactNoiseWorkspace(wsCtx, capture.NoiseUndoWindow, 0); err != nil {
		return err
	}
	// After the stages that MOVE the ledger, so a backlog this pass just cleared
	// is not reported as stalled on its way out.
	if err := w.engine.NoticeBacklogStalledWorkspace(wsCtx, w.backlogNotice); err != nil {
		return err
	}
	// Last, and after redaction: a `personal` verdict destroys rather than
	// hides, so it is the most irreversible thing this pass does and it runs
	// once everything reversible has settled.
	if w.purger == nil {
		return nil
	}
	destroyed, err := w.purger.SweepPersonalMail(wsCtx, capture.DefaultPersonalPurgeWindows())
	if err != nil {
		return err
	}
	if destroyed > 0 && w.log != nil {
		w.log.InfoContext(ctx, "counterparty verdict: destroyed personal mail past its window",
			"workspace", workspace.String(), "messages", destroyed)
	}
	return nil
}
