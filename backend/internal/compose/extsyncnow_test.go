// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// SyncNow's bounds, over the composed set rather than a fake: what makes "the
// unit's own job and nothing else" true is which declarations this boot
// registered, so a test that supplied its own would be testing the supply.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// composedJobsForTest installs a two-unit job set for one test and puts the
// process back the way it found it. The set is boot state; a test that left its
// own behind would govern every test after it.
func composedJobsForTest(t *testing.T, decls ...extension.JobDeclaration) {
	t.Helper()
	before := servedExtensionJobs()
	set := make([]composedJob, 0, len(decls))
	for _, decl := range decls {
		set = append(set, composedJob{decl: decl})
	}
	setComposedJobs(set)
	t.Cleanup(func() { setComposedJobs(before) })
}

func jobOf(unit, job string) extension.JobDeclaration {
	return extension.JobDeclaration{Unit: extension.Name(unit), Job: job}
}

// wiredRuntime is one unit's per-call Runtime, pinned to a workspace and wired
// to a pool it never reaches: every case below is refused before the queue, and
// a pool that answered would be testing the queue rather than the bound.
func wiredRuntime(unit string) *callRuntime {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return &callRuntime{
		unit:    unit,
		live:    true,
		callCtx: ctx,
		deps:    extensionRuntimeBinding{pool: &pgxpool.Pool{}},
	}
}

// A unit may ask for its OWN job and for nothing else. Two units declaring a
// job each is the shape that matters: "alpha asks for beta's job" has to read
// the same as "alpha asks for a job nobody declares", or the refusal tells a
// unit what its neighbours have.
func TestSyncNowReachesOnlyTheInvokingUnitsOwnJobs(t *testing.T) {
	composedJobsForTest(t, jobOf("alpha", "refresh"), jobOf("beta", "reconcile"))

	for _, probe := range []struct {
		what string
		job  string
	}{
		{"another unit's job", "reconcile"},
		{"a job nobody declares", "not_a_job"},
		{"the empty name", ""},
	} {
		t.Run(probe.what, func(t *testing.T) {
			rt := wiredRuntime("alpha")
			err := rt.SyncNow(context.Background(), extension.JobName(probe.job))
			if !errors.Is(err, extension.ErrNoSuchJob) {
				t.Errorf("err = %v, want ErrNoSuchJob — a unit reaches its own declarations and no others", err)
			}
		})
	}
}

// And the resolver DOES find a unit's own job, so the test above is about
// ownership rather than about every name being refused.
//
// Asserted on declaredJobFor rather than through SyncNow, because a name that
// resolves goes on to the queue — and what this states is which declarations
// the lookup can see, not what happens after it.
func TestTheJobLookupIsScopedToTheDeclaringUnit(t *testing.T) {
	composedJobsForTest(t, jobOf("alpha", "refresh"), jobOf("beta", "reconcile"))

	if _, found := declaredJobFor("alpha", "refresh"); !found {
		t.Error("a unit's own declared job did not resolve")
	}
	if _, found := declaredJobFor("beta", "reconcile"); !found {
		t.Error("the second unit's own declared job did not resolve")
	}
	// The pair that matters: each unit's name against the OTHER's job.
	if _, found := declaredJobFor("alpha", "reconcile"); found {
		t.Error("alpha resolved beta's job — the lookup is not scoped to the declaring unit")
	}
	if _, found := declaredJobFor("beta", "refresh"); found {
		t.Error("beta resolved alpha's job")
	}
}

// A released Runtime asks for nothing. The capability is call-scoped like every
// other, and a handler that stashed one and pressed save later must not be able
// to keep the scheduler busy.
func TestSyncNowFailsClosedAfterTheCallEnds(t *testing.T) {
	composedJobsForTest(t, jobOf("alpha", "refresh"))
	rt := wiredRuntime("alpha")
	rt.release()
	if err := rt.SyncNow(context.Background(), "refresh"); !errors.Is(err, extension.ErrRuntimeExpired) {
		t.Errorf("err = %v, want ErrRuntimeExpired", err)
	}
}

// The coalescing that turns a held-down save into one tick, asserted on the
// opts rather than on a queue: it is the uniqueness that does it, and the
// difference from the two neighbouring helpers is the whole reason this one
// exists.
func TestAnAskedForRunCoalescesAndIsNotAFleetPass(t *testing.T) {
	// A real composed child kind, because fanOutChildSpec reads the
	// declaration table and a made-up kind would panic rather than answer.
	kinds := fanOutChildren()
	if len(kinds) == 0 {
		t.Fatal("this build declares no fan-out child kinds, so this test would prove nothing")
	}
	var child string
	for kind := range kinds {
		if fanOutChildSpec(kind).OptsOwner == jobs.OptsFanOut {
			child = kind
			break
		}
	}
	if child == "" {
		t.Fatal("no fan-out-owned child kind in this build")
	}

	asked, err := attendedChildOpts(child)
	if err != nil {
		t.Fatalf("reading the opts for %s: %v", child, err)
	}
	if !asked.UniqueOpts.ByArgs || len(asked.UniqueOpts.ByState) == 0 {
		t.Error("an asked-for run does not coalesce: a member holding down save enqueues a tick per press")
	}
	// Not a fleet pass. The sweep gauges count rows carrying jobs.SweepTag, and
	// a run somebody asked for would inflate the coverage reading of a pass
	// that never ran.
	//
	// The TAG, by name, rather than a count of tags: a count is silent about
	// which tags those are, so an asked-for run that later grew a second tag
	// beside the sweep one would read as different-and-therefore-fine. It is
	// also what the gauge reads — sweepTagPredicate is `'sweep' = ANY(tags)` —
	// so this asks the question the gauge asks.
	fleet := workspaceSweepOpts(child)
	if !slices.Contains(fleet.Tags, jobs.SweepTag) {
		t.Fatalf("the fleet sweep's opts carry tags %v, not the %q this test is checking the asked-for run is free of — "+
			"the marker moved and this assertion is no longer about anything", fleet.Tags, jobs.SweepTag)
	}
	if slices.Contains(asked.Tags, jobs.SweepTag) {
		t.Errorf("an asked-for run carries %q, so the sweep gauges count it as coverage of a pass that never ran", jobs.SweepTag)
	}
	// Same queue and attempt cap as every other share of this kind: one
	// workspace's tick is one workspace's tick however it was asked for.
	if asked.Queue != fleet.Queue || asked.MaxAttempts != fleet.MaxAttempts {
		t.Errorf("an asked-for run routes to %s/%d, the clock's to %s/%d — one kind, two routings",
			asked.Queue, asked.MaxAttempts, fleet.Queue, fleet.MaxAttempts)
	}
}
