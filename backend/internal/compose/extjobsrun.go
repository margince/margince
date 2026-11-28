// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The running half of the extension job seam: the two args types every composed
// job shares, the dispatcher that fans out over the fleet, and the workspace
// child that mints the tick's authority and then runs the unit's tick.
//
// extjobs.go is the other half — what gets registered, and the two shapes that
// are refused before anything is.

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// extJobDispatcherArgs is EVERY composed extension job's dispatcher row.
//
// The kind is a FIELD rather than a constant, which is the one thing about this
// pair that differs from every other args type in the tree, and it is not a
// convenience: the composed set is not known when this package is compiled, so
// there is no type to write per kind. River reads the kind off the args value
// on both sides of the seam — args.Kind() on insert, and the value handed to
// river.AddWorkerArgs on registration — so a field is exactly as load-bearing
// as a constant would be, and the census's kind↔type pairing still holds
// because the registration passes the same value the insert will.
type extJobDispatcherArgs struct {
	JobKind string `json:"job_kind"`
}

// Kind is the composed dispatcher kind this row carries, ext_<unit>_<job>.
func (a extJobDispatcherArgs) Kind() string { return a.JobKind }

// FleetWide marks this as a dispatcher: it enumerates and enqueues and touches
// no tenant data (jobs.FleetWide).
func (extJobDispatcherArgs) FleetWide() {}

// extJobWorkspaceArgs is the tick of one composed job.
//
// The row carries no principal. What acts is the job, and its authority is a
// pure function of the declaration (extensionJobPrincipal), so a queued row has
// nothing to carry stale.
//
// The args are the kind alone, so ByArgs uniqueness holds one live tick of a
// job. A second varying field would need `river:"unique"` here, or the hash
// would widen and let a second tick start while the first runs.
type extJobWorkspaceArgs struct {
	JobKind string `json:"job_kind"`
}

// Kind is the composed child kind, ext_<unit>_<job>_ws.
func (a extJobWorkspaceArgs) Kind() string { return a.JobKind }

// extJobDispatcherWorker enqueues the tick of one composed job.
type extJobDispatcherWorker struct {
	pool *pgxpool.Pool
	decl extension.JobDeclaration
}

// Work enqueues the job's tick in one insert, once the installation has a
// company.
//
// Enablement is directory presence: a unit composed into an installation is
// composed for all of it, so the tick has no switch to consult.
func (w *extJobDispatcherWorker) Work(ctx context.Context, _ *river.Job[extJobDispatcherArgs]) error {
	_, ok, err := database.LiveWorkspace(ctx, w.pool)
	if err != nil || !ok {
		return jobs.FaultContext(ctx, err)
	}
	child := w.decl.ChildKind()
	// workspaceSweepOpts and not sweepInsertOpts: it adds ByArgs, which keeps a
	// slow tick from being started a second time while it is still running.
	// FaultContext, not FaultForKind, because a dispatcher runs NO unit code: it
	// holds a pool and a declaration and never the unit's handler, so every error
	// below is compose's own and no declared failure class can reach here. Asking
	// for kind-verification would be asking about a value that cannot arrive.
	return jobs.FaultContext(ctx, dispatchOnce(ctx, clientInsertMany(ctx), workspaceSweepOpts(child),
		extJobWorkspaceArgs{JobKind: child}))
}

// extensionJobPrincipal is the authority a scheduled tick answers as: the JOB,
// named by its dispatcher kind, and no user at all.
//
// WHAT IS ABSENT IS THE POINT. A tick is work nobody requested, so there is no
// contact to name — and naming one anyway is what this used to do, by resolving a
// seeded `is_agent` row that authorized nothing and cost a licence seat. The
// units that ship scheduled work do not act on this authority: each lands its
// records through Runtime.Ingest, which resolves the MEMBER's live grants and
// discards whatever the tick was carrying. So the honest answer here is a job.
//
// THE EMPTY Permissions IS LOAD-BEARING, not an omission. There are two
// independent locks on the governed core-write door: extcore.go refuses an
// unattended write outright, and auth.Require denies every object to a principal
// holding no grants. This is the second one, and extjobprincipal_test.go is what
// holds it — nothing else does.
//
// PrincipalAgent, NOT PrincipalSystem. System looks like the tidier word for
// "no user" and it would delete that second lock: auth.Require returns nil for
// PrincipalSystem before Permissions is consulted, and auth.Unbounded hands it
// RowScopeAll. It also keeps audit_log.actor_type = 'agent', which is what the
// captured_by_kind=agent lane reads. PrincipalHuman would break both at once.
//
// No error to return: every field is a pure function of the declaration, which
// is why the row this runs for carries no principal to go stale.
func extensionJobPrincipal(decl extension.JobDeclaration) principal.Principal {
	return principal.Principal{
		Type: principal.PrincipalAgent,
		ID:   "agent:" + decl.DispatcherKind(),
		// The declared scope and nothing wider. It is a REQUEST an operator
		// resolves, so it bounds the tick rather than granting it anything: the
		// capability surface the handler holds is the Runtime's, and this is
		// what the audit rows record the tick as having asked for.
		Scopes: principal.NewScopeSet(principal.Scope(decl.RequestedScope)),
	}
}

// extJobWorkspaceWorker runs ONE workspace's tick of one composed job.
type extJobWorkspaceWorker struct {
	pool   *pgxpool.Pool
	decl   extension.JobDeclaration
	handle extension.JobHandler
	log    *slog.Logger
}

func (w *extJobWorkspaceWorker) Work(ctx context.Context, job *river.Job[extJobWorkspaceArgs]) error {
	// First, before anything reads a row: every capability the handler gets
	// re-derives the workspace from the context. An unbound context would refuse
	// at the unit's first query, which names the wrong fault.
	wsCtx := ctx
	wsCtx = principal.WithActor(wsCtx, extensionJobPrincipal(w.decl))
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	// FaultForKind, on the CHILD's kind, because w.tick is the one call here that
	// runs the UNIT's code and so the one that can return a declared failure
	// class. A class is honoured only when it is exactly one this installation
	// registered for this kind; see jobs.FaultForKind for why the write path
	// verifies rather than trusts.
	//
	// On wsCtx: the log line is written from this context, and jobs.FaultForKind
	// reads the correlation id and the workspace off it. ctx carries neither.
	return jobs.FaultForKind(wsCtx, w.decl.ChildKind(), w.tick(wsCtx))
}

// tick mints the call-scoped Runtime, runs the unit's handler with it, and
// releases it — the job seam's copy of extensionTool.Handle, and for the same
// reason: nothing an extension can reach exists until this line runs.
//
// The panic recovery is HERE rather than left to River's, and it does NOT buy
// the things it looks like it buys. River already recovers a panicking worker
// and fails the attempt — one attempt dies, the runner does not — and
// `defer rt.release()` already runs during panic unwinding whether or not
// anything recovers, so this frame guarantees neither.
//
// It buys two things River cannot, and both were confirmed by deleting it:
//
//   - ATTRIBUTION. One args type serves every composed job in the process, so
//     River's report names a shared Go type and a kind string and says nothing
//     about whose code ran. The slog line below names the unit, the job and the
//     kind. It is the LOG rather than the row because river_job.errors is
//     fleet-visible with no RLS, so jobs.FaultContext deliberately replaces an
//     unclassified cause with a fixed sentence before it is stored — the
//     diagnosis belongs where FaultContext already sends it.
//   - CONTAINMENT of the panic value. River's own recovery does not go through
//     FaultContext, so an unrecovered panic writes the unit's raw panic text
//     straight into that fleet-visible column. A third-party unit's panic value
//     is precisely the text that must not land there. Converting it to an error
//     here puts it back on the path every other worker's failure takes.
func (w *extJobWorkspaceWorker) tick(ctx context.Context) (err error) {
	deps := boundExtensionRuntime()
	// No version: a job declaration carries none, and a tick writes no core
	// record to attribute (Core refuses one outright), so the surface alone is
	// the whole of what a tick's provenance could say.
	rt := jobRuntimeFor(ctx, string(w.decl.Unit), "", "job/"+w.decl.Job, deps)
	defer rt.release()
	defer func() {
		if r := recover(); r != nil {
			instrument.Panic(ctx, r, debug.Stack())
			w.log.Error("extension job tick panicked",
				"unit", string(w.decl.Unit), "job", w.decl.Job, "kind", w.decl.ChildKind(), "panic", r)
			err = fmt.Errorf("compose: extension %q job %q panicked: %v", w.decl.Unit, w.decl.Job, r)
		}
	}()
	return w.handle(ctx, rt)
}
