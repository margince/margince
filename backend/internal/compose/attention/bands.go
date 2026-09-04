// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The four outcome bands a reader sees as headings.
//
// A LEVEL says what kind of work a row is — somebody else's clock, a promise
// the product is breaking, revenue at risk. A BAND says what the reader is
// being asked to do about it today, which is the question a page organizes
// around. Seven levels make a correct ordering and a poor set of headings: a
// reader scanning for "what must happen now" should not have to know that
// levels 1 through 3 mean that and 4 through 6 do not.
//
// Derived, never stored. A band is a function of the level and the source the
// row already carries, so it cannot disagree with the ordering — and a row
// whose level changes takes its heading with it.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The bands, in the order a page draws them.
const (
	// bandNow is an external clock or material revenue outcome that needs
	// action today: the buyer waiting, the meeting about to start, the deal
	// past its close date.
	bandNow = "now"
	// bandBuildPipeline is work that CREATES pipeline rather than protecting
	// it — a lead to qualify, a follow-up that keeps a conversation alive.
	bandBuildPipeline = "build_pipeline"
	// bandKeepMomentum is agreed work that should move, and expires today for
	// nobody: an assigned task, a deal drifting below the material bar.
	bandKeepMomentum = "keep_momentum"
	// bandReview is judgement and hygiene — approvals, duplicate pairs, and
	// everything the product reports about itself.
	bandReview = "review"
)

// bandOrder is the order a page draws the bands in, and the order the ranking
// already produces. Spelled here so the client is told the sequence rather than
// inferring it from the rows it happens to receive: a day with no `now` rows
// must still draw its remaining bands in this order.
var bandOrder = []string{bandNow, bandBuildPipeline, bandKeepMomentum, bandReview}

// bandOfRow says which heading a row sits under.
//
// The LEVEL leads, because the ordering already sorts by it and a band that
// disagreed with the ordering would draw a heading over rows that had sorted
// somewhere else. The source only refines what the level cannot see: level 4
// covers both an assigned task and a lead's follow-up, and those are different
// answers to "what am I being asked to do".
//
// CROWDING CHANGES THE BAND, and this is where two right rules meet. Past the
// lead group a wait is demoted so a hundred replies cannot own the page — that
// is the anti-monopoly rule, and it has to move the row a long way down.
// Headings have to stay contiguous, or the page draws "Now" twice with other
// work in between. Demoting the row's POSITION while leaving its heading
// alone cannot satisfy both.
//
// So a crowded row is not `now` work any more. It is still a customer waiting,
// still says so on its face, and still sits above the hygiene: what changed is
// the claim that it needs answering today, which is exactly what being the
// ninth of its kind means.
func bandOfRow(row ranked) string {
	item := row.item
	// A pinned row is whatever the reader said it was, and they put it at the
	// top: it leads the page, so it heads the first band.
	//
	// BEFORE the crowding test, because the pin step sorts before the crowding
	// step. Asked the other way round a pinned row that had also been crowded
	// would sort first and be headed `keep_momentum` — the page drawing that
	// heading above its own Now band, and the reader's one override putting a
	// row where they asked while telling them it was somewhere else.
	if item.Level == levelPinned {
		return bandNow
	}
	if row.crowded {
		return bandKeepMomentum
	}
	if item.Level <= levelWaiting {
		return bandNow
	}
	if item.Level <= levelMaterialRisk {
		// A promise the product is breaking, and revenue worth interrupting
		// for. Both are today's work by the level's own definition.
		return bandNow
	}
	// THE LEVEL DECIDES BEFORE THE SOURCE DOES, and the order matters because
	// the bands sort above the level in the ordering: a band chosen against the
	// level would let a level-6 row lead a level-5 one, which the contract
	// forbids and a reader would read as the page ranking hygiene over a
	// blocked decision.
	//
	// levelRoutine IS the hygiene level. A stale unfunded wait is demoted to it
	// by classifyWaiting — whose own comment says the row "belongs to review,
	// not to today" — and banding it by source instead would put it above the
	// blocking decisions it was demoted past.
	if item.Level >= levelRoutine {
		return bandReview
	}
	switch categoryOfSource(item.Source) {
	case categoryDecisions, categorySystem:
		// Judgement and hygiene that arrive ABOVE the routine level: a duplicate
		// pair or a broken pipe is review work whatever rank it carries.
		return bandReview
	}
	if buildsPipeline(item) {
		return bandBuildPipeline
	}
	return bandKeepMomentum
}

// categoryDecisions is the cut a row asking for judgement lands in.
// categorySystem is already spelled in batches.go and is reused.
const categoryDecisions = crmcontracts.WorklistItemCategory("decisions")

// buildsPipeline reports whether a row's work CREATES pipeline rather than
// protecting what exists.
//
// The distinction the level cannot make: an assigned task and a lead's
// follow-up are both agreed work, and a rep choosing what to do next wants them
// apart. Read off the record the row is about, because that is what says
// whether there is a deal yet.
func buildsPipeline(item crmcontracts.WorklistItem) bool {
	return item.Subject != nil && item.Subject.Type == subjectLead
}

// subjectLead is the subject kind a row filed under a lead carries. Tasks reach
// it through primaryLink, which ranks a lead first: a task raised for a lead is
// ABOUT that lead even when the row also names the company it came from.
const subjectLead = crmcontracts.AttentionSubjectType("lead")

// bandsOf summarizes the headings for one page.
//
// Every band, in draw order, even where the page has no rows under it: a reader
// whose Now band is empty is being told something, and a client that inferred
// the headings from the rows it received could not say it.
func bandsOf(items []crmcontracts.WorklistItem) []crmcontracts.WorklistBand {
	shown := map[string]int{}
	for _, item := range items {
		if item.Band != nil {
			shown[string(*item.Band)]++
		}
	}
	out := make([]crmcontracts.WorklistBand, 0, len(bandOrder))
	for _, band := range bandOrder {
		out = append(out, crmcontracts.WorklistBand{
			Band:  crmcontracts.WorklistBandBand(band),
			Shown: shown[band],
		})
	}
	return out
}

// bandRank is a band's place in the draw order, for the ordering to compare.
// A band this build does not know sorts last rather than first: an unknown
// heading must not push real work off the top of the page.
func bandRank(band string) int {
	for i, known := range bandOrder {
		if known == band {
			return i
		}
	}
	return len(bandOrder)
}
