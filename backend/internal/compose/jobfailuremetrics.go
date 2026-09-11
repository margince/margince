// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// WHAT is failing, in the vocabulary an operator already filters by.
//
// The gauges beside it say how much work is stuck. This says why, which is the
// difference between an outage somebody reads on a screen and one a monitor can
// act on: the only alertable signal before this was the discarded count going
// up, and that rises identically for a provider outage, a revoked credential
// and a bug. Those three want three different responses, and an alert that
// cannot tell them apart is one an operator learns to ignore.
//
// A GAUGE, though the ticket asked for a counter, and the reason is what the
// number is. Every family in this exposition is derived from the live job
// table, so this value falls when the rows are retried or cleaned — declaring
// it a counter would tell every rate() downstream to read a decrease as a
// counter reset and invent a spike. `margince_job_failures > 0` is the alert
// either way; a wrong type would break the arithmetic around it.

import (
	"cmp"
	"fmt"
	"io"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// unclassifiedFailureClass is the class a failure gets when nothing recognises
// its text.
//
// It counts. A failure in a shape nobody enumerated is exactly the one an
// outage arrives in, and dropping it would make that outage invisible to the
// alert as well as to the screen — the surface would go quiet in the one case
// it exists for. It costs one series, and it is reserved rather than derived so
// no unit can declare a class that silently merges with it.
const unclassifiedFailureClass = "unclassified"

// failureKey is one published series: a kind and the class of what went wrong.
type failureKey struct {
	kind  string
	class string
}

// writeJobFailureGauge renders the failing rows by kind and class.
//
// THE CLASS IS RESOLVED THE WAY THE SCREEN RESOLVES IT — jobs.VettedFailure,
// from the row's kind and its stored sentence — rather than from a second table
// here. Two classifiers over one vocabulary is two answers to one question, and
// the one an alert fired on would be the one nobody was reading.
//
// SERIES BOUND, stated because the exposition values a knowable cardinality and
// nothing was checking this one: at most (core classes + every composed unit's
// own + one reserved) × the kinds that are failing. Both halves of that product
// grow when somebody adds a unit, which is the growth
// TestTheFailureSeriesBoundIsWhatTheVocabulariesAllow turns from a surprise
// into a number a reviewer sees. Nothing here is keyed by workspace: a class is
// a fact about the failure, not about whose work it was, and the tenant split is
// already published by the gauges above.
func writeJobFailureGauge(w io.Writer, failures []jobs.FailureCount) error {
	if err := writeFamilyHeader(w, "margince_job_failures",
		"Jobs in a failed state (retryable or discarded) per kind and failure class. Cancelled work is counted apart by margince_job_cancelled: a deliberate stop is not an outage. The class is the same vocabulary the failure list shows, and "+unclassifiedFailureClass+" is a failure whose recorded text nothing recognises -- which is what an outage nobody has enumerated looks like."); err != nil {
		return err
	}
	series := map[failureKey]int64{}
	for _, f := range failures {
		series[failureKey{kind: f.Kind, class: failureClassOf(f)}] += f.Count
	}
	keys := sortedKeysOf(series, func(a, b failureKey) int {
		return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.class, b.class))
	})
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "margince_job_failures{kind=%s,class=%s} %d\n",
			label(k.kind), label(k.class), series[k]); err != nil {
			return err
		}
	}
	return nil
}

// failureClassOf answers the class token one group publishes under.
//
// A row recording NO cause and a row recording an unrecognised one both land on
// the reserved class, and that is a deliberate flattening: the difference
// matters to somebody reading one failure and not to an alert counting them,
// which is asking whether anything is failing in a shape nobody named.
func failureClassOf(f jobs.FailureCount) string {
	if detail, ok := jobs.VettedFailure(f.Kind, f.Stored); ok {
		return detail.Class
	}
	return unclassifiedFailureClass
}

// failureSeriesBound is the most series margince_job_failures can publish for a
// given set of failing kinds: every class any vocabulary declares, plus the
// reserved one, for each kind.
//
// It is deliberately an over-count — one kind's rows can only carry the core
// vocabulary and its own unit's, never another unit's — so a fleet that stayed
// under it proves the bound rather than merely agreeing with it.
func failureSeriesBound(kinds int) int {
	classes := len(jobs.CoreFailureClasses()) + 1
	for _, kind := range jobs.ComposedFailureKinds() {
		classes += len(jobs.ComposedFailureClasses(kind))
	}
	return kinds * classes
}
