// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signal producers' River wiring (SIG-F-3): one hourly pass per workspace
// that runs both halves — the deterministic ghosted-thread rule, then the
// model read of the settled conversations.
//
// They ride ONE job on purpose. Both write signals about the same accounts,
// and a rep reading the page should not see half an account's signals because
// two schedules drifted apart. The deterministic half runs first and always:
// it costs one query and no model call, so an installation with no AI
// configured still gets the signals a comparison can produce.
//
// Job args and worker adapters only — the engines stay River-agnostic.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SignalScanArgs runs one fleet-wide signal-producer pass.
type SignalScanArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (SignalScanArgs) Kind() string { return "signal_scan" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (SignalScanArgs) FleetWide() {}

// signalScanWorker scans every live workspace.
//
// One worker where there were two (ADR-0103).
type signalScanWorker struct {
	pool      *pgxpool.Pool
	extractor *SignalExtractor
	proposer  *SignalProposer
	now       func() time.Time
	log       *slog.Logger
}

func (w *signalScanWorker) Work(ctx context.Context, _ *river.Job[SignalScanArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.scanWorkspaceSignals))
}

// signalScanWorkspaceWorker runs both producers for one workspace.
//
// signalScanTimeout overrides River's one-minute default.
//
// One pass reads up to extractThreadCap conversations, each of them a model
// call, so a minute covers a quiet workspace and nothing else. An installation
// that has just connected a mailbox has thousands of messages of history: under
// the default such a pass is killed part-way through, retried twice against the
// same backlog and discarded, while the reading it managed is committed and
// fine.
//
// Five minutes is a pass that gets somewhere against a real backlog. It is a
// ceiling, not a target: the pass stops itself with extractStopMargin to spare
// (see outOfTime), so this bound is what it may use, never what it waits for.
const signalScanTimeout = 5 * time.Minute

// Timeout gives one pass room to work through a real backlog.
func (w *signalScanWorker) Timeout(*river.Job[SignalScanArgs]) time.Duration {
	return signalScanTimeout
}

func (w *signalScanWorker) scanWorkspaceSignals(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// The producer is the acting principal: every signal it writes carries
	// agent: provenance, and a reader can tell it from a human's own note.
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "agent:signal-scan",
	})
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	wsID := ids.From[ids.WorkspaceKind](workspace)

	now := w.now()
	var ghosted, quiet GhostedPass
	if err := database.WithWorkspaceTx(wsCtx, w.pool, func(tx pgx.Tx) error {
		pass, err := WriteGhostedSignals(wsCtx, tx, now)
		if err != nil {
			return err
		}
		ghosted = pass
		// The second comparison rule rides the same transaction: both are
		// one query and no model call, and a rep should not see one half of
		// the deterministic findings because two schedules drifted.
		quiet, err = WriteProjectQuietSignals(wsCtx, tx, now)
		return err
	}); err != nil {
		return jobs.FaultContext(ctx, err)
	}

	var read ExtractPass
	var extractErr error
	if w.extractor != nil {
		read, extractErr = w.extractor.RunWorkspace(wsCtx, wsID)
	}
	// The offers come last, over every open signal rather than only the ones
	// this pass raised: a crash between a signal and its offer self-heals here,
	// and a signal raised before this surface existed still gets its offer.
	//
	// It runs even when a producer above it failed, and for exactly that
	// reason: it is the pass that reconciles offers for every open signal, so
	// skipping it on a producer's error withholds approval offers from signals
	// that are already standing. Every error is reported, none of them stop
	// the reconcile.
	standing, proposeErr := w.proposer.RunWorkspace(wsCtx)

	// Logged on EVERY pass, including the ones that raised nothing and the ones
	// that failed, and carrying what each half was OFFERED as well as what it
	// wrote.
	//
	// Raised alone cannot tell a calm week from a broken walk: both write
	// nothing. Considered and due are what separate them, and a pass that
	// failed is exactly when "what did the working half manage?" is worth
	// knowing — so this runs before the error is returned, not after.
	//
	// The extractor's numbers are omitted when no model lane is bound, because
	// a hard zero there reads as the broken-queue signature rather than as an
	// installation that bought no model.
	fields := []any{
		"ghosted_considered", ghosted.Considered, "ghosted_raised", ghosted.Raised,
		"quiet_projects_considered", quiet.Considered, "quiet_projects_raised", quiet.Raised,
		"offers_standing", standing, "model_lane", w.extractor != nil,
	}
	if w.extractor != nil {
		fields = append(fields,
			"threads_due", read.Due, "extracted", read.Raised,
			"at_cap", read.AtCap, "budget_deferred", read.Deferred,
			"out_of_time", read.OutOfTime)
	}
	w.log.InfoContext(wsCtx, "signal scan pass", fields...)

	if err := errors.Join(extractErr, proposeErr); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return nil
}

// newSignalExtractorIfConfigured builds the model half of the pass, or nil
// when no lane is bound. The nil IS the wiring: the worker runs the
// deterministic half either way, so an unconfigured installation gets the
// signals a comparison can produce and none of the ones only a reader can.
func newSignalExtractorIfConfigured(pool *pgxpool.Pool, brain completer, log *slog.Logger) *SignalExtractor {
	if brain == nil {
		return nil
	}
	return NewSignalExtractor(pool, brain, time.Now, log)
}

// addSignalJobs registers both producers and hands back the dispatcher's
// schedule, so this wiring stays one line in jobs.go as the surface grows —
// the same shape addGraphJobs and addPrivacyRetentionJobs already keep. The
// cadence is api/jobs.yaml's, which is why periodicFor is what places it.
//
// Both register unconditionally. The deterministic ghosted-thread rule needs
// no model, so an installation that bought none still gets the signals a
// comparison can produce; only the extractor half is absent by omission, and
// that absence is inside the worker rather than a registration gate.
func addSignalJobs(
	reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger,
) []*river.PeriodicJob {
	addDeclaredWorker[SignalScanArgs](reg, &signalScanWorker{
		pool:      pool,
		extractor: newSignalExtractorIfConfigured(pool, cfg.SignalExtractBrain, log),
		// The SAME approvals service the HTTP surface decides on, so a released
		// effect can redeem the offer this reconciler staged.
		proposer: NewSignalProposer(pool, approvalsServiceWithEffects(pool), log),
		now:      time.Now,
		log:      log,
	})
	return periodicFor(cfg, SignalScanArgs{})
}
