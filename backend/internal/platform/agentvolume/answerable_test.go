// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"testing"
	"time"
)

// A meter that cannot reach its counter store is the state this signal exists
// for: every governed counter reports its threshold passed, so the whole agent
// surface refuses while the pod reports healthy.
func TestAMeterWithNoStoreReportsItselfUnanswerable(t *testing.T) {
	t.Parallel()
	reach := New(nil, Limits{}, time.Minute).Answerable()
	if !reach.Bound {
		t.Error("a meter composed to bound agent reads reports no bound; an operator would read that as a role that never claimed to")
	}
	if reach.Reachable {
		t.Error("a meter with no counter store reports itself answerable, which is the fail-closed state going unseen")
	}
}

// Unmetered is a DECLARATION, not a fault. Reporting it as unreachable would
// alert an operator about a control this role was never asked to run.
func TestAnUnmeteredMeterReportsNoBoundRatherThanAFailingOne(t *testing.T) {
	t.Parallel()
	reach := Unmetered().Answerable()
	if reach.Bound {
		t.Error("Unmetered reports a bound; it is the composition that declared there is none")
	}
	if reach.Reachable {
		t.Error("Unmetered reports a reachable store; there is no store and no question")
	}
}

// A role that composed no meter at all is the same statement as Unmetered, and
// must not take the scrape down to say it.
func TestANilMeterAnswersRatherThanPanics(t *testing.T) {
	t.Parallel()
	var none *Meter
	if reach := none.Answerable(); reach.Bound || reach.Reachable {
		t.Errorf("a role that wired no meter reports %+v, want no bound and nothing to reach", reach)
	}
}
