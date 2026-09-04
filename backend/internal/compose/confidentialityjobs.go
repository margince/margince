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
)

// ConfidentialityVerdictArgs runs one fleet-wide confidentiality dispatch.
type ConfidentialityVerdictArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ConfidentialityVerdictArgs) Kind() string { return "capture_confidentiality_verdict" }

// FleetWide marks this a dispatcher: it enumerates and enqueues, and does no
// tenant work of its own (jobs.FleetWide).
func (ConfidentialityVerdictArgs) FleetWide() {}

// confidentialityVerdictWorker enqueues one pass per workspace.
type confidentialityVerdictWorker struct {
	pool *pgxpool.Pool
}

func (w *confidentialityVerdictWorker) Work(ctx context.Context, _ *river.Job[ConfidentialityVerdictArgs]) error {
	return jobs.FaultContext(ctx, dispatchPerWorkspace(ctx, w.pool,
		workspaceSweepOpts(ConfidentialityVerdictWorkspaceArgs{}.Kind()),
		func(ws ids.UUID) river.JobArgs { return ConfidentialityVerdictWorkspaceArgs{Workspace: ws} }))
}

// ConfidentialityVerdictWorkspaceArgs runs one workspace's confidentiality pass.
type ConfidentialityVerdictWorkspaceArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ConfidentialityVerdictWorkspaceArgs) Kind() string {
	return "capture_confidentiality_verdict_workspace"
}

// WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
func (a ConfidentialityVerdictWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }

// confidentialityVerdictWorkspaceWorker drains one workspace's thread backlog.
type confidentialityVerdictWorkspaceWorker struct {
	engine *ConfidentialityVerdictEngine
}

func (w *confidentialityVerdictWorkspaceWorker) Work(ctx context.Context, job *river.Job[ConfidentialityVerdictWorkspaceArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// A deployment with no model bound holds every thread, which is the correct
	// answer rather than a fault: RunWorkspace returns cleanly. Failing the job
	// instead would fill the log with an alarm about a configuration somebody
	// chose.
	if err := w.engine.RunWorkspace(wsCtx, 0); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// Threads that spent every attempt without an answer end at `unsure`, which
	// HOLDS. Retiring runs after judging so a thread that exhausted its last
	// attempt this tick is retired in the same pass rather than sitting
	// claimable-but-never-claimed until the next one.
	if _, err := w.engine.RetireExhausted(wsCtx); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// And the answers that never reached their messages. After retiring, so a
	// thread that just became `unsure` has its messages settled in the same
	// tick rather than waiting for the next one.
	if _, err := w.engine.FinishSettledThreads(wsCtx); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return nil
}
