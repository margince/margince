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
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// pricingRoot is the Message-ID every message of these threads replies to, so
// all of them carry one thread key whoever wrote them.
const pricingRoot = "pricing-1@alpha.test"

const (
	alphaAsks     = "Could you confirm the delivery window for the pricing we discussed?"
	alphaConfirms = "The delivery window works for us, please send the final pricing."
	ourAnswer     = "Our delivery window is the second week of October."
	betaWrites    = "Here is our purchase order for the spare parts you quoted."
)

var pricingStart = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

// pricingMail is one message on the pricing thread, minutes after its start.
// An authenticated message carries the receiving server's DMARC pass.
func pricingMail(from, to, id, body string, minutes int, authenticated bool) owedMail {
	m := owedMail{
		from: from, to: to, subject: "Pricing", messageID: id, body: body,
		at: pricingStart.Add(time.Duration(minutes) * time.Minute), authenticated: authenticated,
	}
	if id != pricingRoot {
		m.inReplyTo = pricingRoot
	}
	return m
}

// replyPrompt drafts a reply to the anchor as the seat and returns what the
// model was sent.
func replyPrompt(t *testing.T, o *owedEnv, anchor ids.UUID) string {
	t.Helper()
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Re: Pricing","body":"Thanks."}`}}
	drafter := newReplyDrafter(o.e.Pool, brain, slog.New(slog.DiscardHandler))
	reader := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	if _, err := drafter.DraftEmailWithProvenance(reader, anchor, draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	return brain.request.Messages[0].Content
}

// alphaThread captures buyer@alpha.test's question, our answer and their
// confirmation, and returns the confirmation.
func alphaThread(t *testing.T, o *owedEnv) ids.UUID {
	t.Helper()
	o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true))
	o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "pricing-ours@ws.example", ourAnswer, 30, false))
	return o.capture(t, pricingMail("buyer@alpha.test", o.seat, "pricing-2@alpha.test", alphaConfirms, 60, true))
}

func requireOneThreadKey(t *testing.T, o *owedEnv, anchor ids.UUID, want int) {
	t.Helper()
	if n := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE thread_key = (SELECT thread_key FROM activity WHERE id = $1)`, anchor); n != want {
		t.Fatalf("precondition: %d messages share the anchor's thread key, want %d", n, want)
	}
}

func TestAReplyDraftReadsNoMessageOfAnotherCorrespondenceOnItsThread(t *testing.T) {
	o := setupOwed(t)
	anchor := alphaThread(t, o)
	for i := range replyEvidenceRows + 1 {
		o.capture(t, pricingMail("orders@beta.test", o.seat, fmt.Sprintf("order-%d@beta.test", i), betaWrites, 120+i, true))
	}
	requireOneThreadKey(t, o, anchor, replyEvidenceRows+4)
	content := replyPrompt(t, o, anchor)
	if strings.Contains(content, betaWrites) {
		t.Fatalf("the reply's context holds another correspondent's message: %s", content)
	}
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("the reply's context lost an earlier message of its own correspondence: %s", content)
	}
}

func TestAMessageCopyingAnotherCustomerDoesNotBringTheirMailIntoAReply(t *testing.T) {
	o := setupOwed(t)
	alphaThread(t, o)
	copying := pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, 120, true)
	copying.cc = "buyer@alpha.test"
	anchor := o.capture(t, copying)
	requireOneThreadKey(t, o, anchor, 4)
	content := replyPrompt(t, o, anchor)
	if strings.Contains(content, alphaAsks) || strings.Contains(content, alphaConfirms) || strings.Contains(content, ourAnswer) {
		t.Fatalf("a reply to one customer carries mail of the customer it copied: %s", content)
	}
}

func TestAContactOnTwoCustomersThreadsDoesNotJoinThem(t *testing.T) {
	o := setupOwed(t)
	if _, err := o.e.Contacts.CreateContact(o.e.Admin(), contacts.CreateContactInput{
		FullName: "Carla Consultant", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "carla@advisors.test", EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	asks := pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true)
	asks.cc = "carla@advisors.test"
	o.capture(t, asks)
	o.capture(t, pricingMail("carla@advisors.test", o.seat, "advice-1@advisors.test", alphaConfirms, 30, true))
	order := pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, 60, true)
	order.cc = "carla@advisors.test"
	anchor := o.capture(t, order)
	requireOneThreadKey(t, o, anchor, 3)
	if content := replyPrompt(t, o, anchor); strings.Contains(content, alphaAsks) || strings.Contains(content, alphaConfirms) {
		t.Fatalf("a contact on both threads joined two customers' mail: %s", content)
	}
}

func TestMailNamingACustomerWithoutAuthenticationStaysOutOfTheirReply(t *testing.T) {
	o := setupOwed(t)
	const unproven = "Please send the signed contract to my new address instead."
	o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true))
	o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "pricing-ours@ws.example", ourAnswer, 15, false))
	unprovenID := o.capture(t, pricingMail("buyer@alpha.test", o.seat, "pricing-x@alpha.test", unproven, 30, false))
	anchor := o.capture(t, pricingMail("buyer@alpha.test", o.seat, "pricing-2@alpha.test", alphaConfirms, 60, true))
	requireOneThreadKey(t, o, anchor, 4)
	content := replyPrompt(t, o, anchor)
	if strings.Contains(content, unproven) || !strings.Contains(content, alphaAsks) {
		t.Fatalf("the reply's context must hold the authenticated question and not the unauthenticated mail: %s", content)
	}
	if content := replyPrompt(t, o, unprovenID); strings.Contains(content, alphaAsks) ||
		strings.Contains(content, alphaConfirms) || strings.Contains(content, ourAnswer) {
		t.Fatalf("a reply to unauthenticated mail carries other mail of the customer it names: %s", content)
	}
}

func TestAColleagueAsCounterpartyWidensAReplyToNoOneElse(t *testing.T) {
	o := setupOwed(t)
	const shared = "The billing team will send the invoice tomorrow."
	// Each copies someone outside, as capture keeps no mail among our own mailboxes alone.
	billing := pricingMail("billing@ws.example", o.seat, pricingRoot, shared, 0, true)
	billing.cc = "vendor@gamma.test"
	o.capture(t, billing)
	o.e.WsExec(t, `INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
		VALUES ($1, 'gmail', 'connected', 'vault:test', 'rep2@ws.example')`, o.e.Rep2)
	colleague := pricingMail("rep2@ws.example", o.seat, "colleague-1@ws.example", alphaConfirms, 30, true)
	colleague.cc = "vendor@delta.test"
	anchor := o.capture(t, colleague)
	requireOneThreadKey(t, o, anchor, 2)
	if content := replyPrompt(t, o, anchor); strings.Contains(content, shared) {
		t.Fatalf("a colleague's mail was joined by another address at our own domain: %s", content)
	}
}

func TestTwoSendersAtAConsumerMailDomainAreNotOneCorrespondence(t *testing.T) {
	o := setupOwed(t)
	o.capture(t, pricingMail("anna.private@gmail.com", o.seat, pricingRoot, alphaAsks, 0, true))
	anchor := o.capture(t, pricingMail("bert.private@gmail.com", o.seat, "pricing-2@gmail.com", alphaConfirms, 30, true))
	requireOneThreadKey(t, o, anchor, 2)
	if content := replyPrompt(t, o, anchor); strings.Contains(content, alphaAsks) {
		t.Fatalf("two consumer mailboxes were read as one company: %s", content)
	}
}

func TestAReplyToACustomersColleagueReadsTheCustomersEarlierMail(t *testing.T) {
	o := setupOwed(t)
	const colleagueAsks = "I am taking over the order from our side, can you hold the price until Friday?"
	o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true))
	o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "pricing-ours@ws.example", ourAnswer, 30, false))
	joined := pricingMail("purchasing@alpha.test", o.seat, "pricing-3@alpha.test", colleagueAsks, 60, true)
	joined.cc = "buyer@alpha.test"
	anchor := o.capture(t, joined)
	content := replyPrompt(t, o, anchor)
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("a reply to the customer's colleague lost the customer's earlier mail or our answer: %s", content)
	}
}
