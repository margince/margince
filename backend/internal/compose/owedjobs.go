// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two jobs behind the owed-verdict pass: one dispatcher that enumerates
// workspaces and one worker that drains a single workspace's backlog.
//
// The same pair the capture-label pass uses, and the split matters for the same
// reason: a shared pass over every tenant lets one large backlog spend the
// model budget and starve every workspace behind it.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OwedVerdictArgs runs one catch-up pass over every workspace.
type OwedVerdictArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (OwedVerdictArgs) Kind() string { return "owed_verdict" }

// FleetWide marks this a dispatcher: it enumerates and enqueues, and does no
// tenant work of its own (jobs.FleetWide).
func (OwedVerdictArgs) FleetWide() {}

// owedVerdictWorker is the dispatcher for the verdict pass.
type owedVerdictWorker struct {
	pool *pgxpool.Pool
}

func (w *owedVerdictWorker) Work(ctx context.Context, _ *river.Job[OwedVerdictArgs]) error {
	return jobs.FaultContext(ctx, dispatchPerWorkspace(ctx, w.pool,
		workspaceSweepOpts(OwedVerdictWorkspaceArgs{}.Kind()),
		func(ws ids.UUID) river.JobArgs { return OwedVerdictWorkspaceArgs{Workspace: ws} }))
}

// OwedVerdictWorkspaceArgs is one workspace's catch-up verdict pass.
type OwedVerdictWorkspaceArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (OwedVerdictWorkspaceArgs) Kind() string { return "owed_verdict_workspace" }

// WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
func (a OwedVerdictWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }

// owedVerdictWorkspaceWorker drives the verdict engine for one workspace.
//
// The engine commits per model call, so a mid-pass crash or a budget stop loses
// nothing: what was judged stays judged, and the next tick reads a backlog that
// has shrunk by exactly that much.
type owedVerdictWorkspaceWorker struct {
	classifier *OwedClassifier
}

func (w *owedVerdictWorkspaceWorker) Work(ctx context.Context, job *river.Job[OwedVerdictWorkspaceArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	return jobs.FaultContext(ctx, w.classifier.RunWorkspace(wsCtx, 0))
}
