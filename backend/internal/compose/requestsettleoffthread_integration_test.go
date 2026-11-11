// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
	var seen int
	for _, message := range o.evidence(t, candidate) {
		if message.ID != reply {
			continue
		}
		seen++
		if message.OffThread != "" {
			t.Fatalf("a reply on the request's own thread reaches the model as %q", message.OffThread)
		}
	}
	if seen != 1 {
		t.Fatalf("the reply reaches the model %d times, want once", seen)
	}
}

// Our reply filed before a request dated by its day, at that day's stored instant, answers it (occurredday.Answers).
// The model reads that reply even under another subject, so the request the pass offers is judged.
func TestADayTieReplyFiledFirstReachesTheModel(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	reply := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Countersigned copy", inReplyTo: "dated@customer.example",
		at: o.now.Add(-72 * time.Hour),
	})
	request := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signed NDA",
		messageID: "dated@customer.example", at: o.now.Add(-72 * time.Hour),
	})
	if o.threadKey(t, reply) != o.threadKey(t, request) {
		t.Fatal("capture left the reply off the request's thread; the fixture proves nothing")
	}
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity
			SET occurred_at = $3, occurred_precision = CASE WHEN id = $1 THEN 'day' ELSE occurred_precision END
			WHERE id IN ($1, $2)`, request, reply, occurredday.Anchor(o.now.Add(-72*time.Hour)))
		return err
	}); err != nil {
		t.Fatalf("dating the request by its day: %v", err)
	}
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
		"prompts-test", clocktest.Now(t)); err != nil || !applied {
		t.Fatalf("judging the request: applied=%v err=%v", applied, err)
	}

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("a request our reply answers by its day was never offered")
	}
	if !slices.ContainsFunc(o.evidence(t, candidate), func(message threadMessage) bool { return message.ID == reply }) {
		t.Fatal("the reply that answers the request never reaches the model")
	}
}

// askWithTiedCorrection captures a request, their second mail in its own second on its thread, and our later reply.
func (o *owedEnv) askWithTiedCorrection(t *testing.T) (request, correction ids.UUID) {
	t.Helper()
	o.contact(t, "Pat Buyer", "pat@customer.example")
	asked := clocktest.Now(t).Add(-3 * time.Hour).Truncate(time.Second)
	request = o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signed NDA", messageID: "same-second@customer.example", at: asked,
	})
	correction = o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Re: Signed NDA", inReplyTo: "same-second@customer.example",
		messageID: "correction@customer.example", at: asked, authenticated: true,
	})
	if o.threadKey(t, correction) != o.threadKey(t, request) {
		t.Fatal("capture left the correction off the request's thread; the fixture proves nothing")
	}
	if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
		"prompts-test", clocktest.Now(t)); err != nil || !applied {
		t.Fatalf("judging the request: applied=%v err=%v", applied, err)
	}
	o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "Re: Signed NDA", inReplyTo: "same-second@customer.example",
		at: asked.Add(time.Hour),
	})
	return request, correction
}

// Their second mail in the request's own second is no answer, but the model still reads it: it may correct the ask.
func TestTheirMailAtTheRequestsInstantReachesTheModel(t *testing.T) {
	o := setupOwed(t)
	request, correction := o.askWithTiedCorrection(t)

	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("a request with a reply on its thread was never offered")
	}
	if candidate.NewestAnswerID == correction {
		t.Fatal("their mail at the request's instant was taken for an answer")
	}
	if !slices.ContainsFunc(o.evidence(t, candidate), func(message threadMessage) bool { return message.ID == correction }) {
		t.Fatal("their mail at the request's instant never reaches the model")
	}
}

// The verdict is bound to their tied mail as well. Edited after the model read it, the verdict is dropped.
func TestAVerdictOverAnEditedTiedMailIsDropped(t *testing.T) {
	o := setupOwed(t)
	request, correction := o.askWithTiedCorrection(t)
	candidate, ok := o.offered(t, request)
	if !ok || !o.owed(t, request) {
		t.Fatalf("the request was not offered while owed: offered=%v", ok)
	}

	body := "Please send the countersigned copy instead."
	if _, err := o.e.Activities.UpdateActivity(o.e.Admin(), ids.From[ids.ActivityKind](correction),
		activities.UpdateActivityInput{Body: &body}); err != nil {
		t.Fatalf("editing their tied mail: %v", err)
	}
	if err := activities.NewStore(o.e.DB()).SettleRequest(o.judge(), activities.RequestSettlementInput{
		Request: candidate, Verdict: activities.RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatalf("settling: %v", err)
	}
	if !o.owed(t, request) {
		t.Fatal("a verdict over a tied mail edited after the model read it settled the request")
	}
}

func TestSettlementReadsTheFilesOnTheActualAnswer(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.askOf(t, "Send the report", o.now.Add(-3*time.Hour))
	reply := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Send the report", body: "The report is attached.", at: o.now.Add(-2 * time.Hour)})
	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("answer not offered")
	}
	assertFiles := func(want int) {
		t.Helper()
		messages := o.evidence(t, candidate)
		found := false
		for _, message := range messages {
			if message.ID == reply {
				found = true
				if len(message.Attachments) != want {
					t.Fatalf("answer files=%v want %d", message.Attachments, want)
				}
			}
		}
		if !found {
			t.Fatal("answer absent from evidence")
		}
	}
	assertFiles(0)
	fileStore := activities.NewStore(o.e.DB()).WithBlobstore(blobstore.NewMemory())
	file, err := fileStore.UploadAttachment(o.e.Admin(), activities.AttachmentInput{
		EntityType: "activity", EntityID: reply, Filename: "report.txt", ContentType: "text/plain", Content: strings.NewReader("Synthetic report"),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(0)
	actor := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	store := comms.NewStore(o.e.DB(), func() time.Time { return o.now }, o.e.Activities)
	var delivery ids.UUID
	if err := database.WithWorkspaceTx(actor, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		delivery, err = store.StageTx(actor, tx, comms.StageInput{ActivityID: ids.From[ids.ActivityKind](reply), Provider: "gmail", MessageID: ids.NewV7().String() + "@ws.example", Recipients: []string{"pat@customer.example"}, Attachments: []comms.OutboundFile{{AttachmentID: ids.UUID(file.Id), Filename: "report.txt"}}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	assertFiles(0)
	if err := store.RecordSent(sendWorkerScope(actor), delivery, connector.SendReceipt{ProviderMessageID: "report-delivery"}); err != nil {
		t.Fatal(err)
	}
	if err := fileStore.ArchiveAttachment(o.e.Admin(), ids.UUID(file.Id)); err != nil {
		t.Fatal(err)
	}
	assertFiles(1)
	mail := owedMail{from: o.seat, to: "pat@customer.example", subject: "Re: Send the report", at: o.now.Add(-time.Hour), messageID: ids.NewV7().String() + "@ws.example"}
	raw := strings.Replace(string(mail.raw()), "Content-Type: text/plain; charset=utf-8\r\n\r\nText.\r\n",
		"Content-Type: multipart/mixed; boundary=REPORT\r\n\r\n--REPORT\r\nContent-Type: text/plain\r\n\r\nThe report is attached.\r\n--REPORT\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=report.txt\r\n\r\nSynthetic report\r\n--REPORT--\r\n", 1)
	parsed, err := mailmap.Parse([]byte(raw), o.seat)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{Blob: blobstore.NewMemory()}).Upsert(threadConnectorCtx(o.e, o.owner), parsed.AttestSentByOwner(true).ToRecord("gmail", []byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	reply = ref.ID
	candidate, ok = o.offered(t, request)
	if !ok {
		t.Fatal("file answer not offered")
	}
	assertFiles(1)
}
