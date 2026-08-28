// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The "do it now" a scheduled connector needs, and the three things it must not
// become.
//
// pkg/extension/syncnow.go carries why the capability exists. This is where the
// bounds are actual rather than documented, and each one is a property of how
// the enqueue is CONSTRUCTED rather than a check somebody has to remember:
//
//   - THE UNIT'S OWN JOB. The name is resolved against the declarations of the
//     unit this Runtime was minted for, which the handler has no way to supply
//     — the same fact that scopes its secrets and its ledger.
//   - THE CALLER'S OWN TENANT. The workspace is the invocation's, read the way
//     every other capability reads it, so there is no parameter to get wrong
//     and no shape in which a unit asks for somebody else's sync.
//   - ONE RUN, NOT A THOUSAND. Repeated calls coalesce onto the queued row for
//     the workspace, so a member holding down save gets one tick.
//
// And the invariant it leaves alone: what is enqueued is the SAME child kind
// the clock enqueues, with the same workspace agent seat as its principal. The
// tick runs unattended, exactly as it always did — what crosses this seam is a
// request to schedule, never an authority to ingest, so ErrAttendedIngest keeps
// meaning what it means.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// SyncNow enqueues this unit's own declared job for the calling workspace.
func (r *callRuntime) SyncNow(ctx context.Context, job extension.JobName) error {
	ctx, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	decl, found := declaredJobFor(r.unit, string(job))
	if !found {
		// The unit's OWN set, so a name that belongs to another unit reads the
		// same as one that belongs to nobody: this unit declares no such job.
		// Answering differently would tell a unit which jobs its neighbours have.
		return fmt.Errorf("%w: %q", extension.ErrNoSuchJob, job)
	}
	// The invocation's own binding, which scoped has already found non-nil —
	// the same pool every other capability on this Runtime reaches the
	// installation through, rather than a second lookup that could answer
	// differently.
	pool := r.deps.pool
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return database.ErrNoWorkspace
	}
	// The SAME actor the clock's dispatch records: the workspace's agent seat,
	// re-derived at execution rather than carried, so a seat deactivated while
	// the row waits ends the tick the way it ends a scheduled one.
	actor, err := extensionJobActor(ctx, pool, ws)
	if err != nil {
		return err
	}
	if actor.IsZero() {
		// The same state the fleet dispatch skips a workspace for. Said out
		// loud here because a caller asked: a screen that reported "checking
		// now" over a tick that will never run is the failure this capability
		// was added to end, in a new place.
		return extension.ErrNoUnattendedSeat
	}
	inserter, err := jobs.NewInserter(pool, slog.Default())
	if err != nil {
		return fmt.Errorf("compose: opening the queue to ask for %s: %w", job, err)
	}
	child := decl.ChildKind()
	opts, err := attendedChildOpts(child)
	if err != nil {
		return err
	}
	return inserter.Enqueue(ctx, extJobWorkspaceArgs{JobKind: child, Workspace: ws, Principal: actor}, opts)
}

// declaredJobFor answers the declaration of one unit's job, and whether the
// unit declares it at all.
//
// Read from the composed set rather than from anything the caller supplies,
// which is what makes "its own job" a property of construction: the set is what
// this boot registered, and the unit is the one the Runtime was minted for.
func declaredJobFor(unit, job string) (extension.JobDeclaration, bool) {
	for _, served := range servedExtensionJobs() {
		if string(served.decl.Unit) == unit && served.decl.Job == job {
			return served.decl, true
		}
	}
	return extension.JobDeclaration{}, false
}

// attendedChildOpts is how a caller-requested run is enqueued, and it sits
// between its two neighbours for reasons worth stating rather than inheriting.
//
// It is NOT workspaceSweepOpts: that marks the row as one workspace's share of
// a fleet pass, and the sweep gauges count tagged rows. A run somebody asked
// for is not a pass the clock made, and tagging it would inflate the coverage
// reading of a pass that never ran.
//
// It is NOT oneOffChildOpts either, and this is the half that matters here.
// That one deliberately drops active-state uniqueness, because an event's whole
// claim is that new rows landed and deduplicating against a pass already
// running would drop exactly the data the event fired about. A human pressing
// save makes no such claim — they are asking for a check, and a check already
// queued or running IS the check. So uniqueness stays on, and it is what turns
// a held-down button into one tick.
//
// It RETURNS its refusal where both neighbours panic, and that difference is
// the call site rather than a change of heart. Those two are reached from boot
// wiring and from a dispatcher, where a kind whose declaration says something
// else is a composition fault and a panic is the boot failure it deserves.
// This one is reached from a unit's handler, inside a member's request: the
// same fault there would take down the call, or the worker, for a mistake in a
// declaration nobody in that request can fix.
func attendedChildOpts(childKind string) (*river.InsertOpts, error) {
	spec := fanOutChildSpec(childKind)
	if spec.OptsOwner != jobs.OptsFanOut {
		return nil, fmt.Errorf(
			"compose: %s declares an opts_owner other than fan_out, so its queue and attempt cap are not "+
				"this helper's to set", childKind)
	}
	return &river.InsertOpts{
		Queue:       spec.Queue,
		MaxAttempts: spec.MaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}, nil
}
