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

// queued is the appointment as the engine stores it. The invitee comes first,
// then each further address with the roster entry it resolved to when booked.
func (w attendeeWorld) queued(t *testing.T, invitee ids.UUID, attendees ...string) connector.CalendarAppointment {
	t.Helper()
	further, err := w.calendar.ResolveAttendees(w.e.Admin(), w.host, attendees[1:])
	if err != nil {
		t.Fatal(err)
	}
	roster := append([]connector.CalendarAttendee{{Address: attendees[0], Basis: connector.AttendeeContact, ID: invitee, AskConsent: true}}, further...)
	return connector.CalendarAppointment{ContactID: invitee, Attendees: attendees, Roster: roster}
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
		return w.queued(t, invitee, attendees...)
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

// A further attendee who was a contact when the booking was queued stays
// under the consent gate at send time. That holds even once the contact no
// longer has the address. Re-resolved alone it would read as host-named and go out
// unchecked. The gate instead refuses an address it ties to no subject.
func TestAQueuedContactThatLosesItsAddressIsNotSentUnchecked(t *testing.T) {
	for name, lose := range map[string]string{
		"address removed":  `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`,
		"contact archived": `UPDATE contact SET archived_at = now() WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			w := newAttendeeWorld(t)
			invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
			further := w.contact(t, w.e.Rep1, "further@buyer.test", "granted")
			queued := w.queued(t, invitee, "invitee@buyer.test", "further@buyer.test")
			if !queued.Roster[1].AskConsent {
				t.Fatalf("the further contact was queued without a consent check: %+v", queued.Roster[1])
			}
			w.e.WsExec(t, lose, further)

			gated, err := w.calendar.consentAddresses(w.e.Admin(), w.host, queued)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(gated, "further@buyer.test") {
				t.Fatalf("gated = %v: the address left the consent gate once its contact lost it", gated)
			}
			if err := consentGateFor(w.e.Pool).RequireGrantedForEmails(w.e.Admin(), gated, "transactional"); !errors.Is(err, apperrors.ErrConsentNotGranted) {
				t.Errorf("the gate answered %v for an address no subject holds any more, want the consent refusal", err)
			}
		})
	}
}

// Every further address a contact or lead holds goes to the consent gate, the
// archived and promoted ones too, however the address is cased. An address no
// record holds goes out unless a stop is recorded on it.
func TestEveryGuestARecordHoldsIsAskedOfTheGate(t *testing.T) {
	refused := func(t *testing.T, w attendeeWorld, invitee ids.UUID, guest string) {
		t.Helper()
		gated, err := w.calendar.consentAddresses(w.e.Admin(), w.host, w.queued(t, invitee, "invitee@buyer.test", guest))
		if err == nil {
			err = consentGateFor(w.e.Pool).RequireGrantedForEmails(w.e.Admin(), gated, "transactional")
		}
		if !errors.Is(err, apperrors.ErrConsentNotGranted) {
			t.Fatalf("%s answered %v, want the consent refusal", guest, err)
		}
	}
	lead := func(t *testing.T, w attendeeWorld, address string) ids.UUID {
		t.Helper()
		id := ids.NewV7()
		w.e.WsExec(t, `INSERT INTO lead (id, full_name, email, source, captured_by) VALUES ($1, 'Lea Lead', $2, 'manual', 'human:test')`, id, address)
		return id
	}

	t.Run("an archived contact who withdrew", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		gone := w.contact(t, w.e.Rep1, "gone@buyer.test", "withdrawn")
		w.e.WsExec(t, `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, gone)
		w.e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, gone)
		refused(t, w, invitee, "gone@buyer.test")
	})
	t.Run("an archived lead", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		w.e.WsExec(t, `UPDATE lead SET archived_at = now() WHERE id = $1`, lead(t, w, "archived@lead.test"))
		refused(t, w, invitee, "archived@lead.test")
	})
	t.Run("a promoted lead", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		w.e.WsExec(t, `UPDATE lead SET status = 'promoted', promoted_contact_id = $2, archived_at = now() WHERE id = $1`, lead(t, w, "promoted@lead.test"), invitee)
		refused(t, w, invitee, "promoted@lead.test")
	})
	t.Run("a withdrawn contact typed in mixed case", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		w.contact(t, w.e.Rep1, "withdrawn@buyer.test", "withdrawn")
		refused(t, w, invitee, "Withdrawn@Buyer.TEST")
	})
	t.Run("a suppressed address no record holds", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		w.e.WsExec(t, `INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level)
			VALUES ('bounced@elsewhere.test', 'hard_bounce', 'test', 'human:test', 'subject')`)
		refused(t, w, invitee, "bounced@elsewhere.test")
	})
	stopped := func(t *testing.T, w attendeeWorld, address, kind string) {
		t.Helper()
		w.e.WsExec(t, `INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level)
			VALUES ($1, $2, 'test', 'human:test', 'subject')`, address, kind)
	}
	t.Run("a stop on the address of a contact who granted consent", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		w.contact(t, w.e.Rep1, "bounced@buyer.test", "granted")
		stopped(t, w, "bounced@buyer.test", "hard_bounce")
		refused(t, w, invitee, "bounced@buyer.test")
	})
	t.Run("a restriction on an address no record holds", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		stopped(t, w, "restricted@elsewhere.test", "processing_restriction")
		refused(t, w, invitee, "restricted@elsewhere.test")
	})
	t.Run("a marketing objection does not stop an invitation", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		stopped(t, w, "objector@elsewhere.test", "marketing_objection")
		gated, err := w.calendar.consentAddresses(w.e.Admin(), w.host, w.queued(t, invitee, "invitee@buyer.test", "objector@elsewhere.test"))
		if err != nil {
			t.Fatalf("a marketing objection refused a calendar invitation: %v", err)
		}
		if err := consentGateFor(w.e.Pool).RequireGrantedForEmails(w.e.Admin(), gated, "transactional"); err != nil {
			t.Fatalf("a marketing objection refused a calendar invitation: %v", err)
		}
	})
	t.Run("a lead typed in mixed case", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		lead(t, w, "mixed@lead.test")
		refused(t, w, invitee, "Mixed@Lead.TEST")
	})
	t.Run("a contact's secondary address", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		other := w.contact(t, w.e.Rep1, "primary@buyer.test", "withdrawn")
		w.e.WsExec(t, `INSERT INTO contact_email(contact_id,email,is_primary,source,captured_by) VALUES ($1,'secondary@buyer.test',false,'manual','human:test')`, other)
		refused(t, w, invitee, "secondary@buyer.test")
	})
	t.Run("a merged contact's address", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		source := w.contact(t, w.e.Rep1, "merged@buyer.test", "withdrawn")
		target := w.contact(t, w.e.Rep1, "survivor@buyer.test", "")
		if _, err := w.e.Contacts.MergeContact(w.e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), nil); err != nil {
			t.Fatal(err)
		}
		refused(t, w, invitee, "merged@buyer.test")
	})
	t.Run("an address no record holds", func(t *testing.T) {
		w := newAttendeeWorld(t)
		invitee := w.contact(t, w.e.Rep1, "invitee@buyer.test", "granted")
		gated, err := w.calendar.consentAddresses(w.e.Admin(), w.host, w.queued(t, invitee, "invitee@buyer.test", "stranger@elsewhere.test"))
		if err != nil {
			t.Fatal(err)
		}
		if err := consentGateFor(w.e.Pool).RequireGrantedForEmails(w.e.Admin(), gated, "transactional"); err != nil {
			t.Fatalf("an address no record holds was refused: %v", err)
		}
	})
}
