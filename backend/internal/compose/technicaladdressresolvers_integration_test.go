// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A contact stored with a technical address before the rule existed is not
// found by the resolvers that look a contact up by address. The CSV deal
// stakeholder link and the meeting attendee roster each treat the address as
// naming no contact; a human address beside it still resolves.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestACSVLinkToATechnicalAddressNamesNoContact(t *testing.T) {
	e := integration.Setup(t)
	conn := integration.OwnerConn(t)
	machineAddressContact(t, e, conn, "HubSpot Logger", hubspotBCC)
	human := machineAddressContact(t, e, conn, "Anna at HubSpot", "anna@hubspot.com")
	r := newLinkResolver(e.DB(), "csv:test")

	technical, err := r.visibleContactByEmail(e.Admin(), hubspotBCC)
	if err != nil {
		t.Fatal(err)
	}
	if technical.Found {
		t.Errorf("a CSV link to %s found contact %v, want none", hubspotBCC, technical.ID)
	}
	found, err := r.visibleContactByEmail(e.Admin(), "anna@hubspot.com")
	if err != nil {
		t.Fatal(err)
	}
	if !found.Found || found.ID != human {
		t.Errorf("a CSV link to a human found %+v, want %v", found, human)
	}
}

func TestAMeetingAttendeeAtATechnicalAddressIsNoContact(t *testing.T) {
	w := newAttendeeWorld(t)
	technical := w.contact(t, w.e.Rep1, hubspotBCC, "")
	got, err := w.calendar.ResolveAttendees(w.e.Admin(), w.host, []string{hubspotBCC})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Basis == connector.AttendeeContact || got[0].ID == technical {
		t.Errorf("the technical attendee resolved as %+v, want no contact", got)
	}
}
