// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func decayItem(facts *crmcontracts.AttentionRelationshipFacts) crmcontracts.AttentionItem {
	contactID := ids.NewV7()
	name := "Dana Weiss"
	days := 63
	return crmcontracts.AttentionItem{
		Id:           contactID.String(),
		Source:       "relationship_decay",
		Title:        &name,
		QuietDays:    &days,
		Relationship: facts,
		Subject:      subjectOf("contact", contactID),
		Actions:      []crmcontracts.AttentionItemActions{},
	}
}

func TestRelationshipHistoryDoesNotCreateAnObligation(t *testing.T) {
	for _, c := range []struct {
		name  string
		facts *crmcontracts.AttentionRelationshipFacts
		want  int
	}{
		{
			name:  "a strong relationship going quiet",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: new(crmcontracts.AttentionRelationshipFactsStrength("strong"))},
			want:  levelRoutine,
		},
		{
			name:  "a moderate one, the edge of the rule",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: new(crmcontracts.AttentionRelationshipFactsStrength("moderate"))},
			want:  levelRoutine,
		},
		{
			name:  "a weak one with nothing resting on it",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: new(crmcontracts.AttentionRelationshipFactsStrength("weak")), HasOpenDeal: new(false)},
			want:  levelRoutine,
		},
		{
			name:  "a weak contact linked to an open deal",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: new(crmcontracts.AttentionRelationshipFactsStrength("weak")), HasOpenDeal: new(true)},
			want:  levelRoutine,
		},
		{
			name:  "a relationship §4 scored at nothing",
			facts: &crmcontracts.AttentionRelationshipFacts{Strength: new(crmcontracts.AttentionRelationshipFactsStrength("none"))},
			want:  levelRoutine,
		},
		{
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

func TestAnOpenDealDoesNotProveAReconnectionDependency(t *testing.T) {
	row := classifyDecay(decayItem(&crmcontracts.AttentionRelationshipFacts{HasOpenDeal: new(true)}), rankInstant)
	if hasReason(row.item, "expected_revenue") || row.item.Level != levelRoutine {
		t.Fatalf("open-deal link manufactured work: %+v", row.item)
	}
}

func TestALapsedRelationshipWithNoDealClaimsNoRevenue(t *testing.T) {
	row := classifyDecay(decayItem(&crmcontracts.AttentionRelationshipFacts{
		Strength: new(crmcontracts.AttentionRelationshipFactsStrength("strong")), HasOpenDeal: new(false),
	}), rankInstant)

	if hasReason(row.item, "expected_revenue") {
		t.Fatalf("a contact with no open deal claims revenue: %v", row.item.Because)
	}
}

func TestAStrongLapsedRelationshipNeverClaimsTheAccountHasNoChampion(t *testing.T) {
	row := classifyDecay(decayItem(&crmcontracts.AttentionRelationshipFacts{
		Strength: new(crmcontracts.AttentionRelationshipFactsStrength("strong")),
	}), rankInstant)

	if hasReason(row.item, "no_champion") {
		t.Fatalf("a decayed champion is reported as the account having none: %v", row.item.Because)
	}
}

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

func TestALapsedRelationshipOffersADraftReconnect(t *testing.T) {
	row := classifyDecay(crmcontracts.AttentionItem{
		Source:       crmcontracts.AttentionItemSourceRelationshipDecay,
		Relationship: &crmcontracts.AttentionRelationshipFacts{Strength: new(crmcontracts.AttentionRelationshipFactsStrength("strong"))},
	}, rankInstant)

	if row.item.Move == nil {
		t.Fatal("the row offers no move, so the lane still only lets a reader dismiss it")
	}
	if row.item.Move.Action != crmcontracts.WorklistMoveActionDraftEmail {
		t.Errorf("the move is %q, want draft_email", row.item.Move.Action)
	}
	if row.item.Move.ActivityId != nil {
		t.Error("the reconnect names an activity, so the queue draws it as a reply " +
			"to a message that does not exist")
	}
}
