// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a lapsed relationship is WORTH, and therefore how high it sits.
//
// Apart from the other classifiers for the reason classifyrisk.go is: those
// rank a clock, and this one weighs the relationship the clock ran out on.
// Nobody is waiting on the reader for a silence, which is exactly why it goes
// unnoticed — so the lane exists at all, and the question it has to answer is
// which of these silences the rep should have noticed first.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// classifyDecay: a relationship going quiet. Nobody is waiting on the reader
// for it, which is exactly why it goes unnoticed — and why it sits low rather
// than not at all.
//
// How low depends on what the relationship was WORTH. One that still carries
// money, or that was real before it went silent, is agreed work; a weak one
// with nothing on it is routine. The same shape classifyWaiting uses for a
// stale wait, and for the same reason: a lane where every row ranks alike is
// one a rep reads once, and the rep's strongest contact going silent is not
// the same morning's work as a cc drifting.
func classifyDecay(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	level := levelRoutine
	if decayMatters(item.Relationship) {
		level = levelAgreed
	}
	row := base(item, level, "system", "data_drifts")
	quiet := quietDaysOf(item)
	if quiet > 0 {
		row.Because = append(row.Because, reason("quiet_days", daysValue(quiet)))
	}
	// Why this silence outranks the others, in the vocabulary the contract
	// already declares. Only the deal says so out loud: `expected_revenue`
	// reads as "money rests on this" with no figure, which is exactly the claim.
	//
	// The BAND deliberately states nothing. It moves the level, and the reason
	// vocabulary has no word for it — `no_champion` is the nearest and means
	// the opposite here, an account with nobody carrying it rather than a
	// person who WAS carrying it. A reason that reads backwards is worse than
	// a rank the row does not explain, so the band waits for a word of its own.
	if facts := item.Relationship; facts != nil && facts.HasOpenDeal != nil && *facts.HasOpenDeal {
		row.Because = append(row.Because, reason("expected_revenue", nil))
	}
	return ranked{item: row, waitingDays: quiet, occurredAt: occurredOf(item, asOf)}
}

// decayMatters says whether a lapsed relationship is worth more than routine
// attention: money still resting on it, or a real relationship behind it.
//
// A lane that sent no facts answers false rather than true. Absent is not a
// claim, and promoting every silence on a field nobody filled would empty the
// routine band for a reason no reader could see.
func decayMatters(facts *crmcontracts.AttentionRelationshipFacts) bool {
	if facts == nil {
		return false
	}
	if facts.HasOpenDeal != nil && *facts.HasOpenDeal {
		return true
	}
	if facts.Strength == nil {
		return false
	}
	// Moderate counts. The §4 threshold for it already means a real two-way
	// history, and a rep losing those quietly is who this lane is for —
	// admitting only `strong` would leave it ranking almost nothing.
	return *facts.Strength == crmcontracts.AttentionRelationshipFactsStrengthModerate ||
		*facts.Strength == crmcontracts.AttentionRelationshipFactsStrengthStrong
}

// sourceDecay is the lane's own name on the wire.
//
// A constant rather than a literal at each match, because the failure mode of a
// typo here is silence: a suppressor comparing against a misspelt source simply
// never matches, so it suppresses nothing and every test that does not plant the
// duplicate row still passes.
const sourceDecay = crmcontracts.WorklistItemSource("relationship_decay")

// dropDecayAlreadyWaiting removes the lapsed-relationship row for a person the
// reader is already shown as waiting on.
//
// One person is one row. The two lanes say opposite things about the same
// contact and both are true: nobody has spoken in sixty days, AND that person
// wrote last week and is waiting for an answer. Drawn together they read as a
// contradiction, and the rep is left to work out which one to believe.
//
// The WAITING row wins, for the reason the drifting-deal suppressor gives: it
// is the more urgent and the more actionable of the two, because it names the
// message to reply to. A silence has no message to point at.
//
// The silence itself is NOT absorbed: "quiet 60 days" is a claim the waiting
// row contradicts rather than completes — the contact is not quiet, they are
// unanswered. But the money IS, and that half took a defect to notice. The two
// lanes answer different questions about a deal: a wait asks whether one rides
// on THIS THREAD, the decay lane asks whether the person sits on any open deal
// the reader can see. So a fifteen-day-old wait about a contact who carries a
// deal elsewhere loses `expected_revenue` when its decay row goes, and drops a
// band with it — the row falls out of the day for lack of a fact the page had.
func dropDecayAlreadyWaiting(rows []ranked) []ranked {
	waitingPeople := waitingContacts(rows)
	if len(waitingPeople) == 0 {
		return rows
	}
	// What the dropped rows were going to say about money, kept for the row
	// that replaces them.
	funded := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceDecay && waitingPeople[row.item.Id] &&
			hasReason(row.item, reasonExpectedRevenue) {
			funded[row.item.Id] = true
		}
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		// The decay row's id IS the person's id, which is what makes this
		// match at all: the lane has no activity to key on.
		if row.item.Source == sourceDecay && waitingPeople[row.item.Id] {
			continue
		}
		if row.item.Source == sourceWaiting && funded[row.person.String()] &&
			!hasReason(row.item, reasonExpectedRevenue) {
			row.item.Because = append(row.item.Because, reason(reasonExpectedRevenue, nil))
		}
		kept = append(kept, row)
	}
	return kept
}

// waitingContacts collects the contacts named by the waiting rows on this page.
//
// It reads the row's OWN person rather than its subject, and that distinction
// is the whole correctness of this pass. A wait carrying both a deal and a
// person takes the deal as its subject — the deal says more about what the
// reply is for — so a subject-keyed lookup misses exactly the contacts most
// likely to also be lapsing, and the page shows them twice.
func waitingContacts(rows []ranked) map[string]bool {
	people := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceWaiting && !row.person.IsZero() {
			people[row.person.String()] = true
		}
	}
	return people
}

// reasonExpectedRevenue says money still rests on this row.
const reasonExpectedRevenue = crmcontracts.WorklistReasonKind("expected_revenue")

// hasReason reports whether a row already states one fact.
func hasReason(item crmcontracts.WorklistItem, kind crmcontracts.WorklistReasonKind) bool {
	for _, because := range item.Because {
		if because.Kind == kind {
			return true
		}
	}
	return false
}
