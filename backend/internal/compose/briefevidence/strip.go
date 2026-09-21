// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefevidence

// Strip clears every email summary out of a value about to be written down.
//
// Two writers cache a shape that carries evidence — the deal status card and
// the account scan's settled findings — and both save a value the reader-side
// assembly may already have enriched. A summary that rode into either store
// would be served back to whoever reads the cache next, out of the audience and
// content grants of whoever filled it: the one reader-scoped field on an
// otherwise reader-independent row.
//
// Attach runs AFTER the save for exactly this reason, and Strip is the second
// lock — the one that still holds when a later writer forgets the ordering.
// Spelled once and shared by both writers rather than copied into each, because
// two spellings of "never persist this" is one spelling and one omission.
//
// Held by: TestNobodyClearsAnEmailSummaryByHand (backend/gates/briefevidencestrip_test.go),
// with TestEveryCacheOfGroundedProseStripsThroughTheSharedRule beside it holding
// that both writers reach the rule at all.
//
// It takes the same Targets the collectors produce, so a writer strips what a
// reader would have filled and the two can never name different fields.
func Strip(targets []Target) {
	for _, target := range targets {
		target.set(nil)
	}
}
