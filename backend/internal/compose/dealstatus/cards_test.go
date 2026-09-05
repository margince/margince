// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// Lifting a written standing out of a stored card.
//
// The queue reads these for a whole page at once, so every case here is about
// what ONE unusable entry must not do to the rest of the page — the rule
// moves_test.go states next door, over the other half of the same card.

import (
	"encoding/json"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func storedVerdict(t *testing.T, verdict *crmcontracts.DealStatusCardVerdict) []byte {
	t.Helper()
	payload, err := json.Marshal(stored{Card: crmcontracts.DealStatusCard{Verdict: verdict}})
	if err != nil {
		t.Fatalf("encoding a stored card: %v", err)
	}
	return payload
}

func citedLine(text string, cites ...crmcontracts.OrganizationBriefEvidence) crmcontracts.OrganizationBriefSentence {
	return crmcontracts.OrganizationBriefSentence{Text: text, Evidence: cites}
}

func citesActivity(id ids.UUID) crmcontracts.OrganizationBriefEvidence {
	return crmcontracts.OrganizationBriefEvidence{
		EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeActivity,
		EntityId:   openapi_types.UUID(id),
	}
}

func citesDeal(id ids.UUID) crmcontracts.OrganizationBriefEvidence {
	return crmcontracts.OrganizationBriefEvidence{
		EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeDeal,
		EntityId:   openapi_types.UUID(id),
	}
}

// The ordinary case: a written card hands its standing and the sentence behind
// it through unchanged, with the message that sentence was written from named
// so the caller can ask the audience about it.
func TestAWrittenCardHandsItsStandingThrough(t *testing.T) {
	cited := ids.NewV7()
	payload := storedVerdict(t, &crmcontracts.DealStatusCardVerdict{
		Standing: "blocked",
		Because: crmcontracts.DealStatusCardSection{
			Sentences: []crmcontracts.OrganizationBriefSentence{
				citedLine("Legal has not returned the DPA.", citesActivity(cited)),
			},
		},
	})

	card, ok := cardFromPayload(payload)

	if !ok {
		t.Fatal("a written card yielded no standing")
	}
	if card.Standing != "blocked" {
		t.Errorf("standing = %q, wanted the card's own word", card.Standing)
	}
	if card.DecisiveLine != "Legal has not returned the DPA." {
		t.Errorf("line = %q", card.DecisiveLine)
	}
	if len(card.CitedActivities) != 1 || card.CitedActivities[0] != cited {
		t.Errorf("the cited message did not survive the lift: %v — the caller has nothing to re-gate", card.CitedActivities)
	}
}

// An unreadable payload is a MISS and not a failure: failing a whole page of
// the queue over one stale blob would take every other row's reading with it.
func TestAnUnreadablePayloadYieldsNoStandingRatherThanFailing(t *testing.T) {
	if _, ok := cardFromPayload([]byte("{not json")); ok {
		t.Fatal("a corrupt payload produced a standing")
	}
}

// A card written before this build's verdict existed carries none. Absent, not
// an empty judgement.
func TestACardWithNoVerdictYieldsNoStanding(t *testing.T) {
	payload := storedVerdict(t, nil)

	if _, ok := cardFromPayload(payload); ok {
		t.Fatal("a card with no verdict produced a standing")
	}
}

// A standing with nothing behind it is dropped whole. "This deal is blocked"
// with no way to ask why is a judgement the reader cannot check, and the row's
// own typed reasons say more.
func TestAStandingWithNoSentenceIsDropped(t *testing.T) {
	payload := storedVerdict(t, &crmcontracts.DealStatusCardVerdict{
		Standing: "cold",
		Because:  crmcontracts.DealStatusCardSection{},
	})

	if _, ok := cardFromPayload(payload); ok {
		t.Fatal("a standing with no sentence behind it reached a row")
	}
}

// The FIRST sentence, not a join of all of them: the card writes its sentences
// in the order it wants them read, and a queue row draws one line.
// Concatenating them would compose a sentence nobody wrote.
func TestTheDecisiveLineIsTheFirstSentenceAndNotAJoin(t *testing.T) {
	payload := storedVerdict(t, &crmcontracts.DealStatusCardVerdict{
		Standing: "drifting",
		Because: crmcontracts.DealStatusCardSection{
			Sentences: []crmcontracts.OrganizationBriefSentence{
				citedLine("Nobody has written since June."),
				citedLine("The champion changed roles."),
			},
		},
	})

	card, ok := cardFromPayload(payload)

	if !ok {
		t.Fatal("a written card yielded no standing")
	}
	if card.DecisiveLine != "Nobody has written since June." {
		t.Errorf("line = %q, wanted the first sentence alone", card.DecisiveLine)
	}
}

// An empty leading sentence is skipped rather than served as a blank line.
func TestAnEmptyLeadingSentenceIsSkipped(t *testing.T) {
	payload := storedVerdict(t, &crmcontracts.DealStatusCardVerdict{
		Standing: "live",
		Because: crmcontracts.DealStatusCardSection{
			Sentences: []crmcontracts.OrganizationBriefSentence{
				citedLine(""),
				citedLine("They booked the security review."),
			},
		},
	})

	card, ok := cardFromPayload(payload)

	if !ok {
		t.Fatal("a card whose first sentence was empty yielded no standing at all")
	}
	if card.DecisiveLine != "They booked the security review." {
		t.Errorf("line = %q", card.DecisiveLine)
	}
}

// ONLY activities are named for the audience question. A sentence resting on
// the deal's own fields cites a record the reader already reached — the deal
// grant is what put this row on their queue — and asking the audience question
// about a record that has none would refuse every standing.
func TestOnlyTheCitedMessagesAreNamedForTheAudienceQuestion(t *testing.T) {
	message := ids.NewV7()
	payload := storedVerdict(t, &crmcontracts.DealStatusCardVerdict{
		Standing: "blocked",
		Because: crmcontracts.DealStatusCardSection{
			Sentences: []crmcontracts.OrganizationBriefSentence{
				citedLine("Legal has not returned the DPA.",
					citesActivity(message), citesDeal(ids.NewV7())),
			},
		},
	})

	card, ok := cardFromPayload(payload)

	if !ok {
		t.Fatal("a written card yielded no standing")
	}
	if len(card.CitedActivities) != 1 || card.CitedActivities[0] != message {
		t.Errorf("cited activities = %v, wanted only the message", card.CitedActivities)
	}
}

// The all-not-any rule, over the predicate alone.
//
// This asserts the LOGIC and not the serving: it hands allReadable a map it
// built itself, so it cannot prove that map ever comes from the audience clause.
// What proves the served behaviour is
// integration/dealstandingaudience_integration_test.go, which narrows a real
// message and watches the standing disappear. This one is here because the rule
// it holds — one lost citation is enough — is a decision somebody could quietly
// loosen to "any", and a loosened predicate would still pass every test that
// only ever cites one message.
func TestTheAllReadableRuleRefusesOnTheFirstLostMessage(t *testing.T) {
	readable, lost := ids.NewV7(), ids.NewV7()
	admitted := map[ids.UUID]bool{readable: true}

	if allReadable([]ids.UUID{readable, lost}, admitted) {
		t.Error("a standing citing a message the reader lost was judged servable")
	}
	if !allReadable([]ids.UUID{readable}, admitted) {
		t.Error("a standing whose every citation is readable was withheld")
	}
}

// A sentence citing no message needs no audience question. The served version of
// this claim is in the integration lane beside the two above it.
func TestTheAllReadableRuleAdmitsASentenceThatCitesNoMessage(t *testing.T) {
	if !allReadable(nil, map[ids.UUID]bool{}) {
		t.Error("a standing that names no message was withheld for an audience it does not have")
	}
}

// One question per message, however many deals cite it — the dedupe that keeps a
// page of thirty rows to one audience query.
func TestOneMessageIsAskedAboutOnceHoweverManyStandingsCiteIt(t *testing.T) {
	shared := ids.NewV7()
	cards := map[ids.UUID]CachedCard{
		ids.NewV7(): {CitedActivities: []ids.UUID{shared}},
		ids.NewV7(): {CitedActivities: []ids.UUID{shared}},
	}

	wanted := citedActivities(cards)

	if len(wanted) != 1 || wanted[0] != shared {
		t.Errorf("asked about %v, wanted the one message once", wanted)
	}
}
