// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	"encoding/json"
	"strings"
	"testing"
)

// The account the brief is about is citable: the id the FILTER accepts has to
// be an id the MODEL was shown, or every company-level sentence is dropped
// whatever the model wrote.
func TestTheAccountTheBriefIsAboutIsCitable(t *testing.T) {
	t.Parallel()
	const companyID = "01a0a940-f295-74dd-9999-000000000001"
	in := Input{ID: companyID, Name: "Brandt Automotive GmbH", Industry: "Automotive", Strength: 41}

	// The id the FILTER accepts and the id the MODEL is shown have to be the
	// same one, so both are read here rather than compared to a literal.
	if _, held := knownRecords(companyID, in)[Evidence{EntityType: citeCompany, EntityID: companyID}]; !held {
		t.Fatalf("the grounding filter does not accept a citation of the account itself")
	}

	payload, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("encoding the summary: %v", err)
	}
	if !strings.Contains(string(payload), companyID) {
		t.Errorf("the summary the model reads does not carry the account's id, so it cannot cite "+
			"the account the prompt tells it to cite:\n%s", payload)
	}
}

// A sentence about the account survives the grounding filter when it cites the
// account by the id the summary gave it.
//
// The assertion above is about the wire; this one is about the consequence, and
// it is the half that would still fail if the id were carried but not accepted.
func TestASentenceCitingTheAccountSurvivesGrounding(t *testing.T) {
	t.Parallel()
	const companyID = "01a0a940-f295-74dd-9999-000000000002"
	in := Input{ID: companyID, Name: "Brandt Automotive GmbH", Industry: "Automotive", Strength: 41}

	reply := `{"sections":[{"kind":"snapshot","sentences":[{"text":"They make automotive parts.",` +
		`"nature":"fact","evidence":[{"entity_type":"company","entity_id":"` + companyID + `"}]}]}]}`

	sections, err := ParseBriefSections(reply, companyID, in)
	if err != nil {
		t.Fatalf("parsing a well-formed brief: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("a sentence citing the account by the id the summary supplied was dropped, so the " +
			"snapshot and health sections of every brief are unwritable by a model")
	}
}

// The id is evidence, never prose.
//
// Putting the account's id in the summary is what makes a citation possible and
// is also an invitation to write it into a sentence, where a reader sees an
// unreadable string. briefSystem forbids it and claims.Ungrounded enforces it;
// this is here so the fix above cannot be read as relaxing that.
func TestTheAccountsIdIsStillRefusedInProse(t *testing.T) {
	t.Parallel()
	const companyID = "01a0a940-f295-74dd-9999-000000000003"
	in := Input{ID: companyID, Name: "Brandt Automotive GmbH", Strength: 41}

	reply := `{"sections":[{"kind":"snapshot","sentences":[{"text":"Company ` + companyID + ` makes parts.",` +
		`"nature":"fact","evidence":[{"entity_type":"company","entity_id":"` + companyID + `"}]}]}]}`

	sections, err := ParseBriefSections(reply, companyID, in)
	if err != nil {
		t.Fatalf("parsing a well-formed brief: %v", err)
	}
	if len(sections) != 0 {
		t.Errorf("a sentence spelling the account's id in its TEXT was kept; the reader sees that "+
			"string: %+v", sections)
	}
}
