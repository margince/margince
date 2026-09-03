// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// Lifting a decided move out of a stored card.
//
// The queue reads these for a whole page at once, so every case here is about
// what ONE unusable entry must not do to the rest of the page.

import (
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func storedCard(t *testing.T, next *crmcontracts.DealStatusCardMove) []byte {
	t.Helper()
	payload, err := json.Marshal(stored{Card: crmcontracts.DealStatusCard{Next: next}})
	if err != nil {
		t.Fatalf("encoding a stored card: %v", err)
	}
	return payload
}

// The ordinary case: a written card hands its verb and its operand through
// unchanged, because the queue draws exactly what the deal page draws.
func TestAWrittenCardHandsItsMoveThrough(t *testing.T) {
	args := map[string]any{"activity_id": "01a05500-0000-7000-8000-0000000000a1"}
	payload := storedCard(t, &crmcontracts.DealStatusCardMove{
		Action:    ActionDraftEmail,
		Reason:    "They wrote 6 days ago and nobody has answered — draft the reply.",
		Arguments: &args,
	})

	move, ok := moveFromPayload(payload)

	if !ok {
		t.Fatal("a written card yielded no move")
	}
	if move.Action != ActionDraftEmail {
		t.Fatalf("the move is %q, wanted the card's own verb", move.Action)
	}
	if move.Arguments == nil || (*move.Arguments)["activity_id"] == "" {
		t.Fatalf("the verb's operand did not survive the lift: %+v", move.Arguments)
	}
}

// `none` is a real answer on the deal page — the card says in words that a
// closed deal has no next step. A queue row has no sentence to put it in, so it
// is dropped HERE rather than left for every caller to recognize.
func TestACardWithNothingToDoYieldsNoMove(t *testing.T) {
	payload := storedCard(t, &crmcontracts.DealStatusCardMove{
		Action: ActionNone,
		Reason: "This deal is won — there is no next step to take.",
	})

	if _, ok := moveFromPayload(payload); ok {
		t.Fatal("a card whose own answer was 'nothing to do' put a verb on a row")
	}
}

// A card written before this build's contract is a MISS, not a failure. The
// card is derived content the deal page rewrites on its next read, and failing
// the page over one stale blob would take every other row's move with it.
func TestAnUnreadablePayloadIsAMissAndNotAFailure(t *testing.T) {
	if _, ok := moveFromPayload([]byte("{not json")); ok {
		t.Fatal("an unreadable card yielded a move")
	}
}

// A card that names no next step at all — an older write, or one whose lane
// never filled the field. Absent and "nothing to do" mean the same thing to a
// row, and neither is a verb.
func TestACardWithNoNextStepYieldsNoMove(t *testing.T) {
	if _, ok := moveFromPayload(storedCard(t, nil)); ok {
		t.Fatal("a card carrying no next step yielded a move")
	}
}
