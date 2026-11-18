// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// repliedRequest is a current request with a reminder that the reader's reply,
// sent at repliedAt, answered. reminder is nil when the reader may not change it.
func repliedRequest(repliedAt time.Time, reminder *openapi_types.UUID, stillAsked *string) crmcontracts.Activity {
	request := inboundMail(repliedAt.Add(-4 * time.Hour))
	reply := &crmcontracts.RequestReply{
		RequestActivityId: request.Id, RepliedAt: repliedAt, ByReader: true,
		RepliedPrecision: crmcontracts.OccurredPrecisionOccurredPrecisionInstant, StillAsked: stillAsked,
	}
	if reminder != nil {
		version := int64(3)
		reply.ReminderId, reply.ReminderVersion = reminder, &version
	}
	request.EmailSummary = &crmcontracts.EmailSummary{RequestHasReminder: new(true), OurReply: reply}
	return request
}

func TestARepliedRequestOffersItsReminderInsteadOfSayingTheBuyerWaits(t *testing.T) {
	reminder := openapi_types.UUID(ids.NewV7())
	request := repliedRequest(testNow.AddDate(0, 0, -3), &reminder, nil)
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, currentRequests: []crmcontracts.Activity{request}}

	mv := decideMove(f)
	if mv.Action != ActionOpenTask || (*mv.Arguments)[argActivityID] != reminder {
		t.Fatalf("a replied request offered %s on %v, want the reminder opened", mv.Action, mv.Arguments)
	}
	if !strings.HasPrefix(mv.Reason, "You replied 3 days ago.") {
		t.Fatalf("the move does not say we replied: %q", mv.Reason)
	}
	card := composeDeterministic(f, mv)
	if card.ReplyTo != nil || card.RequestBasis != nil {
		t.Fatal("the card still names the request as owed a reply")
	}
	if card.OurReply == nil || card.OurReply.RequestActivityId != request.Id {
		t.Fatal("the card does not carry our reply")
	}
	if in := project(f, mv); in.ReplyTo != "" || in.RequestStates[request.Id.String()] == basisCurrentRequest {
		t.Fatal("the model is still told a reply is owed")
	}
}

func TestARepliedRequestQuotesWhatIsStillAsked(t *testing.T) {
	reminder := openapi_types.UUID(ids.NewV7())
	asked := "send the signed order form"
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC,
		currentRequests: []crmcontracts.Activity{repliedRequest(testNow.Add(-time.Hour), &reminder, &asked)}}
	if mv := decideMove(f); mv.Reason != "You replied today. Still asked: "+asked {
		t.Fatalf("the move does not quote the remaining ask: %q", mv.Reason)
	}
}

// A reader who may not change the reminder keeps the reminder rung. It says
// that we replied and names only the request, never a reminder to mark done.
func TestAReplyWhoseReminderIsHiddenNamesOnlyTheRequest(t *testing.T) {
	request := repliedRequest(testNow.AddDate(0, 0, -1), nil, nil)
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, currentRequests: []crmcontracts.Activity{request}}
	mv := decideMove(f)
	if mv.Action != ActionDraftEmail || (*mv.Arguments)[argActivityID] != request.Id {
		t.Fatalf("a hidden reminder offered %s on %v", mv.Action, mv.Arguments)
	}
	if mv.Reason != "You replied yesterday. The request is still open." {
		t.Fatalf("the move asks for a reminder the reader cannot change: %q", mv.Reason)
	}
	if card := composeDeterministic(f, mv); card.ReplyTo != nil || card.OurReply == nil || card.OurReply.ReminderId != nil {
		t.Fatal("the card must say we replied without naming the hidden reminder")
	}
}

func TestAColleaguesReplyIsTheTeams(t *testing.T) {
	reminder := openapi_types.UUID(ids.NewV7())
	request := repliedRequest(testNow.AddDate(0, 0, -2), &reminder, nil)
	request.EmailSummary.OurReply.ByReader = false
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, currentRequests: []crmcontracts.Activity{request}}
	if mv := decideMove(f); !strings.HasPrefix(mv.Reason, "Your team replied 2 days ago.") {
		t.Fatalf("a colleague's reply reads as the reader's: %q", mv.Reason)
	}
}

func TestARequestWithoutOurReplyStillSaysYourMove(t *testing.T) {
	request := inboundMail(testNow.Add(-time.Hour))
	request.EmailSummary = &crmcontracts.EmailSummary{RequestHasReminder: new(true)}
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, currentRequests: []crmcontracts.Activity{request}}
	mv := decideMove(f)
	if mv.Reason != statusCopyFor("en").Say(statusWords.Reminder) {
		t.Fatalf("an unanswered request changed its move: %q", mv.Reason)
	}
	card := composeDeterministic(f, mv)
	if card.ReplyTo == nil || *card.ReplyTo != request.Id || card.OurReply != nil {
		t.Fatal("an unanswered request must stay the reply owed")
	}
}
