// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func probeSpec(absentRegistersAnyway bool, when ...string) Spec {
	return Spec{Registration: Registration{When: when, AbsentRegistersAnyway: absentRegistersAnyway}}
}

// A gated model job is a no-op while no model is bound, with no error for River
// to retry. It works once one is.
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

type kindProbe string

func (k kindProbe) Kind() string { return string(k) }

// Every declared kind a user queues is refused with the typed problem while
// no model is bound, and every other kind is let through. The census comes from
// the declarations, so a new requested kind is covered without a new case.
func TestRequestedModelWorkIsRefusedWhileNoModelIsBound(t *testing.T) {
	unbound := (&Runner{}).WithModelBound(func() bool { return false })
	bound := (&Runner{}).WithModelBound(func() bool { return true })
	var requested int
	for kind, spec := range Declared() {
		want := isRequestedModelWork(spec.Registration.When)
		if want {
			requested++
		}
		if got := errors.Is(unbound.refuseUnbound(kind), httperr.ErrNotConfigured); got != want {
			t.Errorf("%s while unbound: refused = %v, want %v", kind, got, want)
		}
		if err := bound.refuseUnbound(kind); err != nil {
			t.Errorf("%s while bound: %v, want none", kind, err)
		}
		if err := (&Runner{}).refuseUnbound(kind); err != nil {
			t.Errorf("%s on a runner with no check: %v, want none", kind, err)
		}
	}
	if requested == 0 {
		t.Fatal("no declared kind is requested model work; the census is empty")
	}
	_, err := unbound.EnqueueTxUnique(context.Background(), nil, kindProbe("site_deep_read"), nil)
	var fault *httperr.DetailedError
	if !errors.As(err, &fault) || fault.Status != http.StatusConflict || fault.Code != httperr.CodeAINotConfigured {
		t.Fatalf("a deep read queued while unbound = %v, want 409 ai_not_configured", err)
	}
}
