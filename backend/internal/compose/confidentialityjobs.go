// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The jobs that drive the thread confidentiality ledger: a fleet-wide
// dispatcher that enumerates workspaces, and a per-workspace pass that drains
// one workspace's backlog.
//
// Two jobs rather than one for the reason the sender verdict uses two: a
// dispatcher does no tenant work, so a workspace whose pass fails does not stop
// the rest of the fleet from being judged.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ConfidentialityVerdictArgs runs one fleet-wide confidentiality dispatch.
type ConfidentialityVerdictArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ConfidentialityVerdictArgs) Kind() string { return "capture_confidentiality_verdict" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (ConfidentialityVerdictArgs) FleetWide() {}

// confidentialityVerdictWorker drains every live workspace's thread backlog.
//
// One worker where there were two (ADR-0103).
type confidentialityVerdictWorker struct {
	pool   *pgxpool.Pool
	engine *ConfidentialityVerdictEngine
}

func (w *confidentialityVerdictWorker) Work(ctx context.Context, _ *river.Job[ConfidentialityVerdictArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.judgeWorkspace))
}

func (w *confidentialityVerdictWorker) judgeWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// A deployment with no model bound holds every thread, which is the correct
	// answer rather than a fault: RunWorkspace returns cleanly. Failing the job
	// instead would fill the log with an alarm about a configuration somebody
	// chose.
	if err := w.engine.RunWorkspace(wsCtx, 0); err != nil {
		return err
	}
	// Threads that spent every attempt without an answer end at `unsure`, which
	// HOLDS. Retiring runs after judging so a thread that exhausted its last
	// attempt this tick is retired in the same pass rather than sitting
	// claimable-but-never-claimed until the next one.
	if _, err := w.engine.RetireExhausted(wsCtx); err != nil {
		return err
	}
	// And the answers that never reached their messages. After retiring, so a
	// thread that just became `unsure` has its messages settled in the same
	// tick rather than waiting for the next one.
	if _, err := w.engine.FinishSettledThreads(wsCtx); err != nil {
		return err
	}
	return nil
}
