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
// WHY A PASS AND NOT THE QUEUE READ. The verdict does not depend on who reads
// the queue: the bar is the median of the at-risk deals the reader can see,
// and every human seat reads every deal whatever its row scope (deal is an
// identity table in platform/auth). So one judgement per workspace per day,
// taken as the system principal, is the verdict each reader was shown — and a
// GET that wrote rows would make a read a write.
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

	"github.com/margince/margince/backend/internal/compose/attention"
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

// riskVerdictSweepWorker records every live workspace's verdicts for today.
type riskVerdictSweepWorker struct {
	pool  *pgxpool.Pool
	judge *attention.Service
	now   func() time.Time
	log   *slog.Logger
}

// newRiskVerdictSweepWorker binds the pass to the worklist's own service.
//
// No approvals service: the judgement reads the at-risk lane and the pricing
// seam and nothing else, and the lanes that would need one are never asked.
func newRiskVerdictSweepWorker(pool *pgxpool.Pool, now func() time.Time, log *slog.Logger) *riskVerdictSweepWorker {
	return &riskVerdictSweepWorker{
		pool: pool, judge: newAttentionService(pool, nil, now), now: now, log: log,
	}
}

func (w *riskVerdictSweepWorker) Work(ctx context.Context, _ *river.Job[RiskVerdictSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.recordWorkspace))
}

func (w *riskVerdictSweepWorker) recordWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	wsCtx = principal.SystemActing(wsCtx, riskVerdictActor)
	return jobs.FaultContext(ctx, w.record(wsCtx, workspace))
}

// record judges the workspace now and files the verdicts under today's local
// day.
func (w *riskVerdictSweepWorker) record(ctx context.Context, workspace ids.UUID) error {
	zone, err := installationZone(ctx, w.pool)
	if err != nil {
		return err
	}
	material, err := w.judge.MaterialAtRisk(ctx)
	if err != nil {
		return fmt.Errorf("judging the at-risk pipeline: %w", err)
	}
	day := storekit.WorkspaceDay(w.now(), zone)
	store := deals.NewStore(InstallationDB(w.pool), DealsInstallation())
	written, err := store.RecordRiskVerdicts(ctx, day, material)
	if err != nil {
		return err
	}
	w.log.DebugContext(ctx, "recorded the day's material at-risk verdicts",
		"workspace_id", workspace, "local_day", day.Format(time.DateOnly),
		"judged", len(material), "new", written)
	return nil
}
