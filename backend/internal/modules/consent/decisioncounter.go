// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What this process decided at TRANSMIT about the mail it was asked to send.
//
// communication_decision is the durable record and the daily disagreement pass
// reads it, but neither answers "what is happening right now": one is a table an
// operator must think to query, the other a line in yesterday's log. A category
// that started refusing every message an hour ago looks, from outside, exactly
// like a quiet hour.
//
// TRANSMIT ONLY, and the omission is the honest half of this file. Staging
// records decisions too, but it runs on a transaction it does not own and an
// enforced refusal is delivered by rolling that transaction back, so a staging
// deny leaves no row behind. Counting it would publish a refusal rate the
// record cannot corroborate — under the shipped posture, where every category
// enforces, rather than in some corner. Transmit returns its refusal as a
// ticket instead: the transaction commits, the rows persist, and the count
// matches them.
//
// A staging decision taken under OBSERVE or WARN does commit, because
// refuseAtStaging lets a non-absolute denial through. Those rows are real and
// this counter does not see them. Counting only the outcomes that happen to
// survive would make the series mean different things in different postures,
// which is worse than a series that means one thing and says which. The daily
// pass and communication_decision are where the staging question is asked.
//
// BEST EFFORT, not exact correspondence. The count happens after the commit
// returns, so a process that dies in between leaves committed rows uncounted.
// A counter cannot be transactional with the database it describes; this is
// the direction to fail in, because it under-reports rather than inventing
// decisions no row holds.
//
// A COUNTER RATHER THAN A QUERY OVER THE TABLE, in memory rather than in SQL,
// for the reasons capture's outcome counter states and which hold here too.
// /metrics is process-global and binds no workspace, so a windowed query would
// read every tenant's decisions on every scrape. And "how many sends are being
// refused" is a rate an operator alerts on, which is what a monotonic counter is
// for.
//
// NO RECIPIENT, EVER. Decision carries the address it judged, and an address as
// a label would publish who was written to on an endpoint that binds no tenant
// and is unauthenticated unless a deployment sets a metrics token — unbounded
// in cardinality and a disclosure besides. The labels are three closed
// vocabularies: verdict, resolved category, mode. Held by
// TestTheDecisionCounterLabelsOnlyClosedVocabularies and
// TestTheRenderedLabelsAreOnlyTheDeclaredOnes
// (backend/gates/authzmetriclabels_test.go).

import (
	"sync"
	"sync/atomic"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// DecisionCount is one counted combination of the closed label values.
//
// No phase: every counted decision is a transmit one, so a phase label would
// carry the same value on every series and read as though a staging count were
// coming.
type DecisionCount struct {
	Verdict  commsauthz.Verdict
	Category commsauthz.Category
	Mode     commsauthz.Mode
}

// decisionTotals counts decisions RECORDED by this process, keyed by the label
// combination.
//
// A sync.Map for the reason capture's is: the key set stops growing once every
// live combination has been seen, so the read-mostly path costs no lock.
var decisionTotals sync.Map // DecisionCount -> *atomic.Uint64

// countDecision records one decision the transaction committed.
//
// Called AFTER the commit, from the rows the insert reported it added.
// AuthorizeTransmit has two error returns that fire once the decisions are
// already written — an unanswerable legacy gate, and the wording comparison —
// and both roll the rows back. A counter incremented beside the statement would
// keep those increments and report decisions no row holds.
//
// The insert takes ON CONFLICT DO NOTHING on
// (decision_set_id, recipient_address, phase), which is one address reached
// twice in a single set: a recipient on both the To and the Cc line is one
// decision, and counting per loop iteration would report two.
//
// A re-authorization is NOT that case and is not deduplicated: each call mints
// its own decision_set_id, so a redelivered job's second look is a second
// decision, recorded as one row and counted once. The engine did decide twice.
func countDecision(d commsauthz.Decision) {
	key := DecisionCount{Verdict: d.Verdict, Category: d.Resolved, Mode: d.Mode}
	stored, _ := decisionTotals.LoadOrStore(key, &atomic.Uint64{})
	if counter, isCounter := stored.(*atomic.Uint64); isCounter {
		counter.Add(1)
	}
}

// DecisionTotals reports this process's counts, for the metrics endpoint.
func DecisionTotals() map[DecisionCount]uint64 {
	out := map[DecisionCount]uint64{}
	decisionTotals.Range(func(key, value any) bool {
		count, isCount := key.(DecisionCount)
		counter, isCounter := value.(*atomic.Uint64)
		if isCount && isCounter {
			out[count] = counter.Load()
		}
		return true
	})
	return out
}
