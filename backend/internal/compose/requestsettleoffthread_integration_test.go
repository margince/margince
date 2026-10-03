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

// askOf captures a mail from the address and judges it a request.
func (o *owedEnv) askOf(t *testing.T, from, subject string, at time.Time) ids.UUID {
	t.Helper()
	request := o.customerWrites(t, from, subject, at)
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
	request := o.askOf(t, "pat@customer.example", "Signed NDA", o.now.Add(-3*time.Hour))
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
	request := o.askOf(t, "pat@customer.example", "Can we talk next week", o.now.Add(-5*time.Hour))
	o.weLog(t, string(crmcontracts.ActivityKindMeeting), pat, o.now.Add(-time.Hour))

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("a request followed by a held meeting with the sender was never offered")
	}
	messages := o.evidence(t, candidate)
	if last := messages[len(messages)-1]; last.OffThread != string(crmcontracts.ActivityKindMeeting) {
		t.Fatalf("the model would read %+v; want the held meeting last, marked as a meeting", messages)
	}
}

// A meeting with somebody else answers nothing of this sender's.
func TestAMeetingWithSomebodyElseOffersNothing(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	robin := o.contact(t, "Robin Buyer", "robin@other.example")
	request := o.askOf(t, "pat@customer.example", "Can we talk next week", o.now.Add(-5*time.Hour))
	o.weLog(t, string(crmcontracts.ActivityKindMeeting), robin, o.now.Add(-time.Hour))

	if _, ok := o.offered(t, request); ok {
		t.Fatal("a meeting with another contact offered this sender's request for judgement")
	}
}
