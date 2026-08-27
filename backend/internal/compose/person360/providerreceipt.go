// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// What a lookup actually bought.
//
// A run that asked for six categories and returned one used to render exactly
// like one that returned all six: whatever arrived, under a green badge, with
// nothing saying how much of the question went unanswered. This file computes
// the difference, which is what lets the section report it.

import (
	"sort"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// categoriesWithoutAnswer is what the latest run PAID to ask for and the
// provider returned nothing for.
//
// The counterpart to categoriesNotRequested, and the half that was missing: a
// blank field could mean nobody bought it or that the vendor had none, and only
// naming both tells them apart. A run that answered one category out of six
// rendered as a plain success with five silent blanks, which is how an empty
// purchase looked exactly like a full one.
//
// Read from the claims THIS run delivered rather than from the profile's folded
// values: the fold is a union over every retained run, so an older run's answer
// would mask what the latest one failed to return.
func categoriesWithoutAnswer(desc provider.Descriptor, requested []string, delivered map[string]bool) []string {
	out := []string{}
	for _, name := range requested {
		category := provider.Category(name)
		keys, declared := desc.Answers[category]
		if !declared {
			// The adapter never said what answers this category, so nothing
			// here can conclude the provider withheld it.
			continue
		}
		if answeredBy(keys, delivered) {
			continue
		}
		if !issued(desc, category, delivered) {
			// A fallback that was never issued was never asked, so the
			// provider had no chance to answer it. Surfe's personal-email pass
			// runs only when the professional one comes back empty; listing it
			// as "asked for and not found" would report a lookup that did not
			// happen, on a line whose whole job is telling the reader what
			// their money actually bought.
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// answeredBy reports whether any of a category's claim keys arrived.
func answeredBy(keys []provider.ClaimKey, delivered map[string]bool) bool {
	for _, key := range keys {
		if delivered[string(key)] {
			return true
		}
	}
	return false
}

// issued reports whether the run actually put this category to the provider.
//
// Two ways it did not, and they are mirrors of each other. A cascade fires only
// when the category it follows comes back EMPTY, so a satisfied trigger means
// the fallback was never sent. A category with a prerequisite is skipped unless
// its prerequisite came back FULL — Surfe asks for no mobile when it found no
// email, because a subject it could not place has no number either.
//
// Either way no request left the building, so the provider had no chance to
// answer and must not be reported as having had nothing.
func issued(desc provider.Descriptor, category provider.Category, delivered map[string]bool) bool {
	for _, cascade := range desc.Cascades {
		if cascade.Category == category && answeredBy(desc.Answers[cascade.After], delivered) {
			return false
		}
	}
	if prerequisite, ok := desc.RequiresAnswerTo[category]; ok {
		return answeredBy(desc.Answers[prerequisite], delivered)
	}
	return true
}

// answerable reports whether this run's silence is the PROVIDER's answer.
//
// Only a completed run whose claims were written says anything about what the
// vendor had. A queued or in-flight run has no claims yet, a skipped one never
// called at all, and a failed or submission-unknown one never learned. Reading
// any of those as "asked for and not found" reports a verdict nobody reached —
// worst of all for completed_claims_unwritten, where the provider DID answer
// and the hand-off dropped it, which would blame the vendor for our own defect.
func answerable(run providerRunRow) bool {
	return run.state == string(provider.RunCompleted) && !run.claimsUnwritten
}

// deliveredKeys is the set of claim keys one run actually produced.
func deliveredKeys(runID ids.UUID, claims []storedClaim) map[string]bool {
	delivered := map[string]bool{}
	for _, c := range claims {
		if c.runID == runID {
			delivered[c.key] = true
		}
	}
	return delivered
}
