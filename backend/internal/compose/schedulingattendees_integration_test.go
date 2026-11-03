// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type attendeeWorld struct {
	e        *integration.Env
	calendar schedulingCalendar
	host     ids.UserID
	purpose  ids.PurposeID
}

func newAttendeeWorld(t *testing.T) attendeeWorld {
	t.Helper()
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	w := attendeeWorld{e: e, calendar: newSchedulingCalendar(e.Pool, nil), host: ids.From[ids.UserKind](e.Rep1)}
	purpose, err := consent.NewStore(InstallationDB(e.Pool)).CreatePurpose(e.Admin(), "transactional", "transactional", false)
	if err != nil {
		t.Fatal(err)
	}
	w.purpose = purpose.ID
	return w
}

// contact seeds a contact owned by owner at address, with a transactional
// consent in state ("" records none).
func (w attendeeWorld) contact(t *testing.T, owner ids.UUID, address, state string) ids.UUID {
	t.Helper()
	id := w.e.SeedContact(t, address, &owner)
	w.e.WsExec(t, `INSERT INTO contact_email(contact_id,email,is_primary,source,captured_by) VALUES ($1,$2,true,'manual','human:test')`, id, address)
	if state != "" {
		if _, err := consent.NewStore(w.e.DB()).Record(w.e.Admin(), consent.RecordInput{ContactID: ids.From[ids.ContactKind](id), PurposeID: w.purpose, NewState: state, PolicyText: &grantedWording}); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestABookingAttendeeResolvesContactFirstUnderTheHostsScope(t *testing.T) {
	w := newAttendeeWorld(t)
	visible := w.contact(t, w.e.Rep1, "visible@buyer.test", "")
	hidden := w.contact(t, w.e.Rep3, "hidden@buyer.test", "")
	w.e.MakeCapturePrivate(t, "contact", hidden, w.e.Rep3)
	colleague := w.e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, w.e.Rep2)
	alsoContact := w.e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, w.e.Rep3)
	shared := w.contact(t, w.e.Rep1, alsoContact, "")

	got, err := w.calendar.ResolveAttendees(w.e.Admin(), w.host, []string{"visible@buyer.test", "hidden@buyer.test", colleague, alsoContact, "stranger@elsewhere.test"})
	if err != nil {
		t.Fatal(err)
	}
	want := []connector.CalendarAttendee{
		{Address: "visible@buyer.test", Basis: connector.AttendeeContact, ID: visible, AskConsent: true},
		{Address: "hidden@buyer.test", Basis: connector.AttendeeHostNamed, AskConsent: true},
		{Address: colleague, Basis: connector.AttendeeColleague, ID: w.e.Rep2},
		{Address: alsoContact, Basis: connector.AttendeeContact, ID: shared, AskConsent: true},
		{Address: "stranger@elsewhere.test", Basis: connector.AttendeeHostNamed},
	}
	if !slices.Equal(got, want) {
		t.Errorf("resolved\n %+v\nwant\n %+v", got, want)
	}
	if got[1].ID == hidden {
		t.Error("a contact the host cannot see leaked its id onto the roster")
	}
}

// The provider save asks the consent gate for contacts and leads only. A
// colleague and an address no record holds are invited without it. A contact
// who withdrew consent still stops the whole invitation.
func TestOnlyColleaguesAndUnknownAddressesSkipTheConsentGate(t *testing.T) {
	w := newAttendeeWorld(t)
	invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
	w.contact(t, w.e.Rep1, "withdrawn@buyer.test", "withdrawn")
	colleague := w.e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, w.e.Rep2)
	appointment := func(attendees ...string) connector.CalendarAppointment {
		return connector.CalendarAppointment{ContactID: invitee, Attendees: attendees, Roster: make([]connector.CalendarAttendee, len(attendees))}
	}
	gate := consentGateFor(w.e.Pool)

	gated, err := w.calendar.consentAddresses(w.e.Admin(), w.host, appointment("invitee@buyer.test", colleague, "stranger@elsewhere.test"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(gated, []string{"invitee@buyer.test"}) {
		t.Errorf("gated = %v, want only the invitee", gated)
	}
	if err := gate.RequireGrantedForEmails(w.e.Admin(), gated, "transactional"); err != nil {
		t.Errorf("a colleague and a host-named address beside a consenting invitee were refused: %v", err)
	}

	gated, err = w.calendar.consentAddresses(w.e.Admin(), w.host, appointment("invitee@buyer.test", "stranger@elsewhere.test", "withdrawn@buyer.test"))
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.RequireGrantedForEmails(w.e.Admin(), gated, "transactional"); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Errorf("a withdrawn contact beside a host-named address answered %v, want the consent refusal", err)
	}
}
