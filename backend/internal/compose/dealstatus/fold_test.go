// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The fold is what puts the filter's output on the wire, and it is where a
// section becomes absent rather than empty. Testing only the filter proves the
// words survive scrutiny and says nothing about whether the reader sees them.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// factsCitingOneRow is a deal whose timeline holds the row every test below
// cites, so a grounded line actually wires. A fold test against empty facts
// proves nothing: every section comes back empty whatever the filter kept.
func factsCitingOneRow(t *testing.T) (facts, string) {
	t.Helper()
	row := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -3))
	return facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{row},
	}, row.Id.String()
}

func TestAnEmptySectionIsAbsentFromTheCardRatherThanEmptyOnIt(t *testing.T) {
	// A heading over no sentence is furniture, and on this card it is also a
	// claim: an empty "what is holding this up" reads as "nothing is".
	f, id := factsCitingOneRow(t)
	written := WrittenStatus{Story: []WrittenLine{{Text: "It went out.", Evidence: []string{id}}}}
	card := foldWritten(composeDeterministic(f, moveNone()), written, f, moveNone())
	if card.Blocker != nil {
		t.Fatalf("an empty blocker reached the card as %+v", card.Blocker)
	}
	if card.Buyer != nil {
		t.Fatalf("an empty buyer section reached the card as %+v", card.Buyer)
	}
	if len(card.Story.Sentences) != 1 {
		t.Fatalf("story = %+v, want the one grounded line", card.Story.Sentences)
	}
}

func TestAPopulatedSectionReachesTheCard(t *testing.T) {
	f, id := factsCitingOneRow(t)
	written := WrittenStatus{
		Story:   []WrittenLine{{Text: "It went out.", Evidence: []string{id}}},
		Blocker: []WrittenLine{{Text: "Nobody answered.", Evidence: []string{id}}},
		Buyer:   []WrittenLine{{Text: "They want 60-day terms.", Evidence: []string{id}}},
	}
	card := foldWritten(composeDeterministic(f, moveNone()), written, f, moveNone())
	if card.Blocker == nil || len(card.Blocker.Sentences) != 1 {
		t.Fatalf("blocker = %+v, want the one grounded line", card.Blocker)
	}
	if card.Buyer == nil || len(card.Buyer.Sentences) != 1 {
		t.Fatalf("buyer = %+v, want the one grounded line", card.Buyer)
	}
}

func TestAVerdictWithReasoningReachesTheCard(t *testing.T) {
	f, id := factsCitingOneRow(t)
	written := WrittenStatus{
		Story: []WrittenLine{{Text: "It went out.", Evidence: []string{id}}},
		Verdict: WrittenVerdict{
			Standing: "cold",
			Because:  []WrittenLine{{Text: "Nothing since July.", Evidence: []string{id}}},
		},
	}
	card := foldWritten(composeDeterministic(f, moveNone()), written, f, moveNone())
	if card.Verdict == nil {
		t.Fatal("a grounded verdict never reached the card")
	}
	if card.Verdict.Standing != "cold" {
		t.Fatalf("standing = %q, want cold", card.Verdict.Standing)
	}
	if len(card.Verdict.Because.Sentences) != 1 {
		t.Fatalf("because = %+v, want the one grounded line", card.Verdict.Because.Sentences)
	}
}

func TestAVerdictWithNoRecognisedStandingIsNotShownEvenWithReasoning(t *testing.T) {
	// This is the arm the filter leaves open on purpose: an unrecognised word
	// is dropped and its reasoning kept, so the fold is the only thing
	// stopping a "Where this stands" heading with no call under it.
	f, id := factsCitingOneRow(t)
	written := WrittenStatus{
		Story: []WrittenLine{{Text: "It went out.", Evidence: []string{id}}},
		Verdict: WrittenVerdict{
			Standing: "",
			Because:  []WrittenLine{{Text: "Nothing since July.", Evidence: []string{id}}},
		},
	}
	card := foldWritten(composeDeterministic(f, moveNone()), written, f, moveNone())
	if card.Verdict != nil {
		t.Fatalf("a verdict with no call reached the card as %+v", card.Verdict)
	}
}

func TestAVerdictRestingOnNothingIsNotShown(t *testing.T) {
	f, id := factsCitingOneRow(t)
	written := WrittenStatus{
		Story:   []WrittenLine{{Text: "It went out.", Evidence: []string{id}}},
		Verdict: WrittenVerdict{Standing: "cold"},
	}
	card := foldWritten(composeDeterministic(f, moveNone()), written, f, moveNone())
	if card.Verdict != nil {
		t.Fatalf("a call with no reasoning reached the card as %+v", card.Verdict)
	}
}

func TestTheModelWritesTheReasonAndNeverTheVerb(t *testing.T) {
	// A reader clicking the button must reach what the RULES chose. The model
	// supplies the sentence around it and nothing else.
	f, id := factsCitingOneRow(t)
	rules := crmcontracts.DealStatusCardMove{
		Action:    ActionDraftEmail,
		Reason:    "the rules' reason",
		Arguments: &map[string]any{"activity_id": id},
		Evidence: []crmcontracts.DealNextBestActionEvidence{
			{Text: "Unanswered: something"},
		},
	}
	written := WrittenStatus{
		Story:      []WrittenLine{{Text: "It went out.", Evidence: []string{id}}},
		MoveReason: WrittenLine{Text: "the model's reason", Evidence: []string{id}},
	}
	card := foldWritten(composeDeterministic(f, rules), written, f, rules)
	if card.Next == nil {
		t.Fatal("the move never reached the card")
	}
	if card.Next.Action != ActionDraftEmail {
		t.Fatalf("action = %q, want the rules' verb", card.Next.Action)
	}
	if (*card.Next.Arguments)["activity_id"] != id {
		t.Fatalf("arguments = %v, want the rules' operand", *card.Next.Arguments)
	}
	// The evidence is now the REASON's, because the reason on screen is the
	// model's: a sentence shown beside another sentence's sources is the one
	// thing a reader following a citation must not meet.
	if len(card.Next.Evidence) != 1 || card.Next.Evidence[0].ActivityId == nil ||
		card.Next.Evidence[0].ActivityId.String() != id {
		t.Fatalf("evidence = %+v, want the record the model's reason cites", card.Next.Evidence)
	}
	if card.Next.Reason != "the model's reason" {
		t.Fatalf("reason = %q, want the model's words", card.Next.Reason)
	}
}

func TestAFoldedCardSaysTheModelWroteIt(t *testing.T) {
	f, id := factsCitingOneRow(t)
	floor := composeDeterministic(f, moveNone())
	if floor.GeneratedBy != crmcontracts.Deterministic {
		t.Fatalf("the floor says %q wrote it", floor.GeneratedBy)
	}
	written := WrittenStatus{Story: []WrittenLine{{Text: "It went out.", Evidence: []string{id}}}}
	if got := foldWritten(floor, written, f, moveNone()).GeneratedBy; got != crmcontracts.Model {
		t.Fatalf("a model-written card says %q wrote it", got)
	}
}

func moveNone() crmcontracts.DealStatusCardMove {
	return crmcontracts.DealStatusCardMove{Action: ActionNone, Reason: "nothing to do"}
}
