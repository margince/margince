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

// sourceDecay is the lane's own name on the wire, spelled once here because
// three places now match on it and a typo in any of them would simply stop
// matching — a suppressor that silently suppresses nothing.
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
// Nothing is absorbed onto the survivor, unlike the deal case. What the decay
// row would have added — how long the silence ran — is a claim the waiting row
// contradicts rather than completes: the contact is not quiet, they are
// unanswered.
func dropDecayAlreadyWaiting(rows []ranked) []ranked {
	waitingPeople := map[string]bool{}
	for _, row := range rows {
		if row.item.Source == sourceWaiting && row.item.Subject != nil &&
			row.item.Subject.Type == subjectPerson {
			waitingPeople[row.item.Subject.Id.String()] = true
		}
	}
	if len(waitingPeople) == 0 {
		return rows
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		// The decay row's id IS the person's id, which is what makes this
		// match at all: the lane has no activity to key on.
		if row.item.Source == sourceDecay && waitingPeople[row.item.Id] {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}
