// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// Reply language may read related correspondence; reply content stays anchored
// to the selected message, so these checks inspect the language reader itself.
func replyLanguageReading(t *testing.T, o *owedEnv, anchor ids.UUID) string {
	t.Helper()
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Re: Pricing","body":"Thanks."}`}}
	drafter := newReplyDrafter(o.e.Pool, brain, slog.New(slog.DiscardHandler))
	reader := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	if _, err := drafter.DraftEmailWithProvenance(reader, anchor, draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(brain.request.Messages[0].Content, `"conversation":`) {
		t.Fatal("related correspondence reached the content prompt")
	}
	_, written, err := drafter.languageInputs(reader, anchor, draftfloor.Steering{})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(written.Thread, "\n")
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
	content := replyLanguageReading(t, o, anchor)
	if strings.Contains(content, betaWrites) {
		t.Fatalf("the reply's language evidence holds another correspondent's message: %s", content)
	}
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("the reply's language evidence lost an earlier message of its own correspondence: %s", content)
	}
}

func TestAMessageCopyingAnotherCustomerDoesNotBringTheirMailIntoAReply(t *testing.T) {
	o := setupOwed(t)
	alphaThread(t, o)
	copying := pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, 120, true)
	copying.cc = "buyer@alpha.test"
	anchor := o.capture(t, copying)
	requireOneThreadKey(t, o, anchor, 4)
	content := replyLanguageReading(t, o, anchor)
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
	if content := replyLanguageReading(t, o, anchor); strings.Contains(content, alphaAsks) || strings.Contains(content, alphaConfirms) {
		t.Fatalf("a contact on both threads joined two customers' mail: %s", content)
	}
}

func TestUnauthenticatedMailStaysOutOfAProvenReplyAndReadsOnlyItsOwnSender(t *testing.T) {
	o := setupOwed(t)
	const unproven = "Please send the signed contract to my new address instead."
	o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true))
	o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "pricing-ours@ws.example", ourAnswer, 15, false))
	o.capture(t, pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, 20, true))
	unprovenID := o.capture(t, pricingMail("buyer@alpha.test", o.seat, "pricing-x@alpha.test", unproven, 30, false))
	anchor := o.capture(t, pricingMail("buyer@alpha.test", o.seat, "pricing-2@alpha.test", alphaConfirms, 60, true))
	requireOneThreadKey(t, o, anchor, 5)
	content := replyLanguageReading(t, o, anchor)
	if strings.Contains(content, unproven) || !strings.Contains(content, alphaAsks) {
		t.Fatalf("the reply's language evidence must hold the authenticated question and not the unauthenticated mail: %s", content)
	}
	// The reply to the unauthenticated mail goes back to buyer@alpha.test, so
	// it may read our mail with that address, and nothing from anyone else.
	content = replyLanguageReading(t, o, unprovenID)
	if !strings.Contains(content, ourAnswer) || strings.Contains(content, betaWrites) {
		t.Fatalf("a reply to unauthenticated mail must read our mail with its sender and no other correspondent's: %s", content)
	}
}

func TestAReplyGoingToSomeoneOtherThanAnUnprovenSenderReadsTheMessageAlone(t *testing.T) {
	o := setupOwed(t)
	o.e.WsExec(t, `INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
		VALUES ($1, 'gmail', 'connected', 'vault:test', 'rep2@ws.example')`, o.e.Rep2)
	const earlier = "Internal note: the discount stays at five percent."
	note := pricingMail("rep2@ws.example", o.seat, pricingRoot, earlier, 0, false)
	note.cc = "vendor@gamma.test"
	o.capture(t, note)
	// Its From names a colleague, so a reply is sent to the customer it copies.
	copying := pricingMail("rep2@ws.example", o.seat, "pricing-2@ws.example", alphaConfirms, 30, false)
	copying.cc = "buyer@alpha.test"
	anchor := o.capture(t, copying)
	requireOneThreadKey(t, o, anchor, 2)
	if counterparty := o.e.WsScalar(t, `SELECT counterparty_email FROM activity WHERE id = $1`, anchor); counterparty != "rep2@ws.example" {
		t.Fatalf("precondition: the anchor's counterparty is %q, want the colleague it names", counterparty)
	}
	if content := replyLanguageReading(t, o, anchor); strings.Contains(content, earlier) {
		t.Fatalf("a reply sent to someone else carries mail of the unproven sender: %s", content)
	}
}

func TestAnUnauthenticatedSendersReplyReadsTheirMailAndNoOneElses(t *testing.T) {
	o := setupOwed(t)
	const bWrites = "Please also quote the maintenance contract for our second site."
	o.capture(t, pricingMail("anna@customer.test", o.seat, pricingRoot, alphaAsks, 0, false))
	o.capture(t, pricingMail(o.seat, "anna@customer.test", "pricing-ours@ws.example", ourAnswer, 15, false))
	o.capture(t, pricingMail("bert@customer.test", o.seat, "pricing-b@customer.test", bWrites, 20, true))
	o.capture(t, pricingMail("orders@beta.test", o.seat, "order-1@beta.test", betaWrites, 25, true))
	anchor := o.capture(t, pricingMail("anna@customer.test", o.seat, "pricing-2@customer.test", alphaConfirms, 30, false))
	requireOneThreadKey(t, o, anchor, 5)
	content := replyLanguageReading(t, o, anchor)
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("the reply lost the earlier mail between us and its sender: %s", content)
	}
	if strings.Contains(content, bWrites) || strings.Contains(content, betaWrites) {
		t.Fatalf("the reply to an unauthenticated sender reads mail of someone else: %s", content)
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
	if content := replyLanguageReading(t, o, anchor); strings.Contains(content, shared) {
		t.Fatalf("a colleague's mail was joined by another address at our own domain: %s", content)
	}
}

func TestTwoSendersAtAConsumerMailDomainAreNotOneCorrespondence(t *testing.T) {
	o := setupOwed(t)
	o.capture(t, pricingMail("anna.private@gmail.com", o.seat, pricingRoot, alphaAsks, 0, true))
	anchor := o.capture(t, pricingMail("bert.private@gmail.com", o.seat, "pricing-2@gmail.com", alphaConfirms, 30, true))
	requireOneThreadKey(t, o, anchor, 2)
	if content := replyLanguageReading(t, o, anchor); strings.Contains(content, alphaAsks) {
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
	content := replyLanguageReading(t, o, anchor)
	if !strings.Contains(content, alphaAsks) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("a reply to the customer's colleague lost the customer's earlier mail or our answer: %s", content)
	}
}

func TestASelectedColleagueInboundMessageIsDraftedAsOurFollowUp(t *testing.T) {
	e := integration.Setup(t)
	reader := e.As(e.AdminUser, nil, importerPerms())
	status, activity := postActivity(reader, t, e, crmcontracts.CreateActivityRequest{
		Source: "manual", Kind: "email", Subject: new("Pricing"), Body: new(ourAnswer),
		Direction:    new(crmcontracts.CreateActivityRequestDirectionInbound),
		SourceSystem: new("mail_import"), SourceId: new("colleague-inbound"), SourceAuthorId: new(openapi_types.UUID(e.Rep2)),
	})
	if status != http.StatusCreated {
		t.Fatalf("import status = %d", status)
	}
	anchor := ids.UUID(activity.Id)
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Pricing","body":"Hello,\n\nCould we confirm the delivery window?"}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	activity, _, err := drafter.languageInputs(reader, anchor, draftfloor.Steering{})
	if err != nil {
		t.Fatal(err)
	}
	if activity.Direction == nil || *activity.Direction != "inbound" {
		t.Fatal("precondition: the colleague's mail did not arrive inbound")
	}
	if activity.Voice == nil || activity.Voice.Author == nil {
		t.Fatal("precondition: the colleague's mail has no author")
	}
	if activity.Voice.Author.Kind != "colleague" {
		t.Fatalf("precondition: author kind = %q", activity.Voice.Author.Kind)
	}
	if _, err := drafter.DraftEmailWithProvenance(reader, anchor, draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	content := brain.request.Messages[0].Content
	if strings.Contains(content, `"thread":"inbound_mail"`) || !strings.Contains(content, ourAnswer) {
		t.Fatalf("a colleague's message was read as their turn: %s", content)
	}
}
