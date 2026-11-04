// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The approval card marks an address with no visible record the same way
// whether or not a record hidden from the host holds it.
func TestTheInvitationCardMarksHiddenAndUnknownAddressesAlike(t *testing.T) {
	appointment := connector.CalendarAppointment{Roster: []connector.CalendarAttendee{
		{Address: "buyer@buyer.test", Basis: connector.AttendeeContact, AskConsent: true},
		{Address: "hidden@buyer.test", Basis: connector.AttendeeHostNamed, AskConsent: true},
		{Address: "stranger@elsewhere.test", Basis: connector.AttendeeHostNamed},
	}}
	fields := map[string]string{}
	for _, detail := range invitationRecipients(crmcontracts.MeetingInvitationRequest{}, appointment) {
		fields[detail.Value.Text] = detail.Field
	}
	if fields["hidden@buyer.test"] != previewHostNamedAttendee || fields["stranger@elsewhere.test"] != previewHostNamedAttendee {
		t.Errorf("hidden and unknown addresses read %q and %q, want both %q", fields["hidden@buyer.test"], fields["stranger@elsewhere.test"], previewHostNamedAttendee)
	}
	if fields["buyer@buyer.test"] != "to" {
		t.Errorf("the contact reads %q, want to", fields["buyer@buyer.test"])
	}
}
