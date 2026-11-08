// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
)

// cited is a reply scripted with record ids as the model writes it: each id
// the request showed spelled as its ref. An id the request never showed stays
// as written, which is how a model that invents one answers.
func cited(in Input, reply string) string { return shownRefs(in).Refer(reply) }

func parseCitedBrief(reply, contactID string, in Input) ([]Sentence, error) {
	return ParseBrief(cited(in, reply), contactID, in)
}

// The prompt shows the contact and every record by its ref. A reply citing a
// ref reads back to the record's id; the id itself, and a ref the request
// never minted, ground nothing.
func TestTheContactBriefShowsRefsAndReadsThemBackToRecords(t *testing.T) {
	in := inputFixture()
	shown := BriefRequest(in, "en").Messages[0].Content
	if claims.SpellsRecordID(shown) || !strings.Contains(shown, `"id":"c1"`) {
		t.Fatalf("the request must show refs and never an id:\n%s", shown)
	}
	for ref, want := range map[string]int{"c1": 1, briefContactID: 0, "c2": 0} {
		kept, err := ParseBrief(`{"sentences":[{"text":"Anna runs operations.","evidence":[{"entity_type":"contact","entity_id":"`+ref+`"}]}]}`, briefContactID, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != want || (want == 1 && kept[0].Evidence[0].EntityID != briefContactID) {
			t.Errorf("citing %q kept %+v, want %d sentence(s) citing the contact's id", ref, kept, want)
		}
	}
}
