// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting row on the Worklist says who the meeting was with and who hosted
// it, against a real database and through the real writers.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// partiesContact creates a contact owned by owner (unowned when zero), employed
// at employer when one is given.
func partiesContact(t *testing.T, e *integration.Env, name string, owner ids.UUID, employer *ids.CompanyID) ids.UUID {
	t.Helper()
	in := contacts.CreateContactInput{FullName: name}
	if !owner.IsZero() {
		seat := ids.From[ids.UserKind](owner)
		in.OwnerID = &seat
	}
	contact, err := e.Contacts.CreateContact(e.Admin(), in)
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if employer != nil {
		if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &id, CompanyID: employer, Source: "manual",
		}); err != nil {
			t.Fatalf("employing %q: %v", name, err)
		}
	}
	return ids.UUID(contact.Id)
}

// partiesMeeting logs a past meeting as host, linked to the given contacts in
// order; the writer records each as a participant in that order.
func partiesMeeting(t *testing.T, ctx context.Context, e *integration.Env, subject string, with ...ids.UUID) ids.UUID {
	t.Helper()
	at := time.Now().Add(-2 * time.Hour)
	in := activities.LogActivityInput{Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual"}
	for _, contact := range with {
		in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "contact", EntityID: contact})
	}
	meeting, _, err := e.Activities.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	return ids.UUID(meeting.Id)
}

func outcomeRow(t *testing.T, queue []crmcontracts.WorklistItem, meeting ids.UUID) crmcontracts.WorklistItem {
	t.Helper()
	for _, row := range queue {
		if row.Source == crmcontracts.WorklistItemSourceMeetingOutcome && row.Id == meeting.String() {
			return row
		}
	}
	t.Fatalf("meeting %v is not on the Worklist", meeting)
	return crmcontracts.WorklistItem{}
}

// The colleague is linked FIRST, so a reading that took the first link would
// name our own employee as the customer.
func TestAMeetingOwedAnAnswerNamesItsCustomerTheirAccountAndItsHost(t *testing.T) {
	e := integration.Setup(t)
	anchor, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Ourselves GmbH"})
	if err != nil {
		t.Fatalf("saving the installation's own company: %v", err)
	}
	acme, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("creating the customer's company: %v", err)
	}
	acmeID := ids.From[ids.CompanyKind](ids.UUID(acme.Id))
	colleague := partiesContact(t, e, "Colleague Record", ids.UUID{}, &anchor.CompanyID)
	customer := partiesContact(t, e, "Customer Buyer", ids.UUID{}, &acmeID)

	teammate := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	stranger := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms)
	review := partiesMeeting(t, teammate, e, "Quarterly review", colleague, customer)
	demo := partiesMeeting(t, stranger, e, "Demo", customer)

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now)
	day, err := feed.Worklist(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}

	row := outcomeRow(t, day.Queue, review)
	if row.Contact == nil || ids.UUID(row.Contact.Id) != customer {
		t.Fatalf("contact = %+v, want the customer %v and not the colleague linked first", row.Contact, customer)
	}
	if row.Contact.Label == nil || *row.Contact.Label != "Customer Buyer" {
		t.Errorf("contact label = %v, want Customer Buyer", row.Contact.Label)
	}
	if row.Contact.Employer == nil || row.Contact.Employer.CompanyName != "Acme" {
		t.Errorf("employer = %+v, want Acme", row.Contact.Employer)
	}
	if row.Host == nil || row.Host.Id == nil || ids.UUID(*row.Host.Id) != e.Rep2 {
		t.Fatalf("host = %+v, want the teammate %v who logged it", row.Host, e.Rep2)
	}
	if row.Host.Label == nil || *row.Host.Label == "" {
		t.Error("a teammate's host name is withheld from a reader on the same team")
	}

	// Rep3 sits on another team, so the reader may not resolve their name.
	other := outcomeRow(t, day.Queue, demo)
	if other.Host == nil || other.Host.Id == nil || ids.UUID(*other.Host.Id) != e.Rep3 {
		t.Fatalf("host = %+v, want %v", other.Host, e.Rep3)
	}
	if other.Host.Label != nil {
		t.Errorf("a host off the reader's roster is named %q", *other.Host.Label)
	}
}

// The counterparty is the first outside participant THIS reader may see. A
// colleague's private customer, recorded first, is skipped for another reader
// and named for the colleague who may see them.
func TestAMeetingNamesOnlyACustomerTheReaderMaySee(t *testing.T) {
	e := integration.Setup(t)
	hidden := partiesContact(t, e, "Someone Else's Buyer", e.Rep3, nil)
	visibility := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](hidden),
		contacts.UpdateContactInput{Visibility: &visibility}); err != nil {
		t.Fatalf("making the colleague's buyer private: %v", err)
	}
	visible := partiesContact(t, e, "Shared Buyer", ids.UUID{}, nil)
	owner := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AccountRepPerms)
	joint := partiesMeeting(t, owner, e, "Joint call", hidden, visible)

	store := activities.NewStore(InstallationDB(e.Pool))
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	got, err := store.MeetingCounterparties(rep, []ids.UUID{joint})
	if err != nil {
		t.Fatalf("reading who the meeting was with: %v", err)
	}
	if got[joint] != visible {
		t.Errorf("the joint call is with %v, want the buyer this reader may see %v", got[joint], visible)
	}

	theirs, err := store.MeetingCounterparties(owner, []ids.UUID{joint})
	if err != nil {
		t.Fatalf("reading who the meeting was with, as its owner: %v", err)
	}
	if theirs[joint] != hidden {
		t.Errorf("for the owner the joint call is with %v, want the first attendee %v", theirs[joint], hidden)
	}
}
