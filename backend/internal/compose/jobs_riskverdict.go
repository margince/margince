// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The hourly record of which deals the queue judged material and at risk.
//
// The same-day next-step figure on GET /worklist/response needs to know, for a
// past day, which deals were material and at risk on it. The queue decides
// that per read against the pipeline's own median, so the answer for a past
// day is gone the moment the pipeline moves. This pass writes it down.
//
// WHY A PASS AND NOT THE QUEUE READ. The verdict recorded is the workspace's:
// the median over every at-risk amount, taken as the system principal. Every
// human seat reads every deal whatever its row scope (deal is an identity
// table in platform/auth), so for any reader whose field masks leave deal
// money alone it is the verdict their queue showed; a reader with money masked
// is not shown the figure at all. And a GET that wrote rows would make a read
// a write.
//
// THE MORNING SET. The first successful pass after local midnight records the
// day and the deals it judged; every later pass that day finds the day taken
// and writes nothing. The pass runs hourly only so that a missed early pass is
// taken by the next one. Sampling every hour instead would miss a deal that
// turned at risk and was dealt with between two passes, and count against the
// rate only the deals nobody touched.
//
// The judgement is attention.Service.MaterialAtRisk over the SAME service the
// worklist route serves (newAttentionService), so there is one bar in the
// product and the record cannot disagree with the queue.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RiskVerdictSweepArgs schedules one hourly verdict pass across the fleet.
type RiskVerdictSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (RiskVerdictSweepArgs) Kind() string { return "risk_verdict_sweep" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (RiskVerdictSweepArgs) FleetWide() {}

// riskVerdictActor is the principal the pass runs as, and the one every
// verdict row and its audit row name.
const riskVerdictActor = "system:risk-verdict"

// riskVerdictSweepWorker records each live workspace's morning set.
type riskVerdictSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

func newRiskVerdictSweepWorker(pool *pgxpool.Pool, now func() time.Time, log *slog.Logger) *riskVerdictSweepWorker {
	return &riskVerdictSweepWorker{pool: pool, now: now, log: log}
}

func (w *riskVerdictSweepWorker) Work(ctx context.Context, _ *river.Job[RiskVerdictSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.recordWorkspace))
}

func (w *riskVerdictSweepWorker) recordWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	wsCtx = principal.SystemActing(wsCtx, riskVerdictActor)
	return jobs.FaultContext(ctx, w.record(wsCtx, workspace))
}

// record judges the workspace at ONE instant and files the result under the
// local day that instant falls on, unless that day is already recorded.
//
// The clock is read once and handed to a service built for this pass alone,
// so the at-risk selection, the pricing and the day all answer to the same
// instant. Two readings would let a pass that starts at 23:59:59 judge
// yesterday's pipeline and file it under today.
//
// No approvals service: the judgement reads the at-risk lane and the pricing
// seam and nothing else, and the lanes that would need one are never asked.
func (w *riskVerdictSweepWorker) record(ctx context.Context, workspace ids.UUID) error {
	at := w.now()
	zone, err := installationZone(ctx, w.pool)
	if err != nil {
		return err
	}
	judge := newAttentionService(w.pool, nil, func() time.Time { return at })
	material, err := judge.MaterialAtRisk(ctx)
	if err != nil {
		return fmt.Errorf("judging the at-risk pipeline: %w", err)
	}
	day := deals.RiskDay{
		LocalDay: storekit.WorkspaceDay(at, zone),
		Start:    storekit.StartOfDay(at, zone),
		End:      storekit.StartOfNextDay(at, zone),
		JudgedAt: at,
	}
	store := deals.NewStore(InstallationDB(w.pool), DealsInstallation())
	first, err := store.RecordRiskDay(ctx, day, material)
	if err != nil {
		return err
	}
	w.log.DebugContext(ctx, "judged the at-risk pipeline",
		"workspace_id", workspace, "local_day", day.LocalDay.Format(time.DateOnly),
		"material", len(material), "recorded", first)
	return nil
}
