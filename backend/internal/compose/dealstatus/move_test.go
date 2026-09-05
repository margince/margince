// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var testNow = time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

func openDeal() crmcontracts.Deal {
	return crmcontracts.Deal{
		Id: openapi_types.UUID(ids.NewV7()), Name: "Nordwind", Status: crmcontracts.DealStatusOpen,
	}
}

func act(kind crmcontracts.ActivityKind, at time.Time) crmcontracts.Activity {
	subject := "Something happened"
	return crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: kind, OccurredAt: at, Subject: &subject,
	}
}

func inboundMail(at time.Time) crmcontracts.Activity {
	a := act(crmcontracts.ActivityKindEmail, at)
	dir := crmcontracts.ActivityDirectionInbound
	a.Direction = &dir
	return a
}

func TestAnOpenTaskIsReusedInsteadOfDuplicated(t *testing.T) {
	taskID := ids.NewV7()
	f := facts{
		deal: openDeal(), now: testNow,
		timeline:  []crmcontracts.Activity{act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -12))},
		openTasks: []activities.OpenTask{{ID: taskID, Subject: "Agree the next step"}},
	}
	mv := decideMove(f)
	if mv.Action != ActionOpenTask || mv.Arguments == nil {
		t.Fatalf("existing work offered another write: %+v", mv)
	}
	if len(mv.Evidence) != 1 || mv.Evidence[0].ActivityId == nil || *mv.Evidence[0].ActivityId != openapi_types.UUID(taskID) {
		t.Fatalf("move does not name the existing task: %+v", mv)
	}
}

func TestABookedMeetingOutranksEverythingElse(t *testing.T) {
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{
			act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 3)),
			inboundMail(testNow.AddDate(0, 0, -3)),
		},
	}
	if mv := decideMove(f); mv.Action != ActionOpenMeetingBrief {
		t.Fatalf("action = %q, want the meeting brief: a dated meeting beats an unanswered mail", mv.Action)
	}
}

func TestAnUnansweredInboundMailBecomesADraft(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, timeline: []crmcontracts.Activity{inboundMail(testNow.AddDate(0, 0, -3))}}
	mv := decideMove(f)
	if mv.Action != ActionDraftEmail {
		t.Fatalf("action = %q, want a drafted reply", mv.Action)
	}
	if mv.Arguments == nil || (*mv.Arguments)["activity_id"] == nil {
		t.Fatalf("the draft names no mail to answer: %+v", mv.Arguments)
	}
}

func TestAnAnsweredInboundMailIsNotStillWaiting(t *testing.T) {
	outbound := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -2))
	dir := crmcontracts.ActivityDirectionOutbound
	outbound.Direction = &dir
	// The timeline is newest first, so the reply sits above the mail it answers.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{outbound, inboundMail(testNow.AddDate(0, 0, -5))},
	}
	if mv := decideMove(f); mv.Action == ActionDraftEmail {
		t.Fatal("a mail that was already answered was offered as unanswered")
	}
}

// A reply this reader may not READ is still a reply. Skipping it would walk
// past it to the inbound behind it and offer to answer a mail somebody has
// already answered — the card would send a rep to write a duplicate.
func TestAWithheldReplyStillCountsAsAnAnswer(t *testing.T) {
	outbound := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -2))
	dir := crmcontracts.ActivityDirectionOutbound
	outbound.Direction = &dir
	state := crmcontracts.ActivityContentStateWithheld
	outbound.ContentState = &state
	outbound.Subject = nil
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{outbound, inboundMail(testNow.AddDate(0, 0, -5))},
	}
	if _, ok := unansweredInbound(f); ok {
		t.Fatal("a withheld reply was ignored, so an answered mail read as unanswered")
	}
}

// The card's move and the deal page's email box both answer "is an answer
// owed?", and they must never answer it differently: a rep told "draft the
// reply" by one and offered "send an email" by the other cannot tell which is
// right. This fails if either stops reading unansweredInbound.
func TestTheMoveAndReplyToNameTheSameMail(t *testing.T) {
	cases := map[string][]crmcontracts.Activity{
		"an unanswered mail": {inboundMail(testNow.AddDate(0, 0, -3))},
		"nothing logged":     {},
		// The mail is NOT the newest row. Without this case both readings
		// coincide on every input and the comparison can never fail — which is
		// exactly how the first version of this test passed against a reply_to
		// that named the wrong record.
		"a newer note sits above the mail": {
			act(crmcontracts.ActivityKindNote, testNow.AddDate(0, 0, -1)),
			inboundMail(testNow.AddDate(0, 0, -4)),
		},
		"only an outbound": func() []crmcontracts.Activity {
			out := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -1))
			dir := crmcontracts.ActivityDirectionOutbound
			out.Direction = &dir
			return []crmcontracts.Activity{out}
		}(),
	}
	for name, timeline := range cases {
		t.Run(name, func(t *testing.T) {
			f := facts{deal: openDeal(), now: testNow, timeline: timeline}
			card := composeDeterministic(f, decideMove(f))
			mv := decideMove(f)

			// What the button would open, and what the box would open.
			var fromMove string
			if mv.Action == ActionDraftEmail && mv.Arguments != nil {
				if id, ok := (*mv.Arguments)["activity_id"].(openapi_types.UUID); ok {
					fromMove = id.String()
				}
			}
			var fromBox string
			if card.ReplyTo != nil {
				fromBox = card.ReplyTo.String()
			}
			// A draft_email move ALWAYS names a mail. Asserting that here is
			// what stops the comparison below going vacuous: read the operand
			// out with the wrong type and fromMove stays empty, the comparison
			// is skipped, and the test passes having compared nothing.
			if mv.Action == ActionDraftEmail && fromMove == "" {
				t.Fatalf("the move offers a draft but names no mail: %#v", *mv.Arguments)
			}
			if fromMove != "" && fromMove != fromBox {
				t.Fatalf("the move answers %q and the email box answers %q", fromMove, fromBox)
			}
		})
	}
}

func TestLastContactIgnoresWhatIsOnlyScheduled(t *testing.T) {
	// The timeline is newest first and holds booked meetings at the top, which
	// are plans rather than contact. Counting one as contact prints "the last
	// contact was 3 days ago" about a meeting that has not happened.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline: []crmcontracts.Activity{
			act(crmcontracts.ActivityKindMeeting, testNow.AddDate(0, 0, 20)),
			act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -6)),
		},
	}
	last, ok := lastContact(f)
	if !ok {
		t.Fatal("a deal with a past mail has had contact")
	}
	if last.Kind != crmcontracts.ActivityKindEmail {
		t.Fatalf("last contact = %v, want the mail: a meeting 20 days out has not happened", last.Kind)
	}
}

func TestACreateTaskArgumentIsAReadyTaskBody(t *testing.T) {
	// The click sends these arguments as the task's body, unedited. A missing
	// link or source is a task the server refuses after the reader clicked.
	mv := decideMove(facts{deal: openDeal(), now: testNow})
	if mv.Action != ActionCreateTask || mv.Arguments == nil {
		t.Fatalf("move = %+v, want a create_task carrying a body", mv)
	}
	args := *mv.Arguments
	if args["subject"] == "" || args["subject"] == nil {
		t.Fatalf("the task carries no subject: %v", args)
	}
	if args["source"] != "ui" {
		t.Fatalf("source = %v, want ui", args["source"])
	}
	links, ok := args["links"].([]map[string]any)
	if !ok || len(links) != 1 {
		t.Fatalf("links = %v, want the deal it belongs to", args["links"])
	}
	if links[0]["entity_type"] != "deal" {
		t.Fatalf("link = %v, want the deal", links[0])
	}
}

func TestAClosedDealGetsNoMove(t *testing.T) {
	deal := openDeal()
	deal.Status = crmcontracts.DealStatusWon
	if mv := decideMove(facts{deal: deal, now: testNow}); mv.Action != ActionNone {
		t.Fatalf("action = %q, want none: a won deal has no next step", mv.Action)
	}
}

func TestAWithheldRowIsNeverNamedAsTheOperand(t *testing.T) {
	// The verb this arm names gates on content, so pointing it at a row the
	// reader may not open would render a button that 404s.
	withheldMail := inboundMail(testNow.AddDate(0, 0, -3))
	state := crmcontracts.ActivityContentStateWithheld
	withheldMail.ContentState = &state
	withheldMail.Subject = nil
	f := facts{deal: openDeal(), now: testNow, timeline: []crmcontracts.Activity{withheldMail}}
	if mv := decideMove(f); mv.Action == ActionDraftEmail {
		t.Fatal("a withheld mail was offered as the one to answer")
	}
}

// A deal nobody has contacted yet used to be told "Nothing has been logged on
// this deal yet — agree the next step", which restates the empty timeline the
// reader is already looking at. These tests pin the sentence that replaced it:
// who to write to, and the count a reader can check it against.

func TestTheFirstMoveOnAnUncontactedDealNamesWhoToOpenWith(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "blocker", Name: "Patrick Ganzmann"},
		{Role: "champion", Name: "Roland Martinez"},
		{Role: "economic_buyer", Name: "Philipp Königs"},
	}}

	mv := decideMove(f)

	if mv.Action != ActionDraftEmail {
		t.Errorf("the move on an uncontacted deal is %q, not a first email", mv.Action)
	}
	if !strings.Contains(mv.Reason, "Roland Martinez") {
		t.Errorf("the advice does not name who to open with: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "champion") {
		t.Errorf("the advice does not say why that person: %q", mv.Reason)
	}
	// The count is what makes the sentence checkable against the page.
	if !strings.Contains(mv.Reason, "3 people") {
		t.Errorf("the advice does not say how many are named: %q", mv.Reason)
	}
}

func TestTheFirstMoveNeverOpensWithTheBlocker(t *testing.T) {
	// A deal whose ONLY named seat is the person most likely to refuse it. No
	// advice is the right answer; naming them would be worse than silence.
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "blocker", Name: "Patrick Ganzmann"},
	}}

	mv := decideMove(f)

	if mv.Action == ActionDraftEmail {
		t.Error("the card told the rep to open the deal by writing to its blocker")
	}
	if strings.Contains(mv.Reason, "Patrick Ganzmann") {
		t.Errorf("the blocker was named as the way in: %q", mv.Reason)
	}
}

func TestTheFirstMoveTakesTheBestAvailableRole(t *testing.T) {
	// No champion: the economic buyer is the next best answer, not a fallback
	// to "agree the next step".
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "influencer", Name: "Ines Eschbacher"},
		{Role: "economic_buyer", Name: "Philipp Königs"},
	}}

	mv := decideMove(f)

	if !strings.Contains(mv.Reason, "Philipp Königs") {
		t.Errorf("the economic buyer was not chosen over the influencer: %q", mv.Reason)
	}
	if !strings.Contains(mv.Reason, "economic buyer") {
		t.Errorf("the role is not said in words a sentence can carry: %q", mv.Reason)
	}
}

func TestASeatTheReaderMayNotNameStillCarriesItsRole(t *testing.T) {
	// The reader holds deal:read without person:read, so the seam supplies the
	// seats unnamed. The role is not the secret and the advice still works.
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{{Role: "champion"}}}

	mv := decideMove(f)

	if mv.Action != ActionDraftEmail {
		t.Errorf("an unnamed champion produced no opening move: %q", mv.Action)
	}
	if !strings.Contains(mv.Reason, "champion") {
		t.Errorf("the role was dropped along with the name: %q", mv.Reason)
	}
}

func TestAContactedDealKeepsItsOwnMove(t *testing.T) {
	// The opening move is for a deal with NO contact. Once somebody has
	// written, the later rules own the answer and this must not displace them.
	f := facts{
		deal:     openDeal(),
		now:      testNow,
		timeline: []crmcontracts.Activity{inboundMail(testNow.Add(-24 * time.Hour))},
		seats:    []Seat{{Role: "champion", Name: "Roland Martinez"}},
	}

	mv := decideMove(f)

	if strings.Contains(mv.Reason, "Nobody has been contacted") {
		t.Errorf("a deal with an inbound mail was called uncontacted: %q", mv.Reason)
	}
}

func TestADealWithNoSeatsSaysNothingItCannotKnow(t *testing.T) {
	// Nobody is named, so there is nobody to open with. The card falls back to
	// its old sentence rather than inventing a person.
	f := facts{deal: openDeal(), now: testNow}

	mv := decideMove(f)

	if mv.Action == ActionDraftEmail {
		t.Error("the card advised writing an email on a deal with nobody to write to")
	}
}

// `arguments` is typed as an object in the contract, so a move that takes no
// operand ships an empty one.
//
// Asserted through the WIRE, not the Go value: a nil map behind a non-nil
// pointer is invisible in Go — `mv.Arguments != nil` passes — and only
// becomes `"arguments": null` when it is marshalled. A client that reads the
// field as an object gets null where it indexes.
func TestAMoveWithNoOperandShipsAnEmptyObjectNotNull(t *testing.T) {
	f := facts{deal: openDeal(), now: testNow, seats: []Seat{
		{Role: "champion", Name: "Roland Martinez"},
	}}

	encoded, err := json.Marshal(decideMove(f))
	if err != nil {
		t.Fatalf("marshalling the move: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"arguments":null`)) {
		t.Errorf("the move serialized arguments as null, which the contract types as an object: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"arguments":{}`)) {
		t.Errorf("a move with no operand did not ship an empty object: %s", encoded)
	}
}
