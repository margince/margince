// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// What the two writers guarantee, and what the grounding filter refuses.
//
// The fixture is one relationship a reader could act on: an unanswered
// objection, a promise we owe, an open deal and a message whose words are not
// this reader's. Every case below is one thing a reader would be misled by if
// it stopped holding.

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The records every fixture here is about.
const (
	briefPersonID = "11111111-1111-4111-8111-111111111111"
	briefDealID   = "22222222-2222-4222-8222-222222222222"
	objectionID   = "33333333-3333-4333-8333-333333333333"
	schedulingID  = "44444444-4444-4444-8444-444444444444"
	strangerID    = "55555555-5555-4555-8555-555555555555"
)

func inputFixture() Input {
	return Input{
		Name: "Anna Weber", Title: "Head of Operations", Employer: "Acme Logistik GmbH",
		BuyingRole: "economic_buyer", Strength: 64,
		LastInbound:  "2026-08-29T08:10:00Z",
		LastOutbound: "2026-08-20T16:00:00Z",
		OpenDeal: &DealIn{
			ID: briefDealID, Name: "Acme renewal 2027", Stage: "negotiation",
			AmountMinor: 18_000_000, Currency: "EUR", CloseDate: "2026-11-30",
		},
		Changes: []ChangeIn{{Kind: "replied_after_gap", At: "2026-08-29T08:10:00Z", Days: 34}},
		Claims: []ClaimIn{{
			ID: "c-1", Kind: "objection", Body: "One listed sub-processor blocks legal sign-off.",
			Status: "open", Quote: "we cannot go ahead while the analytics vendor is on it",
			SourceID: objectionID,
		}},
		Recent: []ActIn{
			{
				ID: schedulingID, Kind: "email", Subject: "Re: renewal call",
				Preview: "Thursday at ten works for me.", Direction: "inbound", At: "2026-08-29T08:10:00Z",
			},
			{
				ID: objectionID, Kind: "email", Subject: "Sub-processor list before we sign",
				Preview: "We cannot go ahead while the analytics vendor is on it.",
				Move:    "needs_reply", Direction: "inbound", At: "2026-08-24T09:00:00Z",
			},
		},
	}
}

type scriptedLane struct{ reply string }

func (l *scriptedLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}

type nonsenseLane struct{}

func (nonsenseLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: "I'm afraid I can't do that."}, nil
}

// A brief nobody can check is worse than no brief: the reader sees an assertion
// about a person with nothing to check it against.
func TestParseBriefDropsSentencesCitingRecordsTheInputNeverHeld(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief(`{"sentences":[
		{"text":"They are waiting on the sub-processor list.","evidence":[
			{"entity_type":"activity","entity_id":"`+objectionID+`"}]},
		{"text":"They spoke to your CFO last week.","evidence":[
			{"entity_type":"activity","entity_id":"`+strangerID+`"}]}
	]}`, briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept %d sentence(s), want only the grounded one: %+v", len(kept), kept)
	}
	if !strings.Contains(kept[0].Text, "sub-processor") {
		t.Errorf("kept %q, want the sentence citing a record this input carried", kept[0].Text)
	}
}

// The pair is the reference, not the id: a real deal id cited as a person
// passes an id-only check and then routes the reader to the wrong screen.
func TestParseBriefDropsARealIDCitedAsTheWrongKind(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief(`{"sentences":[
		{"text":"They own the renewal.","evidence":[
			{"entity_type":"person","entity_id":"`+briefDealID+`"}]}
	]}`, briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != 0 {
		t.Errorf("kept %+v, want the deal-cited-as-a-person sentence dropped", kept)
	}
}

// The brief is about ONE contact. Accepting any person citation would let a
// reply hand back an id this reader never saw, rendered as a link they could
// click into a record their scope may hide.
func TestParseBriefRefusesAPersonCitationThatIsNotThisContact(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief(`{"sentences":[
		{"text":"They are the economic buyer.","evidence":[
			{"entity_type":"person","entity_id":"`+strangerID+`"}]}
	]}`, briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != 0 {
		t.Errorf("kept %+v, want a sentence about another person dropped", kept)
	}
}

// An unlabelled claim is read as a fact, which is the strictest reading: it
// must be grounded and it may not judge. A nature nobody defined is read the
// same way rather than passed through to a card that would render it.
func TestAnUnknownNatureIsReadAsAFact(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief(`{"sentences":[
		{"text":"They are waiting on us.","nature":"prophecy","evidence":[
			{"entity_type":"person","entity_id":"`+briefPersonID+`"}]}
	]}`, briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != 1 || kept[0].Nature != natureFact {
		t.Errorf("kept %+v, want the unknown nature read as a fact", kept)
	}
}

// The card is four or five sentences beside a page already full of actions. A
// brief offering three moves has handed back the triage it existed to do.
func TestOnlyTheFirstRecommendationSurvives(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief(`{"sentences":[
		{"text":"Send the sub-processor list today.","nature":"recommendation","evidence":[
			{"entity_type":"activity","entity_id":"`+objectionID+`"}]},
		{"text":"Also call their legal team.","nature":"recommendation","evidence":[
			{"entity_type":"activity","entity_id":"`+objectionID+`"}]}
	]}`, briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != maxRecommendations {
		t.Fatalf("kept %d recommendation(s), want %d: %+v", len(kept), maxRecommendations, kept)
	}
	if !strings.Contains(kept[0].Text, "sub-processor list") {
		t.Errorf("kept %q, want the first move rather than a later one", kept[0].Text)
	}
}

// An ungrounded recommendation must not spend the budget: counting it first
// would let one malformed claim suppress the valid advice behind it, and the
// reader would lose the advice and be told nothing about why.
func TestTheAdviceBudgetIsSpentOnlyByAdviceTheReaderSees(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief(`{"sentences":[
		{"text":"Call the CFO.","nature":"recommendation","evidence":[
			{"entity_type":"activity","entity_id":"`+strangerID+`"}]},
		{"text":"Send the sub-processor list today.","nature":"recommendation","evidence":[
			{"entity_type":"activity","entity_id":"`+objectionID+`"}]}
	]}`, briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != 1 || !strings.Contains(kept[0].Text, "sub-processor list") {
		t.Fatalf("kept %+v, want the grounded move the ungrounded one preceded", kept)
	}
}

// A model that wraps its JSON in a ```json fence is answering correctly.
// Trimming whitespace alone would drop the whole lane to the floor on those
// providers, invisibly — the reader would only see a plainer brief.
func TestParseBriefReadsAFencedReply(t *testing.T) {
	t.Parallel()
	kept, err := ParseBrief("```json\n"+`{"sentences":[
		{"text":"They are waiting on the list.","evidence":[
			{"entity_type":"activity","entity_id":"`+objectionID+`"}]}
	]}`+"\n```", briefPersonID, inputFixture())
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(kept) != 1 {
		t.Errorf("kept %d sentence(s) from a fenced reply, want 1", len(kept))
	}
}

// Write degrades rather than failing, and says which writer the reader has.
func TestWriteFallsBackRatherThanFailing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		lane Completer
	}{
		{"no lane configured", nil},
		{"a reply that is not JSON", nonsenseLane{}},
		// Parseable, and every sentence cites a record this input never held —
		// so nothing survives grounding, and a brief of nothing is not an
		// answer.
		{"a reply that cites nothing of this person", &scriptedLane{reply: `{"sentences":[
			{"text":"They spoke to your CFO.","evidence":[
				{"entity_type":"activity","entity_id":"` + strangerID + `"}]}]}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			written, by, err := Write(t.Context(), tc.lane, briefPersonID, inputFixture(),
				string(textlang.English))
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if by != crmcontracts.Deterministic {
				t.Errorf("generated_by = %q, want the floor named honestly", by)
			}
			if len(written) == 0 {
				t.Error("the floor wrote nothing, so the card would render blank")
			}
		})
	}
}

// The model path reports itself as the model path, and keeps only what grounds.
func TestWriteKeepsWhatItGroundsAndNamesTheModel(t *testing.T) {
	t.Parallel()
	lane := &scriptedLane{reply: `{"sentences":[
		{"text":"They are waiting on the sub-processor list before legal signs.","evidence":[
			{"entity_type":"activity","entity_id":"` + objectionID + `"}]},
		{"text":"This renewal cannot close until that is answered.","nature":"assessment","evidence":[
			{"entity_type":"deal","entity_id":"` + briefDealID + `"}]},
		{"text":"They spoke to your CFO.","evidence":[
			{"entity_type":"activity","entity_id":"` + strangerID + `"}]}
	]}`}
	written, by, err := Write(t.Context(), lane, briefPersonID, inputFixture(), string(textlang.English))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if by != crmcontracts.Model {
		t.Fatalf("generated_by = %q, want the model path", by)
	}
	if len(written) != 2 {
		t.Fatalf("kept %d sentence(s), want the two grounded ones: %+v", len(written), written)
	}
	if written[1].Nature != natureAssessment {
		t.Errorf("nature = %q, want the judgment labelled as one", written[1].Nature)
	}
}
