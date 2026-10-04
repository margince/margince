// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A meeting row says who the meeting was with and who hosted it, and names
// neither where the reader may not see them.

import (
	"context"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var partiesInstant = time.Date(2026, 9, 22, 15, 0, 0, 0, time.UTC)

// employersSaying answers a fixed set of employers, and records what it was
// asked for.
type employersSaying struct {
	known map[ids.UUID]crmcontracts.ContactEmployer
	asked *[]ids.UUID
}

func (e employersSaying) CurrentEmployers(_ context.Context, contactIDs []ids.UUID) (map[ids.UUID]crmcontracts.ContactEmployer, error) {
	*e.asked = append(*e.asked, contactIDs...)
	out := make(map[ids.UUID]crmcontracts.ContactEmployer, len(contactIDs))
	for _, id := range contactIDs {
		if employer, ok := e.known[id]; ok {
			out[id] = employer
		}
	}
	return out, nil
}

func TestAnUnansweredMeetingNamesItsCustomerAndItsHost(t *testing.T) {
	t.Parallel()
	customer, host := ids.NewV7(), ids.NewV7()
	row := classifyUnansweredMeeting(meetingAwaitingOutcomeItem(MeetingAwaitingOutcome{
		ID: ids.NewV7(), Subject: "Quarterly review", StartedAt: partiesInstant.Add(-time.Hour),
		HostUserID: host, ContactID: customer,
	}), partiesInstant).item
	if row.Contact == nil || ids.UUID(row.Contact.Id) != customer {
		t.Errorf("contact = %+v, want the customer %v the meeting was with", row.Contact, customer)
	}
	if row.Host == nil || row.Host.Id == nil || ids.UUID(*row.Host.Id) != host {
		t.Fatalf("host = %+v, want the seat %v whose calendar it came off", row.Host, host)
	}
	if row.Host.Kind != crmcontracts.WorklistOwnerKindWorklistOwnerUser {
		t.Errorf("host kind = %q, want user", row.Host.Kind)
	}
}

// An internal meeting, or one whose outside attendees are withheld, names
// nobody, and a meeting booked in the app names no host rather than guessing.
func TestAMeetingWithNobodyOutsideNamesNoCustomerAndNoHost(t *testing.T) {
	t.Parallel()
	row := classifyUnansweredMeeting(meetingAwaitingOutcomeItem(MeetingAwaitingOutcome{
		ID: ids.NewV7(), Subject: "Stand-up", StartedAt: partiesInstant.Add(-time.Hour),
	}), partiesInstant).item
	if row.Contact != nil {
		t.Errorf("contact = %+v, want none for a meeting with nobody outside", row.Contact)
	}
	if row.Host != nil {
		t.Errorf("host = %+v, want none for a meeting no calendar claims", row.Host)
	}
}

// The meeting still ahead names its host the same way, so one appointment says
// the same thing on either side of its start.
func TestAMeetingAheadNamesItsHost(t *testing.T) {
	t.Parallel()
	host := ids.NewV7()
	row := classifyMeeting(meetingItem(Meeting{
		ID: ids.NewV7(), Subject: "Demo", StartsAt: partiesInstant.Add(time.Hour), HostUserID: host,
	}), partiesInstant).item
	if row.Host == nil || row.Host.Id == nil || ids.UUID(*row.Host.Id) != host {
		t.Errorf("host = %+v, want the seat %v", row.Host, host)
	}
}

// The host is named by the owner's rule: a colleague on the reader's roster by
// name, anybody else withheld, with the id still saying who it is.
func TestTheHostIsNamedOnlyWhereTheReaderMayResolveTheName(t *testing.T) {
	t.Parallel()
	colleague, stranger := ids.NewV7(), ids.NewV7()
	svc := &Service{teammates: roster{{UserID: colleague, DisplayName: "Lena Fischer"}}}
	rows := []crmcontracts.WorklistItem{
		{Source: sourceMeetingOutcome, Host: &crmcontracts.WorklistOwner{Kind: "user", Id: idPtr(colleague)}},
		{Source: sourceMeetingOutcome, Host: &crmcontracts.WorklistOwner{Kind: "user", Id: idPtr(stranger)}},
	}
	if err := svc.nameTheOwners(context.Background(), rows); err != nil {
		t.Fatalf("naming the owners: %v", err)
	}
	if rows[0].Host.Label == nil || *rows[0].Host.Label != "Lena Fischer" {
		t.Errorf("a colleague's host label = %v, want Lena Fischer", rows[0].Host.Label)
	}
	if rows[1].Host.Label != nil {
		t.Errorf("a host off the reader's roster is named %q, want the name withheld", *rows[1].Host.Label)
	}
}

// The counterparty's employer rides on meeting rows only: every other row's
// title already names its record. A contact whose employer the reader may not
// see gets none.
func TestOnlyAMeetingRowSaysWhereItsContactWorks(t *testing.T) {
	t.Parallel()
	known, withheld, sender := ids.NewV7(), ids.NewV7(), ids.NewV7()
	acme := crmcontracts.ContactEmployer{CompanyId: openapi_types.UUID(ids.NewV7()), CompanyName: "Acme"}
	var asked []ids.UUID
	svc := &Service{employers: employersSaying{
		known: map[ids.UUID]crmcontracts.ContactEmployer{known: acme, sender: acme},
		asked: &asked,
	}}
	rows := []crmcontracts.WorklistItem{
		{Source: sourceMeeting, Contact: &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(known)}},
		{Source: sourceMeetingOutcome, Contact: &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(withheld)}},
		{Source: "waiting", Contact: &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(sender)}},
	}
	if err := svc.nameTheEmployers(context.Background(), rows); err != nil {
		t.Fatalf("naming the employers: %v", err)
	}
	if rows[0].Contact.Employer == nil || rows[0].Contact.Employer.CompanyName != "Acme" {
		t.Errorf("meeting row employer = %+v, want Acme", rows[0].Contact.Employer)
	}
	if rows[1].Contact.Employer != nil {
		t.Errorf("a withheld employer is named %+v, want none", rows[1].Contact.Employer)
	}
	if rows[2].Contact.Employer != nil {
		t.Errorf("a waiting row carries employer %+v, want none", rows[2].Contact.Employer)
	}
	if !slices.Contains(asked, known) || !slices.Contains(asked, withheld) {
		t.Errorf("the employer read was asked about %v, want both meeting contacts", asked)
	}
	if slices.Contains(asked, sender) {
		t.Error("the employer read was asked about a row that is not a meeting")
	}
}
