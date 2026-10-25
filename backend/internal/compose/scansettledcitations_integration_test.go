// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// settled is what the company scan reads about one cited message.
func (o *owedEnv) settled(t *testing.T, message ids.UUID) bool {
	t.Helper()
	var states map[ids.UUID]bool
	if err := database.WithWorkspaceTx(o.reader(), o.e.Pool, func(tx pgx.Tx) error {
		var err error
		states, err = activities.SettledCitations(o.reader(), tx, []ids.UUID{message}, o.now)
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
// person it was made to. Mail to someone else on the thread keeps nothing.
func TestAScanCitationOfOurPromiseSettlesOnOurFollowUp(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	o.contact(t, "Lee Other", "lee@other.example")
	promise := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Listing details",
		body: "I will send the listing details this week.", at: o.now.Add(-6 * time.Hour)})
	o.weWrite(t, owedMail{to: "lee@other.example", subject: "Re: Listing details",
		inReplyTo: o.messageIDOf(t, promise), at: o.now.Add(-5 * time.Hour)})
	if o.settled(t, promise) {
		t.Fatal("mail to someone else on the thread kept the promise")
	}
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Listing details",
		inReplyTo: o.messageIDOf(t, promise), at: o.now.Add(-4 * time.Hour)})
	if !o.settled(t, promise) {
		t.Error("our follow-up to the person promised did not keep the promise")
	}
}
