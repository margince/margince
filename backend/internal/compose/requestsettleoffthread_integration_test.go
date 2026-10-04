// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// judge is the settlement pass's own principal.
func (o *owedEnv) judge() context.Context {
	return principal.WithActor(o.e.Admin(), o.classifier())
}

// askOf captures a mail from the customer and judges it a request.
func (o *owedEnv) askOf(t *testing.T, subject string, at time.Time) ids.UUID {
	t.Helper()
	request := o.customerWrites(t, "pat@customer.example", subject, at)
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
		"prompts-test", time.Now()); err != nil || !applied {
		t.Fatalf("judging the request: applied=%v err=%v", applied, err)
	}
	return request
}

// offered is the settlement candidate for the request, if the pass offers it.
func (o *owedEnv) offered(t *testing.T, request ids.UUID) (activities.RepliedRequest, bool) {
	t.Helper()
	rows, err := activities.NewStore(o.e.DB()).RepliedRequests(o.judge(), time.Now(), 500)
	if err != nil {
		t.Fatalf("reading the settlement candidates: %v", err)
	}
	for _, row := range rows {
		if row.RequestID == request {
			return row, true
		}
	}
	return activities.RepliedRequest{}, false
}

// evidence is what the settlement model would read for the candidate.
func (o *owedEnv) evidence(t *testing.T, candidate activities.RepliedRequest) []threadMessage {
	t.Helper()
	var messages []threadMessage
	if err := database.WithWorkspaceTx(o.judge(), o.e.Pool, func(tx pgx.Tx) error {
		var err error
		messages, err = settleEvidence(o.judge(), tx, candidate, time.Now())
		return err
	}); err != nil {
		t.Fatalf("reading the settlement evidence: %v", err)
	}
	return messages
}

// A request answered by a reply that lost its thread stays owed until the
// settlement model judges that reply, and settles when it does.
func TestARequestAnsweredOffItsThreadSettlesOnThatAnswer(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.askOf(t, "Signed NDA", o.now.Add(-3*time.Hour))
	reply := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Re: Signed NDA", at: o.now.Add(-2 * time.Hour),
	})
	if o.threadKey(t, reply) == o.threadKey(t, request) {
		t.Fatal("capture joined the reply to the thread; the fixture no longer loses its thread")
	}

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("a request answered off its thread was never offered for judgement")
	}
	if candidate.NewestAnswerID != reply {
		t.Fatalf("the candidate is judged through %v, want the off-thread reply %v", candidate.NewestAnswerID, reply)
	}
	messages := o.evidence(t, candidate)
	last := messages[len(messages)-1]
	if messages[0].ID != request || last.ID != reply || last.OffThread != string(crmcontracts.ActivityKindEmail) {
		t.Fatalf("the model would read %+v; want the request first and the off-thread reply marked as such", messages)
	}
	if !o.owed(t, request) {
		t.Fatal("the request stopped being owed before anything judged the reply")
	}

	if err := activities.NewStore(o.e.DB()).SettleRequest(o.judge(), activities.RequestSettlementInput{
		Request: candidate, Verdict: activities.RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatalf("settling: %v", err)
	}
	if o.owed(t, request) {
		t.Fatal("a request settled on its off-thread answer is still owed")
	}
}

// A meeting held with the sender is an answer the model reads, marked as one.
func TestAHeldMeetingWithTheSenderIsSettlementEvidence(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.askOf(t, "Can we talk next week", o.now.Add(-5*time.Hour))
	subject, notes, held := "Rollout call", "Walked Pat through the phased rollout; agreed November.",
		string(crmcontracts.ActivityMeetingStatusHeld)
	at := o.now.Add(-time.Hour)
	if _, _, err := o.e.Activities.LogActivity(o.e.Admin(), activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, Body: &notes,
		OccurredAt: &at, Source: "manual", MeetingStatus: &held,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: pat}},
	}); err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("a request followed by a held meeting with the sender was never offered")
	}
	messages := o.evidence(t, candidate)
	last := messages[len(messages)-1]
	if last.OffThread != string(crmcontracts.ActivityKindMeeting) || last.Body != notes {
		t.Fatalf("the model would read %+v; want the held meeting last, marked as a meeting, with its notes", last)
	}
}

// A meeting with somebody else answers nothing of this sender's.
func TestAMeetingWithSomebodyElseOffersNothing(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	robin := o.contact(t, "Robin Buyer", "robin@other.example")
	request := o.askOf(t, "Can we talk next week", o.now.Add(-5*time.Hour))
	o.weLog(t, string(crmcontracts.ActivityKindMeeting), robin, o.now.Add(-time.Hour))

	if _, ok := o.offered(t, request); ok {
		t.Fatal("a meeting with another contact offered this sender's request for judgement")
	}
}

// A reply on the thread also matches the off-thread rule (our mail to the
// sender with the same subject). The model reads it once, as thread mail.
func TestAReplyOnTheThreadReachesTheModelOnce(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signed NDA",
		messageID: "nda@customer.example", at: o.now.Add(-3 * time.Hour),
	})
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
		"prompts-test", time.Now()); err != nil || !applied {
		t.Fatalf("judging the request: applied=%v err=%v", applied, err)
	}
	reply := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Re: Signed NDA", inReplyTo: "nda@customer.example",
		at: o.now.Add(-2 * time.Hour),
	})

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("a request with a reply on its thread was never offered")
	}
	var seen int
	for _, message := range o.evidence(t, candidate) {
		if message.ID == reply {
			seen++
			if message.OffThread != "" {
				t.Errorf("the thread reply reaches the model marked as %q", message.OffThread)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("the reply reaches the model %d times, want once", seen)
	}
}

// Past the window's size the newest answer is still in it, because the verdict
// is recorded through it.
func TestTheNewestAnswerIsInTheWindowPastItsSize(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.askOf(t, "Signed NDA", o.now.Add(-20*time.Hour))
	for i := range settleThreadMessages + 2 {
		o.weWrite(t, owedMail{
			to: "pat@customer.example", subject: "Re: Signed NDA",
			at: o.now.Add(time.Duration(i-19) * time.Hour),
		})
	}

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("the request was never offered")
	}
	messages := o.evidence(t, candidate)
	if len(messages) > settleThreadMessages {
		t.Fatalf("the window holds %d messages, past its size %d", len(messages), settleThreadMessages)
	}
	if messages[0].ID != request || messages[len(messages)-1].ID != candidate.NewestAnswerID {
		t.Fatalf("the window runs %v .. %v, want the request first and the newest answer %v last",
			messages[0].ID, messages[len(messages)-1].ID, candidate.NewestAnswerID)
	}
}

// Our reply on the request's thread, sent to a colleague of theirs with them in
// copy, is not in the thread read (it names another correspondent) but meets
// the same-subject rule. It still reaches the model as thread mail.
func TestAReplyOnTheThreadFoundBySubjectIsNotCalledASeparateEmail(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signed NDA",
		messageID: "nda-cc@customer.example", at: o.now.Add(-3 * time.Hour),
	})
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
		"prompts-test", time.Now()); err != nil || !applied {
		t.Fatalf("judging the request: applied=%v err=%v", applied, err)
	}
	reply := o.weWrite(t, owedMail{
		to: "legal@customer.example", cc: "pat@customer.example", subject: "Re: Signed NDA",
		inReplyTo: "nda-cc@customer.example", at: o.now.Add(-2 * time.Hour),
	})

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("the request was never offered")
	}
	for _, message := range o.evidence(t, candidate) {
		if message.ID == reply && message.OffThread != "" {
			t.Fatalf("a reply on the request's own thread reaches the model as %q", message.OffThread)
		}
	}
}
