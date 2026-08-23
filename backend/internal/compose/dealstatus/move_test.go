// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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

func TestAnOpenTaskNoLongerEndsTheReasoning(t *testing.T) {
	// The card this replaces read the task's title back to the reader and
	// said it had nothing to add. An open task is evidence now, not a reason
	// to stay silent, so the deal still gets a move.
	f := facts{
		deal: openDeal(), now: testNow,
		timeline:  []crmcontracts.Activity{act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -12))},
		openTasks: []activities.OpenTask{{ID: ids.NewV7(), Subject: "Agree the next step"}},
	}
	mv := decideMove(f)
	if mv.Action == ActionNone {
		t.Fatalf("a deal with an open task got no move: %+v", mv)
	}
	if mv.Action != ActionCreateTask {
		t.Fatalf("action = %q, want the next-step task", mv.Action)
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
