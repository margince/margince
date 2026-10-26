// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A mail whose text broke while decoding keeps the start for a reader. No
// automatic work is read from it: no promise, no request, no settlement. Each
// case captures the mail through the real parser and sink. Each has a control
// whose text decoded in full.

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// brokenAfter is base64 text that decodes, then a line that does not.
func brokenAfter(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text)) + "\r\n!!!not base64!!!"
}

// readInFull is the same text with nothing broken after it.
func readInFull(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(text))
}

func (o *owedEnv) completeness(t *testing.T, message ids.UUID) string {
	t.Helper()
	var got string
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(o.e.Admin(), `SELECT body_completeness FROM activity WHERE id = $1`, message).Scan(&got)
	}); err != nil {
		t.Fatalf("reading body_completeness: %v", err)
	}
	return got
}

func TestAPromiseReadOnlyInPartIsNotCurrentWork(t *testing.T) {
	promise := "I will send the pricing by Friday."
	for _, tc := range []struct {
		name, body, completeness string
		// laterAnswer is a reply to the promise, read only in part, when set.
		laterAnswer string
		want        CommitmentOutcome
	}{
		{"read in part", brokenAfter(promise), "partial", "", CommitmentOutcome(activities.CommitmentSourceIncomplete)},
		{
			"answered by a mail read in part", readInFull(promise), "complete",
			brokenAfter("No need, we cancelled."), CommitmentOutcome(activities.CommitmentIncomplete),
		},
		{"read in full", readInFull(promise), "complete", "", CommitmentTaskWritten},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			source := o.weWrite(t, owedMail{
				to: "pat@customer.example", subject: "Pricing", body: tc.body, encoding: "base64",
				messageID: "pricing@ws.example", at: o.now.Add(-2 * time.Hour),
			})
			read := []ids.UUID{source}
			if tc.laterAnswer != "" {
				read = append(read, o.capture(t, owedMail{
					from: "pat@customer.example", to: o.seat, subject: "Re: Pricing", inReplyTo: "pricing@ws.example",
					messageID: "pricing-answer@customer.example", body: tc.laterAnswer, encoding: "base64",
					authenticated: true, at: o.now.Add(-time.Hour),
				}))
			}
			if got := o.completeness(t, source); got != tc.completeness {
				t.Fatalf("body_completeness = %q, want %q", got, tc.completeness)
			}
			c := Commitment{
				Meaning: openCommitmentMeaning("", promise), AsOf: o.now, SourceActivityID: source,
				ReadMessages: read, Extractor: signalScanActor, Seat: &o.owner,
				Summary: "Send the pricing", Quote: promise, Body: promise, Confidence: 0.95,
				Locator: commitmentLocator(source, claimKindOurs, o.owner.String(), promise),
			}
			ctx := o.judge()
			dispatcher := NewCommitmentDispatcher(o.e.Pool, approvals.NewService(o.e.DB()))
			var outcome CommitmentOutcome
			if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
				var err error
				outcome, _, err = dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if outcome != tc.want {
				t.Fatalf("outcome = %q, want %q", outcome, tc.want)
			}
		})
	}
}

func TestARequestIsNotJudgedFromAMailReadOnlyInPart(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		offered    bool
	}{
		{"read in part", brokenAfter("Could you send the signed NDA?"), false},
		{"read in full", readInFull("Could you send the signed NDA?"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			ask := o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: "NDA", body: tc.body, encoding: "base64",
				messageID: "nda-ask@customer.example", at: o.now.Add(-time.Hour),
			})
			backlog, _, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "prompts-test", o.now, 100, 400, 400)
			if err != nil {
				t.Fatalf("reading the owed backlog: %v", err)
			}
			var offered bool
			for _, candidate := range backlog {
				offered = offered || candidate.ID == ask
			}
			if offered != tc.offered {
				t.Fatalf("offered for judgement = %v, want %v", offered, tc.offered)
			}
		})
	}
}

// A re-ask after our reply is what keeps a request owed. Read only in part, it
// may be one, so the conversation is not judged at all.
func TestARequestWithAMessageReadOnlyInPartIsNotSettled(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		judged     bool
	}{
		{"read in part", brokenAfter("Thanks, but this is the wrong NDA."), false},
		{"read in full", readInFull("Thanks, but this is the wrong NDA."), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			request := o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: "Signed NDA",
				messageID: "nda@customer.example", at: o.now.Add(-3 * time.Hour),
			})
			// The verdict's compare-and-swap takes the database's own read time.
			_, readAt, err := activities.NewStore(o.e.DB()).OwedBacklog(o.judge(), "prompts-test", o.now, 1, 400, 400)
			if err != nil {
				t.Fatalf("reading the owed backlog: %v", err)
			}
			if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
				"prompts-test", readAt); err != nil || !applied {
				t.Fatalf("judging the request: applied=%v err=%v", applied, err)
			}
			o.weWrite(t, owedMail{
				to: "pat@customer.example", subject: "Re: Signed NDA", inReplyTo: "nda@customer.example",
				messageID: "nda-reply@ws.example", at: o.now.Add(-2 * time.Hour),
			})
			reask := o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: "Re: Signed NDA", inReplyTo: "nda@customer.example",
				messageID: "nda-reask@customer.example", body: tc.body, encoding: "base64", authenticated: true, at: o.now.Add(-time.Hour),
			})
			// Not offered at all when a message was read only in part, so a
			// blocked request cannot hold a place in every batch.
			candidate, ok := o.offered(t, request)
			if ok != tc.judged {
				t.Fatalf("offered for settlement = %v, want %v", ok, tc.judged)
			}
			if !ok {
				return
			}
			evidence := o.evidence(t, candidate)
			var reasked bool
			for _, message := range evidence {
				reasked = reasked || message.ID == reask
			}
			if judged := len(evidence) > 0; judged != tc.judged || (judged && !reasked) {
				t.Fatalf("judged = %v with the re-ask in the evidence = %v, want judged = %v", judged, reasked, tc.judged)
			}
		})
	}
}

// Requests the pass refuses to judge never reach the candidate list, so they
// cannot fill every batch ahead of one it can judge.
func TestRequestsBlockedOnPartialMailDoNotStarveSettlement(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	store := activities.NewStore(o.e.DB())
	_, readAt, err := store.OwedBacklog(o.judge(), "prompts-test", o.now, 1, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	ask := func(n int, replyAt time.Time, reask string) ids.UUID {
		t.Helper()
		key := fmt.Sprintf("ask-%d@customer.example", n)
		request := o.capture(t, owedMail{
			from: "pat@customer.example", to: o.seat, subject: fmt.Sprintf("Question %d", n),
			messageID: key, at: o.now.Add(-5 * time.Hour),
		})
		if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs,
			"prompts-test", readAt); err != nil || !applied {
			t.Fatalf("judging request %d: applied=%v err=%v", n, applied, err)
		}
		o.weWrite(t, owedMail{to: "pat@customer.example", subject: fmt.Sprintf("Re: Question %d", n), inReplyTo: key, at: replyAt})
		if reask != "" {
			o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: fmt.Sprintf("Re: Question %d", n), inReplyTo: key,
				messageID: fmt.Sprintf("reask-%d@customer.example", n), body: reask, encoding: "base64",
				authenticated: true, at: replyAt.Add(time.Minute),
			})
		}
		return request
	}
	settleable := ask(0, o.now.Add(-3*time.Hour), "")
	blocked := make([]ids.UUID, 0, 5)
	for n := 1; n <= 5; n++ {
		blocked = append(blocked, ask(n, o.now.Add(-2*time.Hour), brokenAfter("This is not what I asked for.")))
	}
	batch, err := store.RepliedRequests(o.judge(), o.now, settleBatchSize)
	if err != nil {
		t.Fatal(err)
	}
	offered := map[ids.UUID]bool{}
	for _, candidate := range batch {
		offered[candidate.RequestID] = true
	}
	if !offered[settleable] {
		t.Errorf("the request the pass can judge was not in the batch of %d", len(batch))
	}
	for _, request := range blocked {
		if offered[request] {
			t.Errorf("request %v is offered although a message on it was read only in part", request)
		}
	}
}

// The window marks a message read only in part, so the reading refuses the
// conversation rather than presenting the rest of it as current.
func TestTheEventWindowMarksAMessageReadOnlyInPart(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	at := clocktest.Now(t).UTC().Add(-48 * time.Hour)
	const key = "thread-partial-window"
	for i, row := range []struct{ body, completeness string }{
		{"Happy to continue the contract.", "complete"},
		{"We end the contract at the end of the month.", "partial"},
	} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (kind, direction, subject, body, thread_key, occurred_at, created_at,
			                      source, captured_by, audience, body_completeness,
			                      counterparty_email, inbound_sender_authenticated)
			VALUES ('email', 'inbound', 'Contract', $1, $2, $3, $3, 'gmail', 'connector:gmail', 'workspace', $4,
			        'buyer@contract.test', true)`,
			row.body, key, at.Add(time.Duration(i)*time.Minute), row.completeness); err != nil {
			t.Fatal(err)
		}
	}
	thread := settledThread{Key: key, Newest: at.Add(time.Minute), Count: 2}
	var window []threadMessage
	as := e.Admin()
	if err := database.WithWorkspaceTx(as, e.Pool, func(tx pgx.Tx) error {
		var err error
		window, err = threadMessages(as, tx, &thread)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(window) != 2 {
		t.Fatalf("window = %+v, want both messages", window)
	}
	for _, m := range window {
		if want := m.Body != "Happy to continue the contract."; m.Incomplete != want {
			t.Errorf("message %q marked incomplete = %v, want %v", m.Body, m.Incomplete, want)
		}
	}
}

// A forward that says nothing of its own carries the original's words, which
// are never read as a promise of the mail's sender.
func TestAForwardedOriginalIsNotItsSendersPromise(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	promise := "We will refund you in full."
	raw := []byte(strings.Join([]string{
		"From: " + o.seat, "To: pat@customer.example", "Subject: Fwd: refund",
		"Date: " + o.now.Add(-time.Hour).Format(time.RFC1123Z), "Message-ID: <fwd-refund@ws.example>",
		`Content-Type: multipart/mixed; boundary="F"`, "",
		"--F", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64", "", "!!!not base64!!!",
		"--F", "Content-Type: message/rfc822", "",
		"From: Other Shop <shop@elsewhere.test>", "Subject: refund", "Content-Type: text/plain", "", promise,
		"--F--", "",
	}, "\r\n"))
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(
		threadConnectorCtx(o.e, o.owner), parsed.AttestSentByOwner(true).ToRecord("gmail", raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := o.completeness(t, ref.ID); got != "forwarded" {
		t.Fatalf("body_completeness = %q, want forwarded", got)
	}
	if got := o.promiseOutcome(t, ref.ID, promise); got != CommitmentOutcome(activities.CommitmentSourceIncomplete) {
		t.Errorf("a forwarded original's words were admitted as the sender's promise: %q", got)
	}
}

// Our own last message is the context a request is judged in. Read only in
// part, it may hold the very answer, so the request is not judged on it.
func TestARequestIsNotJudgedAgainstOurLastMessageReadOnlyInPart(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		offered    bool
	}{
		{"read in part", brokenAfter("Here is the pricing you asked for."), false},
		{"read in full", readInFull("Here is the pricing you asked for."), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			o.weWrite(t, owedMail{
				to: "pat@customer.example", subject: "Pricing", body: tc.body, encoding: "base64",
				messageID: "our-pricing@ws.example", at: o.now.Add(-2 * time.Hour),
			})
			ask := o.capture(t, owedMail{
				from: "pat@customer.example", to: o.seat, subject: "Re: Pricing", inReplyTo: "our-pricing@ws.example",
				messageID: "their-ask@customer.example", body: "Could you also send the contract?",
				authenticated: true, at: o.now.Add(-time.Hour),
			})
			if got := o.offeredAsRequest(t, ask); got != tc.offered {
				t.Fatalf("offered for judgement = %v, want %v", got, tc.offered)
			}
		})
	}
}
