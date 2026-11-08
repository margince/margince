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
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// The workspace binding stopped being enough when this pass began writing a
	// reply verdict beside the label. SetCaptureLabel carries no gate, so a bare
	// workspace context served for as long as labelling was all this did;
	// SetReplyVerdict is RBAC-gated like every other write of a derived claim
	// about correspondence, and an unbound context fails it with "no actor bound
	// to context" — on every message, every tick, while the labels keep landing
	// so the pass looks alive. owedjobs.go beside this records the same lesson
	// from the other direction, and named this worker as the one that did not
	// need it yet.
	//
	// A SYSTEM principal, which is what this is: nobody asked for the pass and no
	// seat acts through it. auth.Require admits a system principal before it
	// reads any object grant, so the grants are deliberately absent — RowScopeAll
	// is what the row clauses need, and listing objects would suggest they were
	// consulted.
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:capture_classify",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	return w.classifier.RunWorkspace(wsCtx, 0)
}

// CaptureEnrichArgs runs one signature-enrich pass (ADR-0063; §2.9).
type CaptureEnrichArgs struct {
	Pending bool `json:"pending,omitempty"`
}

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

func (w *captureEnrichWorker) Work(ctx context.Context, job *river.Job[CaptureEnrichArgs]) error {
	if job.Args.Pending {
		return jobs.FaultContext(ctx, runPendingPass(ctx, w.pool, w.enricher.RunPendingWorkspace))
	}
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

// CompanyNamePromotionArgs runs one company-name promotion pass (PO-F-2a).
type CompanyNamePromotionArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CompanyNamePromotionArgs) Kind() string { return "company_name_promotion" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CompanyNamePromotionArgs) FleetWide() {}

// companyNamePromotionWorker promotes names for every live workspace.
//
// One worker where there were two (ADR-0103).
type companyNamePromotionWorker struct {
	pool     *pgxpool.Pool
	promoter *CompanyNamePromoter
}

func (w *companyNamePromotionWorker) Work(ctx context.Context, _ *river.Job[CompanyNamePromotionArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.promoteWorkspace))
}

// companyNamePromotionWorkspaceWorker runs one workspace's pass: a database-only
// walk over the company_name evidence the enrich job collects.
func (w *companyNamePromotionWorker) promoteWorkspace(ctx context.Context, workspace ids.UUID) error {
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

// CounterpartyVerdictArgs runs one counterparty-verdict pass (ADR-0072/A118 §4).
type CounterpartyVerdictArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CounterpartyVerdictArgs) Kind() string { return "capture_counterparty_verdict" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CounterpartyVerdictArgs) FleetWide() {}

// Stages share one job because publication and widening depend on settled verdicts.
// A failed stage is retried from its durable ledger state.
type counterpartyVerdictWorker struct {
	pool   *pgxpool.Pool
	engine *CounterpartyVerdictEngine
	// purger destroys personal mail past its window. Nil in a role with no
	// object store, and the stage is then skipped rather than half-done.
	purger *CapturePurger
	// stripper withholds the files of personal-thread mail past the same
	// window. Nil without an object store, for the same reason.
	stripper *privateThreadStripper
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
	// Owner decisions and address rules settle without a model.
	// Noise retention still applies when model classification is disabled.
	if err := w.engine.RunWorkspace(wsCtx, 0); err != nil {
		return err
	}
	if err := w.engine.ReconcileLedgerWorkspace(wsCtx); err != nil {
		return err
	}
	// Publish answered contacts before widening their mail on the same pass.
	if err := w.engine.PublishAnsweredContactsWorkspace(wsCtx); err != nil {
		return err
	}
	if err := w.engine.LiftVendorMarksWorkspace(wsCtx); err != nil {
		return err
	}
	if err := w.engine.WidenClearedSendersWorkspace(wsCtx); err != nil {
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
	return w.stripPrivateThreads(ctx, wsCtx, workspace)
}

// stripPrivateThreads withholds the files of personal-thread mail past the
// purge's window. After the purge, so mail it destroys is not stripped first.
func (w *counterpartyVerdictWorker) stripPrivateThreads(ctx, wsCtx context.Context, workspace ids.UUID) error {
	if w.stripper == nil {
		return nil
	}
	stripped, err := w.stripper.StripWorkspace(wsCtx, capture.DefaultPersonalPurgeWindows())
	if err != nil {
		return err
	}
	if stripped > 0 && w.log != nil {
		w.log.InfoContext(ctx, "counterparty verdict: withheld the files of personal-thread mail past its window",
			"workspace", workspace.String(), "messages", stripped)
	}
	return nil
}
