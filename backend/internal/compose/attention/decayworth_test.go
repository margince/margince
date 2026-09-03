// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a lapsed relationship was WORTH, and what the queue does with it.
//
// The lane could say only how long a silence had run, so the rep's strongest
// contact going quiet and a cc drifting arrived as the same row at the same
// rank. Both facts that tell them apart were already in the lane's hand and
// discarded: the band is scored from the edge the projection loads, and the
// open deal is one batched read over candidates it already narrowed to.
//
// These run the classifier directly. The question is what the facts BECOME —
// the rank the row takes and the reason it states — not how they were measured.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func decayItem(facts *crmcontracts.AttentionRelationshipFacts) crmcontracts.AttentionItem {
	personID := ids.NewV7()
	name := "Dana Weiss"
	days := 63
	return crmcontracts.AttentionItem{
		Id:           personID.String(),
		Source:       "relationship_decay",
		Title:        &name,
		QuietDays:    &days,
		Relationship: facts,
		Subject:      subjectOf("person", personID),
		Actions:      []crmcontracts.AttentionItemActions{},
	}
}

func band(b crmcontracts.AttentionRelationshipFactsStrength) *crmcontracts.AttentionRelationshipFactsStrength {
	return &b
}

func openDeal(has bool) *bool { return &has }

func hasReason(row crmcontracts.WorklistItem, kind crmcontracts.WorklistReasonKind) bool {
	for _, because := range row.Because {
		if because.Kind == kind {
			return true
		}
	}
	return false
}

// The rank, across the whole band vocabulary plus the deal. This is the table
// that decides which silences a rep sees above the routine tidying, so it is
// written as one rather than as four tests that could each drift.
func TestWhatALapsedRelationshipWasWorthDecidesItsRank(t *testing.T) {
	for _, c := range []struct {
		name  string
		facts *crmcontracts.AttentionRelationshipFacts
		want  int
	}{
		{
			name:  "a strong relationship going quiet",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: band("strong")},
			want:  levelAgreed,
		},
		{
			// Moderate counts, and that is the deliberate edge of this rule:
			// §4's moderate threshold already means a real two-way history,
			// and admitting only `strong` would leave the lane ranking almost
			// nothing.
			name:  "a moderate one, the edge of the rule",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: band("moderate")},
			want:  levelAgreed,
		},
		{
			name:  "a weak one with nothing resting on it",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: band("weak"), HasOpenDeal: openDeal(false)},
			want:  levelRoutine,
		},
		{
			// The case the band alone would get wrong. A contact barely spoken
			// to who still sits on an open deal is money going quiet, and it is
			// exactly the silence nobody notices.
			name:  "a weak one an open deal still rests on",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: band("weak"), HasOpenDeal: openDeal(true)},
			want:  levelAgreed,
		},
		{
			name:  "a relationship §4 scored at nothing",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: band("none")},
			want:  levelRoutine,
		},
		{
			// A lane that sent no facts at all. Absent is not a claim: an
			// installation whose seam predates these fields keeps the routine
			// rank it has rather than having every silence promoted on a field
			// nobody filled.
			name:  "a lane that measured nothing",
			facts: nil,
			want:  levelRoutine,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			row := classifyDecay(decayItem(c.facts), rankInstant)
			if row.item.Level != c.want {
				t.Fatalf("level = %d, want %d", row.item.Level, c.want)
			}
		})
	}
}

// The row says why it outranks the routine ones. A rank a reader cannot account
// for is the complaint this whole campaign started from.
func TestALapsedRelationshipWithMoneyOnItSaysSo(t *testing.T) {
	row := classifyDecay(decayItem(&crmcontracts.AttentionRelationshipFacts{
		Strength: band("weak"), HasOpenDeal: openDeal(true),
	}), rankInstant)

	if !hasReason(row.item, "expected_revenue") {
		t.Fatalf("a lapsed contact with an open deal states %v, want it to say money rests on this",
			row.item.Because)
	}
}

// And a silence with nothing on it does NOT say money rests on it. The reason
// renders as "expected revenue" with no figure, so claiming it for a contact
// carrying no deal is the row inventing a fact.
func TestALapsedRelationshipWithNoDealClaimsNoRevenue(t *testing.T) {
	row := classifyDecay(decayItem(&crmcontracts.AttentionRelationshipFacts{
		Strength: band("strong"), HasOpenDeal: openDeal(false),
	}), rankInstant)

	if hasReason(row.item, "expected_revenue") {
		t.Fatalf("a contact with no open deal claims revenue: %v", row.item.Because)
	}
}

// The band moves the rank and states NOTHING. `no_champion` is the nearest word
// the reason vocabulary has and it means the opposite here — an account with
// nobody carrying it, rather than the person who WAS carrying it going quiet.
//
// This is a test for a deliberate silence, so it is written to fail if somebody
// reaches for that word: a reason that reads backwards is worse than a rank the
// row does not explain.
func TestAStrongLapsedRelationshipNeverClaimsTheAccountHasNoChampion(t *testing.T) {
	row := classifyDecay(decayItem(&crmcontracts.AttentionRelationshipFacts{
		Strength: band("strong"),
	}), rankInstant)

	if hasReason(row.item, "no_champion") {
		t.Fatalf("a decayed champion is reported as the account having none: %v", row.item.Because)
	}
}

// The wire mapping refuses a band this contract does not declare, rather than
// casting it through. The two vocabularies are declared in different places, so
// a cast would widen the wire silently the day either grows a term and reach a
// reader as a word their client cannot translate.
func TestABandThisContractDoesNotDeclareReachesNoReader(t *testing.T) {
	if got := relationshipBand("legendary"); got != nil {
		t.Fatalf("an undeclared band reached the wire as %q", *got)
	}
	for _, bucket := range []string{"none", "weak", "moderate", "strong"} {
		if got := relationshipBand(bucket); got == nil || string(*got) != bucket {
			t.Fatalf("the §4 bucket %q did not reach the wire", bucket)
		}
	}
}
