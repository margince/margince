// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settled is what the company scan reads about one cited message, as the
// mailbox owner, for an account whose contacts are recipients.
func (o *owedEnv) settled(t *testing.T, message ids.UUID, recipients ...ids.UUID) bool {
	t.Helper()
	return o.settledAs(t, o.reader(), message, recipients...)
}

func (o *owedEnv) settledAs(t *testing.T, reader context.Context, message ids.UUID, recipients ...ids.UUID) bool {
	t.Helper()
	roster := make([]ids.ContactID, 0, len(recipients))
	for _, recipient := range recipients {
		roster = append(roster, ids.ContactID{UUID: recipient})
	}
	var states map[ids.UUID]bool
	if err := database.WithWorkspaceTx(reader, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		states, err = activities.SettledCitations(reader, tx, []ids.UUID{message}, roster, o.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := states[message]
	if !ok {
		t.Fatalf("no answer for the cited message %s", message)
	}
	return got
}

// An unconfirmed ask settles on our reply in its thread. A confirmed one
// settles only on a settled verdict, as the Worklist's owed state does: a
// reply alone discharges nothing.
func TestAScanCitationSettlesByTheWorklistsRule(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ask := o.customerWrites(t, "pat@customer.example", "Listing details", o.now.Add(-6*time.Hour))
	if o.settled(t, ask) {
		t.Fatal("an ask nobody answered reads settled")
	}
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Listing details",
		inReplyTo: o.messageIDOf(t, ask), at: o.now.Add(-5 * time.Hour)})
	if !o.settled(t, ask) {
		t.Error("an unconfirmed ask our reply in its thread answered still reads open")
	}

	confirmed := o.askOf(t, "Signed contract", o.now.Add(-4*time.Hour))
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Signed contract",
		inReplyTo: o.messageIDOf(t, confirmed), at: o.now.Add(-3 * time.Hour)})
	if o.settled(t, confirmed) == o.owed(t, confirmed) {
		t.Fatal("the scan and the Worklist disagree about the confirmed ask")
	}
	if o.settled(t, confirmed) {
		t.Fatal("a reply alone settled a confirmed ask")
	}
	candidate, ok := o.offered(t, confirmed)
	if !ok {
		t.Fatal("the confirmed ask was never offered for settlement")
	}
	if err := activities.NewStore(o.e.DB()).SettleRequest(o.judge(), activities.RequestSettlementInput{
		Request: candidate, Verdict: activities.RequestSettled, Confidence: 0.95, DecidedBy: "test-model",
	}); err != nil {
		t.Fatalf("settling: %v", err)
	}
	if !o.settled(t, confirmed) || o.owed(t, confirmed) {
		t.Error("a settled verdict did not settle the scan's citation as it did the Worklist's")
	}
}

// Our promise is kept, for the scan, by our later mail on its thread to the
// contact it was made to. Mail to someone else on the thread keeps nothing.
func TestAScanCitationOfOurPromiseSettlesOnOurFollowUp(t *testing.T) {
	o := setupOwed(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	o.contact(t, "Lee Other", "lee@other.example")
	promise := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Listing details",
		body: "I will send the listing details this week.", at: o.now.Add(-6 * time.Hour)})
	o.weWrite(t, owedMail{to: "lee@other.example", subject: "Re: Listing details",
		inReplyTo: o.messageIDOf(t, promise), at: o.now.Add(-5 * time.Hour)})
	if o.settled(t, promise, pat) {
		t.Fatal("mail to someone else on the thread kept the promise")
	}
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Listing details",
		inReplyTo: o.messageIDOf(t, promise), at: o.now.Add(-4 * time.Hour)})
	if !o.settled(t, promise, pat) {
		t.Error("our follow-up to the contact promised did not keep the promise")
	}
}

// One promise mailed to Alice at one account and Bob at another is kept for
// an account only by a follow-up to its own contact.
func TestAPromiseIsKeptOnlyForTheAccountItsFollowUpReached(t *testing.T) {
	o := setupOwed(t)
	alice := o.contact(t, "Alice Ahrens", "alice@a.example")
	bob := o.contact(t, "Bob Berg", "bob@b.example")
	promise := o.weWrite(t, owedMail{to: "alice@a.example", cc: "bob@b.example", subject: "Price sheet",
		body: "I will send both of you the price sheet tomorrow.", at: o.now.Add(-6 * time.Hour)})
	o.weWrite(t, owedMail{to: "bob@b.example", subject: "Re: Price sheet",
		inReplyTo: o.messageIDOf(t, promise), at: o.now.Add(-4 * time.Hour)})
	if o.settled(t, promise, alice) {
		t.Error("a follow-up to Bob kept the promise for Alice's account")
	}
	if !o.settled(t, promise, bob) {
		t.Error("the follow-up to Bob did not keep the promise for Bob's account")
	}
}

// Our reply restricted to someone else settles nothing for a reader outside
// it: they may not know the reply exists. A reply they may read settles.
func TestOnlyAReplyTheReaderMayReadSettlesTheirCitation(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ask := o.customerWrites(t, "pat@customer.example", "Listing details", o.now.Add(-6*time.Hour))
	reply := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Listing details",
		inReplyTo: o.messageIDOf(t, ask), at: o.now.Add(-5 * time.Hour)})
	colleague := o.e.As(o.e.Rep2, nil, integration.AdminPerms)
	if !o.settledAs(t, colleague, ask) {
		t.Fatal("a reply the colleague may read did not settle the ask; the case below proves nothing")
	}
	if _, err := o.e.Activities.SetAudience(o.reader(), ids.From[ids.ActivityKind](reply), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: o.owner}},
	}); err != nil {
		t.Fatalf("restricting the reply: %v", err)
	}
	if o.settledAs(t, colleague, ask) {
		t.Error("a reply restricted away from the reader settled their citation")
	}
	if !o.settled(t, ask) {
		t.Error("the reply's own author no longer reads the ask as settled")
	}
}
