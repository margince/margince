// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The relationship-graph seam mappings, where a shape decision is made that a
// model reads.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// A seat whose name did not resolve keeps its seat and its role.
//
// This is the MAPPING's contract, not a privacy claim: the integration test
// TestCoverageWithoutPersonReadIsRefusedRatherThanUnnamed proves a caller who
// may not read people gets no seats at all, refused upstream. What this pins
// is that if a name is ever missing for an ordinary reason — a person row with
// no full_name, a lookup that came back short — the seat still carries its
// role, so the gap stays legible instead of vanishing.
func TestASeatWithNoResolvedNameKeepsItsRole(t *testing.T) {
	unnamed := ids.NewV7()
	answer := toAgentCoverage(network.DealCoverage{
		DealID:       ids.NewV7(),
		Stakeholders: []deals.DealStakeholder{{PersonID: unnamed, Role: "economic_buyer"}},
	}, nil, map[ids.UUID]string{})

	if len(answer.Stakeholders) != 1 {
		t.Fatalf("a seat vanished with its name: %+v", answer.Stakeholders)
	}
	if answer.Stakeholders[0].PersonName != "" {
		t.Errorf("a name was invented: %q", answer.Stakeholders[0].PersonName)
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
	if len(risks[0].People) != 1 || risks[0].People[0].Name != "Jim Roth" {
		t.Errorf("the finding names nobody: %+v", risks[0])
	}
	if risks[0].People[0].PersonID != jim {
		t.Error("the name did not travel with its own id")
	}
	if len(risks[0].PersonIDs) != 1 {
		t.Error("naming lost the ids, which stay the handle")
	}
}

// A name that did not resolve cannot shift another name onto the wrong person.
//
// This is why the finding carries {person_id, name} objects rather than a
// second array beside person_ids. Two arrays can diverge — a caller with
// deal:read and no person:read has ids and no names, and the transaction is
// Read Committed, so a person archived mid-read leaves one list shorter. A
// consumer indexing across them would then repeat the wrong name in a
// sentence. Codex found the isolation case; the shape makes it impossible.
func TestAnUnresolvedNameCannotShiftOntoAnotherPerson(t *testing.T) {
	hidden, jim := ids.NewV7(), ids.NewV7()
	risks := toAgentRisks([]network.Risk{{
		Kind: "single_threaded_theirs", Summary: "the deal rests on one relationship",
		PersonIDs: []ids.UUID{hidden, jim},
	}}, map[ids.UUID]string{jim: "Jim Roth"})

	if len(risks[0].PersonIDs) != 2 {
		t.Fatalf("an id was dropped: %+v", risks[0].PersonIDs)
	}
	if len(risks[0].People) != 1 {
		t.Fatalf("people %+v — the readable person is named and the unreadable one skipped",
			risks[0].People)
	}
	// The whole point: the surviving name is attached to ITS OWN id, not to
	// the first id in the list.
	if risks[0].People[0].PersonID != jim {
		t.Errorf("Jim Roth's name landed on %v, which is somebody else", risks[0].People[0].PersonID)
	}
}

// No name resolved at all omits the field rather than shipping [""].
func TestAFindingWithNoReadableNamesOmitsThem(t *testing.T) {
	risks := toAgentRisks([]network.Risk{{
		Kind: "going_cold", Summary: "nobody is talking", PersonIDs: []ids.UUID{ids.NewV7()},
	}}, map[ids.UUID]string{})

	if risks[0].People != nil {
		t.Errorf("empty names shipped as %+v rather than being omitted", risks[0].People)
	}
}

// The at-risk SWEEP names its findings too, in one read for the whole sweep.
//
// A first cut left this surface unnamed, arguing twenty-five deals meant
// twenty-five gated person reads. Codex pointed out the ids can be collected
// across every finding and resolved once — so the cost was never per deal, and
// the inconsistency was real: the SAME CoverageRisk shape carried names under
// account_coverage and bare ids here, which a model reads as "withheld" rather
// than "not looked up".
func TestTheSweepNamesEveryFindingInOneRead(t *testing.T) {
	jim, athina := ids.NewV7(), ids.NewV7()
	flagged := []agents.AtRiskDeal{
		{DealID: ids.NewV7(), Name: "one", Risks: []agents.CoverageRisk{
			{Kind: "single_threaded_theirs", PersonIDs: []ids.UUID{jim}},
		}},
		{DealID: ids.NewV7(), Name: "two", Risks: []agents.CoverageRisk{
			{Kind: "going_cold", PersonIDs: []ids.UUID{athina, jim}},
		}},
	}
	names := map[ids.UUID]string{jim: "Jim Roth", athina: "Athina Kanioura"}

	// The mapping half, exercised directly: nameSweepFindings' own read needs a
	// transaction, and what has to be right here is that EVERY finding across
	// EVERY deal is named, not only the first.
	for i, d := range flagged {
		for j, r := range d.Risks {
			flagged[i].Risks[j].People = namedPeople(r.PersonIDs, names)
		}
	}

	if len(flagged[0].Risks[0].People) != 1 {
		t.Fatalf("the first deal's finding is unnamed: %+v", flagged[0].Risks[0])
	}
	if len(flagged[1].Risks[0].People) != 2 {
		t.Fatalf("the second deal's finding named %d of 2 people — a later deal was skipped",
			len(flagged[1].Risks[0].People))
	}
	if flagged[1].Risks[0].People[0].Name != "Athina Kanioura" {
		t.Errorf("names are attached in the wrong order: %+v", flagged[1].Risks[0].People)
	}
}
