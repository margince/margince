// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A meeting row on the Worklist says who the meeting was with and who hosted
// it, against a real database and through the real writers.

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
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

// The own-company test reads only employment edges the reader may see. A
// reader who may not read edges is shown the first outside participant by
// name, our own employee included, so a hidden edge never decides who is named.
func TestAHiddenEmploymentEdgeDoesNotDecideWhoAMeetingWasWith(t *testing.T) {
	e := integration.Setup(t)
	anchor, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Ourselves GmbH"})
	if err != nil {
		t.Fatalf("saving the installation's own company: %v", err)
	}
	colleague := partiesContact(t, e, "Aaron Inhouse", ids.UUID{}, &anchor.CompanyID)
	customer := partiesContact(t, e, "Zed Customer", ids.UUID{}, nil)
	meeting := partiesMeeting(e.Admin(), t, e, "Kickoff", colleague, customer)

	withoutEdges := integration.AdminPerms
	withoutEdges.Objects = maps.Clone(withoutEdges.Objects)
	delete(withoutEdges.Objects, "relationship")
	for _, tc := range []struct {
		name   string
		reader context.Context
		want   ids.UUID
	}{
		{"a reader who may read the employment edge", e.Admin(), customer},
		{"a reader who may not read edges", e.As(e.AdminUser, nil, withoutEdges), colleague},
	} {
		t.Run(tc.name, func(t *testing.T) {
			with, err := e.Activities.MeetingCounterparties(tc.reader, []ids.UUID{meeting})
			if err != nil {
				t.Fatalf("reading who the meeting was with: %v", err)
			}
			if with[meeting] != tc.want {
				t.Errorf("the meeting was with %v, want %v", with[meeting], tc.want)
			}
		})
	}
}

// A reader who may not read activities is told about nobody, and the page
// around them still stands.
func TestAReaderWhoMayNotReadActivitiesIsToldTheMeetingWasWithNobody(t *testing.T) {
	e := integration.Setup(t)
	customer := partiesContact(t, e, "Zed Customer", ids.UUID{}, nil)
	meeting := partiesMeeting(e.Admin(), t, e, "Kickoff", customer)

	withoutActivities := integration.AdminPerms
	withoutActivities.Objects = maps.Clone(withoutActivities.Objects)
	delete(withoutActivities.Objects, "activity")
	with, err := e.Activities.MeetingCounterparties(e.As(e.AdminUser, nil, withoutActivities), []ids.UUID{meeting})
	if err != nil || len(with) != 0 {
		t.Fatalf("a reader without the activity grant got %v, %v; want nobody and no error", with, err)
	}
}

// colleagueTestDomain is the domain the harness gives every seat's login.
const colleagueTestDomain = "authz.test"

// bookedMeeting writes a booked meeting an hour after journeyMorning as
// calendar capture stores it: Rep1 on it by its login address, then one
// participant row per guest address, resolved to the contact given beside it.
func bookedMeeting(t *testing.T, e *integration.Env, subject string, guests map[string]ids.UUID) ids.UUID {
	t.Helper()
	seat, seatAddress := e.Rep1, "a@"+colleagueTestDomain
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `INSERT INTO activity (kind, subject, meeting_status, occurred_at, source, captured_by)
			VALUES ('meeting', $1, 'booked', $2, 'gcal:seed', $3) RETURNING id`,
			subject, journeyMorning.Add(time.Hour), "connector:gcal:"+seat.String()).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, address, address_domain_folded, role)
			VALUES ($1, $2, $3, $4, 'attendee')`, id, seat, seatAddress, owndomain.StoredOfAddress(seatAddress)); err != nil {
			return err
		}
		for address, contact := range guests {
			var contactID *ids.UUID
			if !contact.IsZero() {
				contactID = &contact
			}
			if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, contact_id, address, address_domain_folded, role)
				VALUES ($1, $2, $3, $4, 'attendee')`, id, contactID, address, owndomain.StoredOfAddress(address)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("writing the meeting %q: %v", subject, err)
	}
	return id
}

// addressedContact creates an unowned contact with one address and no employer.
func addressedContact(t *testing.T, e *integration.Env, name, address string) ids.UUID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	return ids.UUID(contact.Id)
}

// trustOwnDomain makes the seats' login domain a confirmed own domain.
func trustOwnDomain(t *testing.T, e *integration.Env) {
	t.Helper()
	e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ($1, 'admin', true)`, colleagueTestDomain)
}

// The issue's reproduction (#9994): a colleague's contact on the own domain,
// with no employment edge to the own company and no seat, is never named as
// the meeting's contact, and the meeting is not on the lane.
func TestAColleaguesContactOnTheOwnDomainIsNeverAMeetingsContact(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	colleague := addressedContact(t, e, "Colleague Bea", "bea@"+colleagueTestDomain)
	meeting := bookedMeeting(t, e, "One to one",
		map[string]ids.UUID{"bea@" + colleagueTestDomain: colleague})

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	with, err := e.Activities.MeetingCounterparties(rep, []ids.UUID{meeting})
	if err != nil {
		t.Fatalf("reading who the meeting was with: %v", err)
	}
	if named, ok := with[meeting]; ok {
		t.Errorf("the meeting among colleagues names %v as its contact", named)
	}
	if row, at := meetingRow(readJourneyDay(rep, t, e), meeting); at >= 0 {
		t.Errorf("the meeting among colleagues is on the lane at %d with contact %+v", at, row.Contact)
	}
}

// A meeting whose only other participant is a seat's colleague address leaves
// the lane; the same meeting with a customer on it stays and names them.
func TestAMeetingAmongColleaguesLeavesKeepMomentum(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	customer := addressedContact(t, e, "Ines Buyer", "ines@customer.test")
	internal := bookedMeeting(t, e, "Team sync",
		map[string]ids.UUID{"dev@" + colleagueTestDomain: {}})
	external := bookedMeeting(t, e, "Pricing review",
		map[string]ids.UUID{"dev@" + colleagueTestDomain: {}, "ines@customer.test": customer})

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	day := readJourneyDay(rep, t, e)
	if _, at := meetingRow(day, internal); at >= 0 {
		t.Errorf("the meeting among colleagues is on the lane at %d", at)
	}
	row, at := meetingRow(day, external)
	if at < 0 {
		t.Fatal("the customer meeting left the lane")
	}
	if row.Contact == nil || ids.UUID(row.Contact.Id) != customer {
		t.Errorf("the customer meeting names %+v, want the customer %v", row.Contact, customer)
	}
}

// A meeting whose only outside guest the reader may not see stays on the lane,
// naming nobody: a row-scope rule never makes a customer meeting vanish.
func TestAMeetingWhoseOnlyGuestIsHiddenStaysOnTheLane(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	hidden := addressedContact(t, e, "Alex Private", "alex@customer.test")
	owner := ids.From[ids.UserKind](e.Rep3)
	visibility := "owner"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](hidden),
		contacts.UpdateContactInput{OwnerID: &owner, Visibility: &visibility}); err != nil {
		t.Fatalf("making the buyer private to another seat: %v", err)
	}
	meeting := bookedMeeting(t, e, "Discovery",
		map[string]ids.UUID{"alex@customer.test": hidden})

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	row, at := meetingRow(readJourneyDay(rep, t, e), meeting)
	if at < 0 {
		t.Fatal("a customer meeting whose guest the reader may not see left the lane")
	}
	if row.Contact != nil {
		t.Errorf("the meeting names %+v, a contact the reader may not see", row.Contact)
	}
}

// A seat's login on a domain nobody trusts is still a colleague: it is not
// named, and a meeting with nobody else on it leaves the lane.
func TestAMemberLoginOffTheOwnDomainIsAColleague(t *testing.T) {
	e := integration.Setup(t)
	login := "b@" + colleagueTestDomain
	record := addressedContact(t, e, "Rep Two", login)
	meeting := bookedMeeting(t, e, "Pairing", map[string]ids.UUID{login: record})

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	with, err := e.Activities.MeetingCounterparties(rep, []ids.UUID{meeting})
	if err != nil {
		t.Fatalf("reading who the meeting was with: %v", err)
	}
	if named, ok := with[meeting]; ok {
		t.Errorf("a member's login names %v as the meeting's contact", named)
	}
	if _, at := meetingRow(readJourneyDay(rep, t, e), meeting); at >= 0 {
		t.Errorf("the meeting with a member's login is on the lane at %d", at)
	}
}

// Deal Scout's held-meeting walk reaches no company through a colleague on
// the own domain, whatever employer their contact record names.
func TestHeldMeetingAmongColleaguesReachesNoCompany(t *testing.T) {
	e := integration.Setup(t)
	trustOwnDomain(t, e)
	acme, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	acmeID := ids.From[ids.CompanyKind](ids.UUID(acme.Id))
	colleague := addressedContact(t, e, "Colleague Bea", "bea@"+colleagueTestDomain)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: ptrContact(colleague), CompanyID: &acmeID, Source: "manual",
	}); err != nil {
		t.Fatalf("employing the colleague at Acme: %v", err)
	}
	customer := partiesContact(t, e, "Ines Buyer", ids.UUID{}, &acmeID)
	internal := partiesMeeting(e.Admin(), t, e, "Team sync", colleague)
	external := partiesMeeting(e.Admin(), t, e, "Review", customer)

	reached := func(meeting ids.UUID) int {
		return e.WsCount(t, `SELECT count(*) FROM (`+activities.HeldMeetingCounterparties()+`) mc WHERE mc.activity_id = $1`, meeting)
	}
	if n := reached(internal); n != 0 {
		t.Errorf("the meeting with a colleague reaches %d companies, want none", n)
	}
	if n := reached(external); n != 1 {
		t.Errorf("the meeting with a customer reaches %d companies, want Acme", n)
	}
}

func ptrContact(id ids.UUID) *ids.ContactID {
	contact := ids.From[ids.ContactKind](id)
	return &contact
}
