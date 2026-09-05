// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/orgbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The grounding is the whole promise: a finding the reader sees rests on a
// message they were given and quotes words that message contains. Everything
// else the model says is dropped, and what survives carries the receipt the
// page draws.

var scanAt = time.Date(2026, 8, 19, 8, 45, 0, 0, time.UTC)

func scanInput() (Input, MessageIn) {
	nudge := MessageIn{
		ID: ids.NewV7(), Kind: "email", Direction: "inbound", At: scanAt,
		Subject: "Re: Telematics — next steps",
		Text:    "Morning — did the sample reports get held up somewhere? The team is meeting on Thursday.",
	}
	return Input{
		Account:  orgbrief.Input{Name: "Nordlicht Logistik AG"},
		Messages: []MessageIn{nudge},
	}, nudge
}

func reply(findings ...map[string]any) string {
	encoded, err := json.Marshal(map[string]any{"findings": findings})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func finding(message MessageIn, quote string) map[string]any {
	return map[string]any{
		"kind": "question_unanswered", "title": "Answer where the reports are",
		"reason": "Jonas asked where the sample reports are and nobody has written back.",
		"message_id": message.ID.String(), "quote": quote, "action": "draft_reply",
	}
}

func TestAGroundedFindingCarriesTheMessageAsItsReceipt(t *testing.T) {
	in, nudge := scanInput()
	orgID := ids.NewV7().String()

	kept, refused, err := ParseFindings(reply(finding(nudge, "did the sample reports get held up somewhere?")), orgID, in)
	if err != nil || len(refused) != 0 || len(kept) != 1 {
		t.Fatalf("kept %d, refused %v, err %v — want the one grounded finding", len(kept), refused, err)
	}
	got := kept[0]
	if got.Kind != crmcontracts.Organization360SuggestionKindQuestionUnanswered {
		t.Errorf("kind = %q", got.Kind)
	}
	if got.WrittenBy == nil || *got.WrittenBy != crmcontracts.Model {
		t.Errorf("written_by = %v, want the model named as the writer", got.WrittenBy)
	}
	cited := got.Evidence[0]
	if cited.EntityType != crmcontracts.OrganizationBriefEvidenceEntityTypeActivity || cited.EntityId.String() != nudge.ID.String() {
		t.Errorf("evidence = %+v, want the cited message", cited)
	}
	if cited.Quote == nil || *cited.Quote != "did the sample reports get held up somewhere?" {
		t.Errorf("quote = %v, want the verbatim words", cited.Quote)
	}
	if cited.Name == nil || *cited.Name != nudge.Subject || cited.At == nil || !cited.At.Equal(scanAt) {
		t.Errorf("name = %v, at = %v; want the subject and the date the message carries", cited.Name, cited.At)
	}
	if cited.Origin == nil || *cited.Origin != "Email they sent" {
		t.Errorf("origin = %v", cited.Origin)
	}
	if got.Action == nil || got.Action.Kind != crmcontracts.Organization360SuggestionActionKindDraftReply ||
		got.Action.ActivityId == nil || got.Action.ActivityId.String() != nudge.ID.String() {
		t.Errorf("action = %+v, want a draft anchored on the message", got.Action)
	}
	if got.Fingerprint == "" || got.DueAt == nil || !got.DueAt.Equal(scanAt) {
		t.Errorf("fingerprint %q, due %v — a finding is identified and dated by its evidence", got.Fingerprint, got.DueAt)
	}
}

// A quote that is not in the message is the fabrication this filter exists
// to refuse. The finding goes whole; it is not shown with the quote removed.
func TestAFindingWhoseQuoteIsNotInItsMessageIsRefused(t *testing.T) {
	in, nudge := scanInput()

	kept, refused, err := ParseFindings(reply(finding(nudge, "we need this by Monday or the deal is off")), "org", in)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 0 || len(refused) != 1 || !strings.Contains(refused[0], "verbatim") {
		t.Errorf("kept %d, refused %v — want the finding refused for its quote", len(kept), refused)
	}
}

// Whitespace is the one thing a quote may fold: a body arrives with its line
// breaks, and a model that reads across one writes a sentence.
func TestAQuoteMayFoldTheMessagesLineBreaks(t *testing.T) {
	in, nudge := scanInput()
	in.Messages[0].Text = "did the sample reports\nget held   up somewhere?"
	nudge.Text = in.Messages[0].Text

	kept, refused, err := ParseFindings(reply(finding(nudge, "did the sample reports get held up somewhere?")), "org", in)
	if err != nil || len(refused) != 0 || len(kept) != 1 {
		t.Fatalf("kept %d, refused %v, err %v", len(kept), refused, err)
	}
}

func TestAFindingCitingAMessageTheModelWasNotGivenIsRefused(t *testing.T) {
	in, _ := scanInput()
	stranger := MessageIn{ID: ids.NewV7(), Text: "anything"}

	kept, refused, _ := ParseFindings(reply(finding(stranger, "anything")), "org", in)
	if len(kept) != 0 || len(refused) != 1 {
		t.Errorf("kept %d, refused %v — a citation outside the input must not ground", len(kept), refused)
	}
}

func TestOnlyTheReadKindsMayBeRaised(t *testing.T) {
	in, nudge := scanInput()
	raw := finding(nudge, "did the sample reports get held up somewhere?")
	raw["kind"] = "no_reply"

	kept, refused, _ := ParseFindings(reply(raw), "org", in)
	if len(kept) != 0 || len(refused) != 1 {
		t.Errorf("a rule kind raised by the model was kept: %v / %v", kept, refused)
	}
}

func TestAnIdInTheProseIsRefused(t *testing.T) {
	in, nudge := scanInput()
	raw := finding(nudge, "did the sample reports get held up somewhere?")
	raw["reason"] = "See " + nudge.ID.String() + " for the ask."

	kept, refused, _ := ParseFindings(reply(raw), "org", in)
	if len(kept) != 0 || len(refused) != 1 {
		t.Errorf("an id in the reason was shown to a reader: %v / %v", kept, refused)
	}
}

func TestAReplyWithNoFindingsKeyDidNotAnswer(t *testing.T) {
	in, _ := scanInput()
	if _, _, err := ParseFindings(`{"answer": []}`, "org", in); err == nil {
		t.Error("a reply without a findings key was read as an empty answer")
	}
	kept, refused, err := ParseFindings(`{"findings": []}`, "org", in)
	if err != nil || len(kept) != 0 || len(refused) != 0 {
		t.Errorf("an empty answer is a good answer: kept %d, refused %v, err %v", len(kept), refused, err)
	}
}

// The same message raised twice under one kind is one finding: the
// fingerprint is over what it rests on, which is what the dismissal keys on.
func TestTheSameSituationRaisedTwiceIsOneFinding(t *testing.T) {
	in, nudge := scanInput()
	quote := "did the sample reports get held up somewhere?"

	kept, _, _ := ParseFindings(reply(finding(nudge, quote), finding(nudge, quote)), "org", in)
	if len(kept) != 1 {
		t.Errorf("kept %d findings for one situation", len(kept))
	}
}

// The request's reply schema is built from THIS input: a fabricated id fails
// the provider's own validation before the parser sees it.
func TestTheReplySchemaOffersOnlyTheMessagesGiven(t *testing.T) {
	in, nudge := scanInput()
	req := ScanRequest(in, "en")
	schema := string(req.ResponseSchema)
	if !strings.Contains(schema, nudge.ID.String()) {
		t.Error("the message id is not in the citation enum")
	}
	if !strings.Contains(schema, "question_unanswered") || strings.Contains(schema, "no_reply") {
		t.Error("the kind enum is not the four read kinds")
	}
	if !strings.Contains(req.System, "account records") || len(req.Messages) != 1 {
		t.Error("the request is not the fenced one-shot the site sends")
	}
}

// The fingerprint moves with what the model was given and with nothing else:
// two readers with different grants get two, and one reader gets the same
// one until the account moves.
func TestTheFingerprintFollowsTheInput(t *testing.T) {
	in, _ := scanInput()
	a, err := Fingerprint(in, "routing-1", "en")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Fingerprint(in, "routing-1", "en")
	if a != b {
		t.Error("the fingerprint is not stable over identical inputs")
	}
	narrowed := in
	narrowed.Messages = nil
	c, _ := Fingerprint(narrowed, "routing-1", "en")
	if c == a {
		t.Error("a reader who may read fewer messages got the other reader's fingerprint")
	}
	d, _ := Fingerprint(in, "routing-2", "en")
	if d == a {
		t.Error("re-pointing the lane did not move the fingerprint")
	}
}

func TestTheOriginNamesTheChannelAndWhoSpoke(t *testing.T) {
	cases := map[MessageIn]string{
		{Kind: "email", Direction: "outbound"}: "Email you sent",
		{Kind: "call", Direction: "inbound"}:   "Call they sent",
		{Kind: "meeting"}:                      "Meeting",
		{Kind: "fax", Direction: "inbound"}:    "Exchange they sent",
	}
	for message, want := range cases {
		if got := origin(message); got != want {
			t.Errorf("origin(%+v) = %q, want %q", message, got, want)
		}
	}
}
