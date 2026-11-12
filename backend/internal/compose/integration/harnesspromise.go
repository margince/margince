// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PromiseThread is one promise we made by mail, on its own thread.
type PromiseThread struct {
	Contact ids.ContactID
	Address string
	Claim   crmcontracts.ConversationClaim
	SentAt  time.Time
}

// SeedPromiseThread writes a contact, our mail promising them something, and
// the claim read out of it.
func (e *Env) SeedPromiseThread(t *testing.T) PromiseThread {
	t.Helper()
	const address = "lena@kunde.example"
	row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Lena Kunde", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(row.Id))
	sentAt := time.Date(2026, 9, 7, 9, 0, 0, 0, time.UTC)
	promise := e.LogThreadMail(e.Admin(), t, contact, "thread-listing", address, "outbound", sentAt)
	quote := "I'll prepare the listing details and get those over to you shortly."
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: contact, Kind: "commitment_ours", Body: "Send the listing details", ActivityID: promise, Quote: quote, Source: "manual",
	})
	return PromiseThread{Contact: contact, Address: address, Claim: claim, SentAt: sentAt}
}

// LogThreadMail writes one email on a thread through the activity writer.
// Outbound mail is attested as sent by us.
func (e *Env) LogThreadMail(as context.Context, t *testing.T, contact ids.ContactID, thread, counterparty, direction string, at time.Time) ids.UUID {
	t.Helper()
	subject := "Re: Listing details"
	from, to := "rep@margince.example", counterparty
	if direction == "inbound" {
		from, to = counterparty, "rep@margince.example"
	}
	row, _, err := e.Activities.LogActivity(as, activities.LogActivityInput{
		Kind: "email", Subject: &subject, SystemText: &systemtext.Message{Subject: systemtext.Prose(systemtext.Quote, "")}, Direction: &direction, OccurredAt: &at, Source: "manual",
		ThreadKey: thread, CounterpartyEmail: counterparty, EmailFrom: from, EmailTo: []string{to}, CounterpartyOutboundAttested: direction == "outbound",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}},
	})
	if err != nil {
		t.Fatalf("logging a %s email on %s: %v", direction, thread, err)
	}
	return ids.UUID(row.Id)
}
