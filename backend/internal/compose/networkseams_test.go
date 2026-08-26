// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The relationship-graph seam mappings, where a shape decision is made that a
// model reads.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/network"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// An uncovered deal answers empty ARRAYS, not nulls.
//
// "No stakeholder seats and nobody from our side" is the most useful answer this
// tool gives — it is the one that says the deal rests on nothing — and a model
// handed `null` reads it as "unknown" and hedges. Every sibling read on this
// surface normalizes; found by UAT on the one that did not.
func TestAnUncoveredDealAnswersEmptyArraysNotNulls(t *testing.T) {
	answer := toAgentCoverage(network.DealCoverage{DealID: ids.NewV7()}, nil, nil)

	raw, err := json.Marshal(answer)
	if err != nil {
		t.Fatalf("marshalling the coverage answer: %v", err)
	}
	for _, member := range []string{`"stakeholders":[]`, `"our_side":[]`, `"risks":[]`} {
		if !strings.Contains(string(raw), member) {
			t.Errorf("payload = %s, want %s — a null reads to a model as \"unknown\", which is a "+
				"different claim from \"nobody\"", raw, member)
		}
	}
}

// A stakeholder is NAMED, not left as a uuid.
//
// The tool's whole question is which named human is missing from a deal. A seat
// reading "economic_buyer, engaged: false" against a bare uuid has not answered
// it — a rep cannot act on an id, and a model cannot put one in a sentence.
// Found reviewing case 5 on real data, where account_coverage reported a
// disengaged economic buyer and never said Athina Kanioura.
func TestAStakeholderSeatCarriesTheirName(t *testing.T) {
	athina, jim := ids.NewV7(), ids.NewV7()
	answer := toAgentCoverage(network.DealCoverage{
		DealID: ids.NewV7(),
		Stakeholders: []deals.DealStakeholder{
			{PersonID: athina, Role: "economic_buyer", Engaged: false},
			{PersonID: jim, Role: "champion", Engaged: true},
		},
	}, nil, map[ids.UUID]string{athina: "Athina Kanioura", jim: "Jim Roth"})

	if len(answer.Stakeholders) != 2 {
		t.Fatalf("seated %d stakeholders, want 2", len(answer.Stakeholders))
	}
	if answer.Stakeholders[0].PersonName != "Athina Kanioura" {
		t.Errorf("the economic buyer is unnamed: %+v", answer.Stakeholders[0])
	}
	if answer.Stakeholders[0].PersonID != athina {
		t.Error("naming a seat lost its id, which is the handle for a follow-up read")
	}
}

// A person the caller may not read keeps their SEAT and loses only their name.
//
// Dropping the seat would hide that the deal has an economic buyer at all,
// which is the opposite of what a coverage answer is for: how many people carry
// a deal is not the secret, only who they are.
func TestAnUnreadablePersonKeepsTheirSeat(t *testing.T) {
	hidden := ids.NewV7()
	answer := toAgentCoverage(network.DealCoverage{
		DealID:       ids.NewV7(),
		Stakeholders: []deals.DealStakeholder{{PersonID: hidden, Role: "economic_buyer"}},
	}, nil, map[ids.UUID]string{})

	if len(answer.Stakeholders) != 1 {
		t.Fatalf("a seat vanished with its name: %+v", answer.Stakeholders)
	}
	if answer.Stakeholders[0].PersonName != "" {
		t.Errorf("a name was invented for an unreadable person: %q", answer.Stakeholders[0].PersonName)
	}
	if answer.Stakeholders[0].Role != "economic_buyer" {
		t.Error("the role was lost, so the gap is no longer legible")
	}
}

// A finding names the people it is about, so the sentence a model writes has a
// person in it rather than a uuid.
func TestAFindingNamesThePeopleItIsAbout(t *testing.T) {
	jim := ids.NewV7()
	risks := toAgentRisks([]network.Risk{{
		Kind: "single_threaded_theirs", Summary: "the deal rests on one relationship",
		PersonIDs: []ids.UUID{jim},
	}}, map[ids.UUID]string{jim: "Jim Roth"})

	if len(risks) != 1 {
		t.Fatalf("mapped %d risks, want 1", len(risks))
	}
	if len(risks[0].PersonNames) != 1 || risks[0].PersonNames[0] != "Jim Roth" {
		t.Errorf("the finding names nobody: %+v", risks[0])
	}
	if len(risks[0].PersonIDs) != 1 {
		t.Error("naming lost the ids, which stay the handle")
	}
}

// The names on a finding are a SET, never a parallel array.
//
// A private contact resolves to no name, so pairing the two lists by index
// would attach the wrong person's name to the wrong id the first time that
// happens — silently, and in a sentence a rep then repeats.
func TestFindingNamesAreNotPositionallyPairedWithIDs(t *testing.T) {
	hidden, jim := ids.NewV7(), ids.NewV7()
	risks := toAgentRisks([]network.Risk{{
		Kind: "single_threaded_theirs", Summary: "the deal rests on one relationship",
		PersonIDs: []ids.UUID{hidden, jim},
	}}, map[ids.UUID]string{jim: "Jim Roth"})

	if len(risks[0].PersonIDs) != 2 {
		t.Fatalf("an id was dropped: %+v", risks[0].PersonIDs)
	}
	if len(risks[0].PersonNames) != 1 || risks[0].PersonNames[0] != "Jim Roth" {
		t.Fatalf("names %+v — the readable person must be named and the hidden one skipped",
			risks[0].PersonNames)
	}
}

// No name resolved at all omits the field rather than shipping [""].
func TestAFindingWithNoReadableNamesOmitsThem(t *testing.T) {
	risks := toAgentRisks([]network.Risk{{
		Kind: "going_cold", Summary: "nobody is talking", PersonIDs: []ids.UUID{ids.NewV7()},
	}}, map[ids.UUID]string{})

	if risks[0].PersonNames != nil {
		t.Errorf("empty names shipped as %+v rather than being omitted", risks[0].PersonNames)
	}
}
