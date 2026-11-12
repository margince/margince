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

// SeedPromiseTask files the task signal extraction writes for the promise. It
// carries the commitment source and the promise mail as its source, and the
// claim on the recipient points at it. The task is linked to the recipient
// and to any other contact named.
func (e *Env) SeedPromiseTask(t *testing.T, p PromiseThread, linked ...ids.ContactID) ids.UUID {
	t.Helper()
	promise := ids.UUID(p.Claim.SourceActivityId)
	due := p.SentAt.AddDate(0, 0, 2)
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: p.Contact.UUID}}
	for _, contact := range linked {
		links = append(links, activities.ActivityLinkInput{EntityType: "contact", EntityID: contact.UUID})
	}
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Send the listing details"), SystemText: &systemtext.Message{Subject: systemtext.Prose(systemtext.Quote, "")}, Source: "manual",
		AssigneeID: new(ids.From[ids.UserKind](e.AdminUser)), DueAt: &due,
		SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: &promise,
		Links: links,
	})
	if err != nil {
		t.Fatalf("filing the promise task: %v", err)
	}
	e.WsExec(t, `UPDATE conversation_claim SET task_activity_id = $1 WHERE id = $2`, ids.UUID(task.Id), ids.UUID(p.Claim.Id))
	return ids.UUID(task.Id)
}

// SeedThreadContact writes one more contact at the promise's account.
func (e *Env) SeedThreadContact(t *testing.T, name, address string) ids.ContactID {
	t.Helper()
	row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(row.Id))
}

// ClaimPromiseFor records the promise as made to one more recipient, on the
// same task.
func (e *Env) ClaimPromiseFor(t *testing.T, p PromiseThread, task ids.UUID, contact ids.ContactID) {
	t.Helper()
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: contact, Kind: "commitment_ours", Body: "Send the listing details", ActivityID: ids.UUID(p.Claim.SourceActivityId),
		Quote: "I'll prepare the listing details and get those over to you shortly.", Source: "manual",
	})
	e.WsExec(t, `UPDATE conversation_claim SET task_activity_id = $1 WHERE id = $2`, task, ids.UUID(claim.Id))
}
