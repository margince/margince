// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

var replyThreadStart = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

// replyThreadMail is one captured message of the "Pricing" thread rooted at
// <pricing-1@alpha.test>. An empty references starts the thread.
type replyThreadMail struct {
	from, to, cc, id, references, body string
	offset                             time.Duration
}

// captureReplyThreadMail files the message through the capture sink into the
// seat's mailbox and returns the activity it became.
func captureReplyThreadMail(t *testing.T, e *integration.Env, seat string, mail replyThreadMail) ids.UUID {
	t.Helper()
	headers := []string{
		"From: " + mail.from, "To: " + mail.to, "Subject: Pricing",
		"Message-ID: <" + mail.id + ">", "Date: " + replyThreadStart.Add(mail.offset).Format(time.RFC1123Z),
	}
	if mail.cc != "" {
		headers = append(headers, "Cc: "+mail.cc)
	}
	if mail.references != "" {
		headers = append(headers, "References: "+mail.references, "In-Reply-To: <pricing-1@alpha.test>")
	}
	raw := []byte(strings.Join(append(headers, "Content-Type: text/plain; charset=utf-8", "", mail.body), "\r\n"))
	parsed, err := mailmap.Parse(raw, seat)
	if err != nil {
		t.Fatalf("parsing %s: %v", mail.id, err)
	}
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(threadConnectorCtx(e, e.Rep1), parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing %s: %v", mail.id, err)
	}
	return ref.ID
}

// replyThreadPrompt drafts a reply to the anchor as the seat and returns what
// the model was sent.
func replyThreadPrompt(t *testing.T, e *integration.Env, anchor ids.UUID) string {
	t.Helper()
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Re: Pricing","body":"Thanks."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := drafter.DraftEmailWithProvenance(reader, anchor, draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	return brain.request.Messages[0].Content
}

const (
	alphaAsks     = "Could you confirm the delivery window for the pricing we discussed?"
	alphaConfirms = "The delivery window works for us, please send the final pricing."
	betaWrites    = "Here is our purchase order for the spare parts you quoted."
)

func TestAReplyDraftReadsNoMessageOfAnotherCorrespondenceOnItsThread(t *testing.T) {
	e := integration.Setup(t)
	seat := seatAddress(t, e, e.Rep1)
	captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "buyer@alpha.test", to: seat, id: "pricing-1@alpha.test", body: alphaAsks,
	})
	anchor := captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "buyer@alpha.test", to: seat, id: "pricing-2@alpha.test",
		references: "<pricing-1@alpha.test>", body: alphaConfirms, offset: time.Hour,
	})
	// A second correspondent's mail names the same root and lands on the same
	// thread key. It outnumbers the page and is newer than all of the first's mail.
	for i := range replyEvidenceRows + 1 {
		captureReplyThreadMail(t, e, seat, replyThreadMail{
			from: "orders@beta.test", to: seat, id: fmt.Sprintf("order-%d@beta.test", i),
			references: "<pricing-1@alpha.test>", body: betaWrites, offset: 2*time.Hour + time.Duration(i)*time.Minute,
		})
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE thread_key = (SELECT thread_key FROM activity WHERE id = $1)
		AND counterparty_email = 'orders@beta.test'`, anchor); n != replyEvidenceRows+1 {
		t.Fatalf("precondition: %d of the second correspondent's messages share the thread key, want %d", n, replyEvidenceRows+1)
	}
	content := replyThreadPrompt(t, e, anchor)
	if strings.Contains(content, betaWrites) {
		t.Fatalf("the reply's context holds another correspondent's message: %s", content)
	}
	if !strings.Contains(content, alphaAsks) {
		t.Fatalf("the correspondent's earlier message lost its place in the reply's context: %s", content)
	}
}

func TestAReplyDraftReadsTheEarlierMessagesOfItsOwnCorrespondence(t *testing.T) {
	e := integration.Setup(t)
	seat := seatAddress(t, e, e.Rep1)
	const ourAnswer = "Our delivery window is the second week of October."
	captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "buyer@alpha.test", to: seat, id: "pricing-1@alpha.test", body: alphaAsks,
	})
	captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: seat, to: "buyer@alpha.test", id: "pricing-ours@ws.example",
		references: "<pricing-1@alpha.test>", body: ourAnswer, offset: time.Hour,
	})
	anchor := captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "buyer@alpha.test", to: seat, id: "pricing-2@alpha.test",
		references: "<pricing-1@alpha.test> <pricing-ours@ws.example>", body: alphaConfirms, offset: 2 * time.Hour,
	})
	content := replyThreadPrompt(t, e, anchor)
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("the reply's context lost an earlier message of its own correspondence: %s", content)
	}
}

func TestAReplyToANewlyCopiedPersonStillReadsTheOriginalCorrespondentsMail(t *testing.T) {
	e := integration.Setup(t)
	seat := seatAddress(t, e, e.Rep1)
	const copiedAsks = "I am taking over the order from our side, can you hold the price until Friday?"
	first := captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "buyer@alpha.test", to: seat, id: "pricing-1@alpha.test", body: alphaAsks,
	})
	captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "buyer@alpha.test", to: seat, cc: "purchasing@alpha.test", id: "pricing-2@alpha.test",
		references: "<pricing-1@alpha.test>", body: alphaConfirms, offset: time.Hour,
	})
	anchor := captureReplyThreadMail(t, e, seat, replyThreadMail{
		from: "purchasing@alpha.test", to: "buyer@alpha.test", cc: seat, id: "pricing-3@alpha.test",
		references: "<pricing-1@alpha.test> <pricing-2@alpha.test>", body: copiedAsks, offset: 2 * time.Hour,
	})
	content := replyThreadPrompt(t, e, anchor)
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, alphaConfirms) {
		t.Fatalf("a reply to the copied person lost the original correspondent's mail: %s", content)
	}
	if content := replyThreadPrompt(t, e, first); !strings.Contains(content, copiedAsks) {
		t.Fatalf("a reply to the original correspondent lost the copied person's mail to them: %s", content)
	}
}
