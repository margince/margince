// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

import (
	"encoding/json"
	"strings"
	"testing"
)

// The contact the brief is about is citable. Held here as well as in
// companybrief because these are two prompts and two filters: one being right
// says nothing about the other, and both were wrong.
func TestTheContactTheBriefIsAboutIsCitable(t *testing.T) {
	t.Parallel()
	const contactID = "01a0a93f-642c-708a-aaaa-000000000001"
	in := Input{ID: contactID, Name: "Anna Weber", Title: "Head of Legal", Strength: 60}

	if _, held := knownRecords(contactID, in)[Evidence{EntityType: citeContact, EntityID: contactID}]; !held {
		t.Fatal("the grounding filter does not accept a citation of the contact themselves")
	}

	payload, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("encoding the summary: %v", err)
	}
	if !strings.Contains(string(payload), contactID) {
		t.Errorf("the summary the model reads does not carry the contact's id, so it cannot cite "+
			"the contact this brief is about:\n%s", payload)
	}
}

// And a sentence that cites them survives the filter.
func TestASentenceCitingTheContactSurvivesGrounding(t *testing.T) {
	t.Parallel()
	const contactID = "01a0a93f-642c-708a-aaaa-000000000002"
	in := Input{ID: contactID, Name: "Anna Weber", Title: "Head of Legal", Strength: 60}

	reply := `{"sentences":[{"text":"They head legal.","nature":"fact",` +
		`"evidence":[{"entity_type":"contact","entity_id":"` + contactID + `"}]}]}`

	kept, err := ParseBrief(reply, contactID, in)
	if err != nil {
		t.Fatalf("parsing a well-formed brief: %v", err)
	}
	if len(kept) == 0 {
		t.Fatal("a sentence citing the contact by the id the summary supplied was dropped, so no " +
			"model-written sentence about this contact can ever survive")
	}
}
