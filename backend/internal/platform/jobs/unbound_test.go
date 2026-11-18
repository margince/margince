// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
)

func probeSpec(absentRegistersAnyway bool, when ...string) Spec {
	return Spec{Registration: Registration{When: when, AbsentRegistersAnyway: absentRegistersAnyway}}
}

// A kind that registers nothing without its model lane is a no-op while no
// model is bound, with no error for River to retry, and works once one is.
func TestAModelJobIsANoOpUntilAModelIsBound(t *testing.T) {
	var ran bool
	bound := false
	worker := QuietWhileUnbound[pausedProbe](pausedProbe{ran: &ran}, probeSpec(false, "ClassifyBrain"), func() bool { return bound })

	if err := worker.Work(context.Background(), &river.Job[pausedProbe]{}); err != nil || ran {
		t.Fatalf("unbound: Work = %v, ran = %v; want a clean no-op", err, ran)
	}
	bound = true
	if err := worker.Work(context.Background(), &river.Job[pausedProbe]{}); err != nil || !ran {
		t.Fatalf("bound: Work = %v, ran = %v; want the job worked", err, ran)
	}
}

func TestOnlyAJobThatRegistersNothingWithoutItsModelGoesQuiet(t *testing.T) {
	unbound := func() bool { return false }
	for name, spec := range map[string]Spec{
		"a queued request that registers anyway": probeSpec(true, "DeepReadBrain"),
		"a job with no model dependency":         probeSpec(false, "Blobstore"),
		"a job gated on nothing":                 probeSpec(false),
	} {
		var ran bool
		worker := QuietWhileUnbound[pausedProbe](pausedProbe{ran: &ran}, spec, unbound)
		if err := worker.Work(context.Background(), &river.Job[pausedProbe]{}); err != nil || !ran {
			t.Errorf("%s: Work = %v, ran = %v; want it worked while unbound", name, err, ran)
		}
	}
}

func TestNoBoundCheckLeavesTheWorkerAsItIs(t *testing.T) {
	var ran bool
	probe := pausedProbe{ran: &ran}
	if got := QuietWhileUnbound[pausedProbe](probe, probeSpec(false, "EnrichBrain"), nil); got != WorkOnly[pausedProbe](probe) {
		t.Fatalf("QuietWhileUnbound with no check wrapped the worker: %T", got)
	}
}
