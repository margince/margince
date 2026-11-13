// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A booking through a host's public page files the booker as a mail would. The
// host owns them, and their mail domain's company employs them. The source says
// they came through a booking. An existing contact keeps what someone set.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asPublicBooking is the principal the public booking door runs under.
func (e *dedupeEnv) asPublicBooking() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:public_booking"})
}

// book runs the door's steps: the ensure as the public booking principal, then,
// for an existing contact only, the fill with the host's own authority. The host
// is always otherRep, whose authority asOther carries.
func (e *dedupeEnv) book(t *testing.T, name, email string) ids.ContactID {
	t.Helper()
	host := ids.From[ids.UserKind](e.otherRep)
	contact, lead, created, err := e.store.EnsureBookerByEmail(e.asPublicBooking(), host, name, email, "public_booking", storekit.AdmitSelfBooking)
	if err != nil || !lead.IsZero() {
		t.Fatalf("booking as %s: contact %s, lead %s, err %v; want a contact", email, contact, lead, err)
	}
	if !created {
		if err := e.store.FillBookerGaps(e.asOther(), contact, host, "public_booking"); err != nil {
			t.Fatalf("filling the booker's gaps: %v", err)
		}
	}
	return ids.From[ids.ContactKind](contact)
}

// bookerFacts are the owner, the employers and the acquisition kinds of one contact.
type bookerFacts struct {
	owner     *ids.UUID
	employers []ids.UUID
	kinds     []string
}

func (e *dedupeEnv) bookerFacts(t *testing.T, id ids.ContactID) bookerFacts {
	t.Helper()
	ctx := e.asPublicBooking()
	var out bookerFacts
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT owner_id FROM contact WHERE id = $1`, id).Scan(&out.owner); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(array_agg(company_id ORDER BY company_id), '{}') FROM relationship
			 WHERE kind = 'employment' AND contact_id = $1 AND archived_at IS NULL`, id).Scan(&out.employers); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT coalesce(array_agg(kind ORDER BY kind), '{}') FROM contact_acquisition_evidence
			 WHERE contact_id = $1`, id).Scan(&out.kinds)
	}); err != nil {
		t.Fatalf("reading the booker's owner, employer and source: %v", err)
	}
	return out
}

func (f bookerFacts) ownedBy(user ids.UUID) bool { return f.owner != nil && *f.owner == user }

func (f bookerFacts) employedOnlyAt(company ids.CompanyID) bool {
	return len(f.employers) == 1 && f.employers[0] == company.UUID
}

func TestANewBookerIsTheHostsAtTheDomainsCompanyAndCameThroughABooking(t *testing.T) {
	e := setupDedupe(t)
	company := e.createNamedCompany(e.as(), t, "Booked Works", "booked-works.test")

	booker := e.book(t, "Contact A", "contact.a@booked-works.test")

	got := e.bookerFacts(t, booker)
	if !got.ownedBy(e.otherRep) {
		t.Errorf("owner = %v, want the host %s", got.owner, e.otherRep)
	}
	if !got.employedOnlyAt(company) {
		t.Errorf("employers = %v, want only the company on the booker's domain, %s", got.employers, company)
	}
	if len(got.kinds) != 1 || got.kinds[0] != AcquiredEventOrForm {
		t.Errorf("acquisition kinds = %v, want only %s", got.kinds, AcquiredEventOrForm)
	}

	// No company holds this domain, so the booking asks about it once.
	unclaimed := e.book(t, "Contact G", "contact.g@unclaimed-works.test")
	if questions, readers := e.domainQuestionRows(t, "unclaimed-works.test"); questions != 1 || readers != 1 {
		t.Errorf("the unclaimed domain left %d questions and %d readers, want one of each", questions, readers)
	}
	if got := e.bookerFacts(t, unclaimed); len(got.employers) != 0 {
		t.Errorf("employers = %v on an unclaimed domain, want none until the question is answered", got.employers)
	}
}

// domainQuestionRows counts the rows the domain-question writer leaves for one domain.
func (e *dedupeEnv) domainQuestionRows(t *testing.T, domain string) (questions, readers int) {
	t.Helper()
	ctx := e.asPublicBooking()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM company_domain_disposition WHERE domain = $1),
			       (SELECT count(*) FROM company_domain_question_reader WHERE domain = $1)`, domain).Scan(&questions, &readers)
	}); err != nil {
		t.Fatalf("counting the questions about %s: %v", domain, err)
	}
	return questions, readers
}

func TestABookingGivesAContactWithNoOwnerOrCompanyTheHostAndTheDomainsCompany(t *testing.T) {
	e := setupDedupe(t)
	// Made before any company holds the domain, by a door that names no owner,
	// so the contact starts with neither.
	existing := e.contactWithEmail(e.asPublicBooking(), t, "Contact B", "contact.b@later-works.test")
	company := e.createNamedCompany(e.as(), t, "Later Works", "later-works.test")
	before := e.bookerFacts(t, existing)
	if before.owner != nil || len(before.employers) != 0 {
		t.Fatalf("precondition: owner %v, employers %v; want neither", before.owner, before.employers)
	}

	if booked := e.book(t, "Contact B", "contact.b@later-works.test"); booked != existing {
		t.Fatalf("the booking landed on %s, want the existing contact %s", booked, existing)
	}

	got := e.bookerFacts(t, existing)
	if !got.ownedBy(e.otherRep) {
		t.Errorf("owner = %v, want the host %s", got.owner, e.otherRep)
	}
	if !got.employedOnlyAt(company) {
		t.Errorf("employers = %v, want only the company on the booker's domain, %s", got.employers, company)
	}
	if len(got.kinds) != len(before.kinds) || (len(got.kinds) > 0 && got.kinds[0] != before.kinds[0]) {
		t.Errorf("acquisition kinds went from %v to %v; a booking never changes an existing source", before.kinds, got.kinds)
	}
}

func TestABookingKeepsTheOwnerAndCompanyAColleagueAlreadySet(t *testing.T) {
	e := setupDedupe(t)
	chosen := e.createNamedCompany(e.as(), t, "Chosen Employer")
	created, err := e.store.CreateContact(e.as(), CreateContactInput{
		FullName: "Contact C", Source: "manual",
		Emails:   []ContactEmailInput{{Email: "contact.c@domain-works.test", EmailType: "work", IsPrimary: true}},
		Employer: EmployerChoice{CompanyID: &chosen},
	})
	if err != nil {
		t.Fatal(err)
	}
	existing := ids.From[ids.ContactKind](ids.UUID(created.Id))
	e.createNamedCompany(e.as(), t, "Domain Works", "domain-works.test")
	before := e.bookerFacts(t, existing)
	if !before.ownedBy(e.rep) || !before.employedOnlyAt(chosen) {
		t.Fatalf("precondition: owner %v, employers %v; want %s at %s", before.owner, before.employers, e.rep, chosen)
	}

	e.book(t, "Contact C", "contact.c@domain-works.test")

	got := e.bookerFacts(t, existing)
	if !got.ownedBy(e.rep) {
		t.Errorf("owner = %v, want the colleague who set it, %s", got.owner, e.rep)
	}
	if !got.employedOnlyAt(chosen) {
		t.Errorf("employers = %v, want only the company the colleague chose, %s", got.employers, chosen)
	}
}

func TestABookingGivesTheHostsOwnContactTheDomainsCompany(t *testing.T) {
	e := setupDedupe(t)
	existing := e.contactWithEmail(e.asOther(), t, "Contact D", "contact.d@owned-works.test")
	company := e.createNamedCompany(e.as(), t, "Owned Works", "owned-works.test")
	if before := e.bookerFacts(t, existing); !before.ownedBy(e.otherRep) || len(before.employers) != 0 {
		t.Fatalf("precondition: owner %v, employers %v; want the host and no employer", before.owner, before.employers)
	}

	e.book(t, "Contact D", "contact.d@owned-works.test")

	got := e.bookerFacts(t, existing)
	if !got.ownedBy(e.otherRep) {
		t.Errorf("owner = %v, want the host it already had, %s", got.owner, e.otherRep)
	}
	if !got.employedOnlyAt(company) {
		t.Errorf("employers = %v, want only the company on the booker's domain, %s", got.employers, company)
	}
}

func TestABookingGivesAnOwnerlessContactTheHostAndKeepsItsCompany(t *testing.T) {
	e := setupDedupe(t)
	chosen := e.createNamedCompany(e.as(), t, "Picked Employer")
	created, err := e.store.CreateContact(e.asPublicBooking(), CreateContactInput{
		FullName: "Contact E", Source: "manual",
		Emails:   []ContactEmailInput{{Email: "contact.e@unpicked-works.test", EmailType: "work", IsPrimary: true}},
		Employer: EmployerChoice{CompanyID: &chosen},
	})
	if err != nil {
		t.Fatal(err)
	}
	existing := ids.From[ids.ContactKind](ids.UUID(created.Id))
	e.createNamedCompany(e.as(), t, "Unpicked Works", "unpicked-works.test")
	if before := e.bookerFacts(t, existing); before.owner != nil || !before.employedOnlyAt(chosen) {
		t.Fatalf("precondition: owner %v, employers %v; want no owner, employed at %s", before.owner, before.employers, chosen)
	}

	e.book(t, "Contact E", "contact.e@unpicked-works.test")

	got := e.bookerFacts(t, existing)
	if !got.ownedBy(e.otherRep) {
		t.Errorf("owner = %v, want the host %s", got.owner, e.otherRep)
	}
	if !got.employedOnlyAt(chosen) {
		t.Errorf("employers = %v, want only the company already set, %s", got.employers, chosen)
	}
}

func TestABookingFromALiveLeadsAddressMakesNoContactAndLeavesTheLead(t *testing.T) {
	e := setupDedupe(t)
	const address = "lead.f@lead-works.test"
	held := e.createLead(e.as(), t, "Lead F", address, "Lead Works")
	e.createNamedCompany(e.as(), t, "Lead Works", "lead-works.test")
	before := e.leadOwner(t, held)

	contact, lead, created, err := e.store.EnsureBookerByEmail(e.asPublicBooking(), ids.From[ids.UserKind](e.otherRep), "Lead F", address, "public_booking", storekit.AdmitSelfBooking)
	if err != nil || lead != held || contact != ids.Nil || created {
		t.Fatalf("booking as the lead's address: contact %s, lead %s, err %v; want only the lead %s", contact, lead, err, held)
	}
	ctx := e.asPublicBooking()
	var contacts int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM contact_email WHERE email = $1`, address).Scan(&contacts)
	}); err != nil {
		t.Fatal(err)
	}
	if contacts != 0 {
		t.Errorf("%d contacts hold the lead's address, want none", contacts)
	}
	if after := e.leadOwner(t, held); after != before {
		t.Errorf("the lead's owner went from %v to %v; a booking never changes it", before, after)
	}
}

func (e *dedupeEnv) leadOwner(t *testing.T, lead ids.LeadID) ids.UUID {
	t.Helper()
	ctx := e.asPublicBooking()
	var owner ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(owner_id, '00000000-0000-0000-0000-000000000000') FROM lead WHERE id = $1`, lead).Scan(&owner)
	}); err != nil {
		t.Fatalf("reading the lead's owner: %v", err)
	}
	return owner
}
