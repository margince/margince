// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting row on the Worklist says who the meeting was with and who hosted
// it, against a real database and through the real writers.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// partiesMeeting logs a past meeting as host, linked to the given contacts;
// the writer records each as a participant.
func partiesMeeting(ctx context.Context, t *testing.T, e *integration.Env, subject string, with ...ids.UUID) ids.UUID {
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

// The colleague is linked first and sorts first by name, so a reading that took
// either would name our own employee as the customer.
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
	review := partiesMeeting(teammate, t, e, "Quarterly review", colleague, customer)
	demo := partiesMeeting(stranger, t, e, "Demo", customer)

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
// colleague's private customer, first by name, is skipped for another reader
// and named for the colleague who may see them.
func TestAMeetingNamesOnlyACustomerTheReaderMaySee(t *testing.T) {
	e := integration.Setup(t)
	hidden := partiesContact(t, e, "Alex Private", e.Rep3, nil)
	visibility := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](hidden),
		contacts.UpdateContactInput{Visibility: &visibility}); err != nil {
		t.Fatalf("making the colleague's buyer private: %v", err)
	}
	visible := partiesContact(t, e, "Blair Shared", ids.UUID{}, nil)
	owner := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AccountRepPerms)
	joint := partiesMeeting(owner, t, e, "Joint call", hidden, visible)

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
		t.Errorf("for the owner the joint call is with %v, want the first attendee by name %v", theirs[joint], hidden)
	}
}

// Where a contact works is told only about a contact the reader may see.
func TestAContactsEmployerIsWithheldWithTheContact(t *testing.T) {
	e := integration.Setup(t)
	acme, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	acmeID := ids.From[ids.CompanyKind](ids.UUID(acme.Id))
	hidden := partiesContact(t, e, "Alex Private", e.Rep3, &acmeID)
	visibility := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](hidden),
		contacts.UpdateContactInput{Visibility: &visibility}); err != nil {
		t.Fatalf("making the colleague's buyer private: %v", err)
	}
	visible := partiesContact(t, e, "Blair Shared", ids.UUID{}, &acmeID)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	got, err := e.Contacts.CurrentEmployers(rep, []ids.UUID{hidden, visible})
	if err != nil {
		t.Fatalf("reading the employers: %v", err)
	}
	if got[visible].CompanyName != "Acme" {
		t.Errorf("the shared buyer's employer = %+v, want Acme", got[visible])
	}
	if employer, told := got[hidden]; told {
		t.Errorf("a contact the reader may not see is said to work at %q", employer.CompanyName)
	}
}

// A reader without the company grant is refused the employer read, and their
// meeting row still names the customer with no account beside it.
func TestAReaderWhoMayNotReadCompaniesSeesTheMeetingRowWithoutAnAccount(t *testing.T) {
	e := integration.Setup(t)
	acme, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	acmeID := ids.From[ids.CompanyKind](ids.UUID(acme.Id))
	customer := partiesContact(t, e, "Customer Buyer", ids.UUID{}, &acmeID)
	meeting := partiesMeeting(e.Admin(), t, e, "Quarterly review", customer)

	noCompany := integration.AdminPerms
	noCompany.Objects = map[string]principal.ObjectGrant{}
	for object, grant := range integration.AdminPerms.Objects {
		if object != "company" {
			noCompany.Objects[object] = grant
		}
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, noCompany)
	if _, err := e.Contacts.CurrentEmployers(reader, []ids.UUID{customer}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the employer read without a company grant = %v, want it refused", err)
	}

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now)
	day, err := feed.Worklist(reader, "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatalf("reading the Worklist without a company grant: %v", err)
	}
	row := outcomeRow(t, day.Queue, meeting)
	if row.Contact == nil || ids.UUID(row.Contact.Id) != customer {
		t.Fatalf("contact = %+v, want the customer %v", row.Contact, customer)
	}
	if row.Contact.Employer != nil {
		t.Errorf("a reader without a company grant is told the customer works at %q", row.Contact.Employer.CompanyName)
	}
}
