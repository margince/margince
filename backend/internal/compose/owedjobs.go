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

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OwedVerdictArgs runs one catch-up pass over every workspace.
type OwedVerdictArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (OwedVerdictArgs) Kind() string { return "owed_verdict" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (OwedVerdictArgs) FleetWide() {}

// owedVerdictWorker drives the verdict engine for every live workspace.
//
// The engine commits per model call, so a mid-pass crash or a budget stop loses
// nothing: what was judged stays judged, and the next tick reads a backlog that
// has shrunk by exactly that much.
//
// One worker where there were two (ADR-0103).
type owedVerdictWorker struct {
	pool       *pgxpool.Pool
	classifier *OwedClassifier
	// settler asks the second half of the same question: once we have written
	// back, did our own words settle what was asked. It runs AFTER the
	// classifier on purpose — a request has to be recognised before it can be
	// settled, and running the two in one tick means a request recognised this
	// hour is judged this hour rather than next.
	settler *RequestSettler
}

func (w *owedVerdictWorker) Work(ctx context.Context, _ *river.Job[OwedVerdictArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.judgeWorkspace))
}

func (w *owedVerdictWorker) judgeWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// The workspace binding alone is not enough here, and the difference is the
	// whole reason this is spelled out: the backlog read and the verdict write
	// are both RBAC-gated, so an unbound context fails them with "no actor bound
	// to context" and the pass writes nothing on every tick. The capture-label
	// worker beside this one needs none because its store methods carry no gate.
	//
	// A SYSTEM principal, which is what this is: nobody asked for the pass and
	// no seat is acting through it. auth.Require admits a system principal
	// before it reads any object grant, so the grants are deliberately absent —
	// RowScopeAll is what the row clauses need, and listing objects as well
	// would suggest they were consulted.
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: activities.OwedVerdictCapturedBy,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	if err := w.classifier.RunWorkspace(wsCtx, 0); err != nil {
		return err
	}
	return w.settleWorkspace(ctx, workspace)
}

// settleWorkspace judges what our replies did, under its OWN principal.
//
// A principal of its own rather than the verdict pass's, because captured_by is
// what the settlement store reads to decide whether a reminder was filed by a
// machine and may therefore be retitled. Running this under
// system:owed_verdict would make every task this pass touches look like one the
// owed pass had filed.
func (w *owedVerdictWorker) settleWorkspace(ctx context.Context, workspace ids.UUID) error {
	// No settler is a deployment with no model bound to the settlement task,
	// and then the pass does what it did before this existed: recognise
	// requests and settle none. The engine's own nil-brain guard cannot answer
	// this one — there is no engine to ask.
	if w.settler == nil {
		return nil
	}
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:request_settlement",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	return w.settler.RunWorkspace(wsCtx, 0)
}
