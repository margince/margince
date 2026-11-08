// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/claims"
)

// cited is a reply scripted with record ids as the model writes it: each id
// the request showed spelled as its ref. An id the request never showed stays
// as written, which is how a model that invents one answers.
func cited(in Input, reply string) string { return shownRefs(in).Refer(reply) }

func parseBrief(reply string, in Input) ([]Sentence, error) {
	return ParseBrief(cited(in, reply), in)
}

// The prompt shows each buyer, document and thread by its ref. A reply
// citing a ref reads back to the record's id; the id itself, and a ref the
// request never minted, ground nothing.
func TestTheRoomBriefShowsRefsAndReadsThemBackToRecords(t *testing.T) {
	in := room()
	shown := BriefRequest(in, "en").Messages[0].Content
	if claims.SpellsRecordID(shown) || !strings.Contains(shown, `"id":"t1"`) {
		t.Fatalf("the request must show refs and never an id:\n%s", shown)
	}
	thread := in.Threads[0].ID
	for ref, want := range map[string]int{"t1": 1, thread: 0, "t9": 0} {
		kept, err := ParseBrief(`{"sentences":[{"text":"A buyer is waiting.","nature":"fact","evidence":[{"entity_type":"thread","entity_id":"`+ref+`"}]}]}`, in)
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != want || (want == 1 && kept[0].Evidence[0].EntityID != thread) {
			t.Errorf("citing %q kept %+v, want %d sentence(s) citing the thread's id", ref, kept, want)
		}
	}
}
