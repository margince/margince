// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// directionReading is one page's answer to "which way does it run".
type directionReading struct {
	direction crmcontracts.RelationshipDirection
	inboundAt *time.Time
}

func (f *relationshipFixture) readings(t *testing.T) (contact, company directionReading, inbound90d int) {
	t.Helper()
	ctx := f.e.Admin()
	page, err := f.contactPages().Assemble(ctx, ids.ContactID{UUID: f.contact})
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.companyPages().Assemble(ctx, ids.CompanyID{UUID: f.company})
	if err != nil {
		t.Fatal(err)
	}
	if page.RelationshipDirection == nil || account.RelationshipDirection == nil || page.Strength == nil || page.Strength.Inbound90d == nil {
		t.Fatalf("a page left out its reading: contact %v, company %v, strength %+v",
			page.RelationshipDirection, account.RelationshipDirection, page.Strength)
	}
	return directionReading{*page.RelationshipDirection, page.LastInboundAt},
		directionReading{*account.RelationshipDirection, account.LastInboundAt}, *page.Strength.Inbound90d
}

// A company with one contact reads what the contact page reads. A colleague's
// reply on a thread the contact is copied on is not the contact writing.
// Activity filed on the account alone moves neither page's direction.
func TestOneContactCompanyReadsItsContactsDirection(t *testing.T) {
	f := newRelationshipFixture(t)
	f.mail(t, "proposal", f.seatAddr, f.contactAddr, "", 9)
	f.mail(t, "proposal-chase", f.seatAddr, f.contactAddr, "", 6)
	f.reply(t, "colleague-answer", "proposal", "Bob Partner <bob@partner.test>", f.seatAddr+", "+f.contactAddr, 5)
	f.logOnCompanyOnly(t, 4)

	contact, company, inbound := f.readings(t)
	if contact.direction != crmcontracts.RelationshipDirectionOutboundOnly || contact.inboundAt != nil || inbound != 0 {
		t.Errorf("a colleague's reply read as the contact writing: direction %q, last inbound %v, inbound_90d %d",
			contact.direction, contact.inboundAt, inbound)
	}
	if company != contact {
		t.Errorf("the company reads %+v and its only contact %+v", company, contact)
	}

	f.reply(t, "anna-answer", "proposal", f.contactAddr, f.seatAddr, 2)
	contact, company, inbound = f.readings(t)
	if contact.direction != crmcontracts.RelationshipDirectionTwoWay || contact.inboundAt == nil || inbound != 1 {
		t.Errorf("the contact's own reply did not count: direction %q, last inbound %v, inbound_90d %d",
			contact.direction, contact.inboundAt, inbound)
	}
	if company.direction != contact.direction || company.inboundAt == nil || !company.inboundAt.Equal(*contact.inboundAt) {
		t.Errorf("the company reads %+v and its only contact %+v", company, contact)
	}
}

// logOnCompanyOnly files an inbound mail to an account alias against the
// account and no contact.
func (f *relationshipFixture) logOnCompanyOnly(t *testing.T, daysAgo int) {
	t.Helper()
	subject, inbound, at := "Mail to the account alias", "inbound", f.now.AddDate(0, 0, -daysAgo)
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "company", EntityID: f.company}},
	}); err != nil {
		t.Fatal(err)
	}
}

// An older inbound mail that lost its sender row is still the contact's when
// its counterparty is their address.
func TestAMailThatLostItsSenderRowStillCountsAsTheContacts(t *testing.T) {
	f := newRelationshipFixture(t)
	older := f.mail(t, "anna-older-capture", f.contactAddr, f.seatAddr, "", 3)
	f.e.WsExec(t, `DELETE FROM activity_participant WHERE activity_id = $1 AND role = 'from'`, older)

	contact, company, inbound := f.readings(t)
	if contact.direction != crmcontracts.RelationshipDirectionInboundOnly || contact.inboundAt == nil || inbound != 1 {
		t.Errorf("the mail without its sender row vanished: direction %q, last inbound %v, inbound_90d %d",
			contact.direction, contact.inboundAt, inbound)
	}
	if company.direction != contact.direction || company.inboundAt == nil {
		t.Errorf("the company reads %+v and its only contact %+v", company, contact)
	}
}

// An employee the reader may not see moves none of the account's clocks for
// that reader.
func TestAHiddenEmployeeMovesNoCompanyClockForTheReader(t *testing.T) {
	f := newRelationshipFixture(t)
	made, err := f.e.Contacts.CreateContact(f.e.Admin(), contacts.CreateContactInput{
		FullName: "Hans Hidden", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "hans@client.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden := ids.UUID(made.Id)
	f.e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
		VALUES ('employment', $1, $2, 'Buyer', 'manual', 'human:x')`, hidden, f.company)
	f.mail(t, "hans-note", "hans@client.test", f.seatAddr, "", 2)
	rep := f.e.As(f.seat, []ids.UUID{f.e.Team1}, integration.AccountRepPerms)
	inboundAt := func() *time.Time {
		t.Helper()
		account, err := f.companyPages().Assemble(rep, ids.CompanyID{UUID: f.company})
		if err != nil {
			t.Fatal(err)
		}
		return account.LastInboundAt
	}
	if inboundAt() == nil {
		t.Fatal("the employee's mail does not move the clock while they are visible; the case proves nothing")
	}
	f.e.MakeCapturePrivate(t, "contact", hidden, f.e.Rep3)
	if got := inboundAt(); got != nil {
		t.Errorf("an employee the reader may not see moved the account's inbound clock to %v", got)
	}
}
