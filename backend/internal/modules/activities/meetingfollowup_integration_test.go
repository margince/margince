// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// metWith seeds a meeting with the contact, `ago` back, held by host.
func (e *loadEnv) metWith(t *testing.T, contact, host ids.UUID, status string, ago time.Duration) ids.UUID {
	t.Helper()
	meeting := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status, host_user_id, source, captured_by)
		VALUES ($1, 'meeting', 'Discovery workshop', now() - $2::interval, 3600, $3, $4, 'seed', 'system')`,
		meeting, ago.String(), status, host)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), meeting, contact)
	return meeting
}

func owesMeetingFollowUp(t *testing.T, s *Store, e *loadEnv, meeting ids.UUID) bool {
	t.Helper()
	rows, err := s.MeetingsAwaitingFollowUp(e.followUpReader(), time.Now())
	if err != nil {
		t.Fatalf("reading meeting follow-ups: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == meeting {
			return true
		}
	}
	return false
}

func TestAMeetingWithNothingSentSinceIsAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)

	quiet := e.metWith(t, e.customerAt(t, "customer"), e.rep, "held", 3*24*time.Hour)

	theyWrote := e.customerAt(t, "customer")
	stillOwed := e.metWith(t, theyWrote, e.rep, "held", 3*24*time.Hour)
	e.afterwards(t, theyWrote, "email", "inbound", "")

	if !owesMeetingFollowUp(t, s, e, quiet) {
		t.Error("a meeting three days ago with nothing sent since is not a follow-up")
	}
	if !owesMeetingFollowUp(t, s, e, stillOwed) {
		t.Error("the customer writing after the meeting settled the reader's own follow-up")
	}
}

func TestAMeetingFollowUpNeedsAHeldMeetingOfTheReadersWithNothingSent(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)

	wrote := e.customerAt(t, "customer")
	followedUp := e.metWith(t, wrote, e.rep, "held", 3*24*time.Hour)
	e.afterwards(t, wrote, "email", "outbound", "")

	canceled := e.metWith(t, e.customerAt(t, "customer"), e.rep, "canceled", 3*24*time.Hour)
	fresh := e.metWith(t, e.customerAt(t, "customer"), e.rep, "held", 20*time.Hour)
	colleagues := e.metWith(t, e.customerAt(t, "customer"), e.other, "held", 3*24*time.Hour)

	for name, meeting := range map[string]ids.UUID{
		"followed up by email": followedUp, "canceled": canceled,
		"inside the window": fresh, "a colleague's": colleagues,
	} {
		if owesMeetingFollowUp(t, s, e, meeting) {
			t.Errorf("a %s meeting is a follow-up", name)
		}
	}
}

// Importing a copy of a colleague's meeting is not taking part in it, so it
// owes no follow-up. Being recorded as an attendee is, whoever hosted it.
func TestAMeetingTheReaderOnlyImportedIsNotTheirsButOneTheyAttendedIs(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	imported := e.metWith(t, e.customerAt(t, "customer"), e.other, "held", 3*24*time.Hour)
	e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, imported, e.rep)
	if owesMeetingFollowUp(t, s, e, imported) {
		t.Error("a meeting the reader only imported is their follow-up")
	}
	attended := e.metWith(t, e.customerAt(t, "customer"), e.other, "held", 3*24*time.Hour)
	e.exec(t, `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'attendee')`, attended, e.rep)
	if !owesMeetingFollowUp(t, s, e, attended) {
		t.Error("a colleague's meeting the reader attended is not their follow-up")
	}
}

// A message sent while the meeting was still on is not the follow-up after it.
func TestAMessageSentDuringTheMeetingDoesNotSettleIt(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	contact := e.customerAt(t, "customer")
	meeting := e.metWith(t, contact, e.rep, "held", 3*24*time.Hour)
	during := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'the deck', now() - interval '3 days' + interval '10 minutes', 'seed', 'system')`, during)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), during, contact)
	if !owesMeetingFollowUp(t, s, e, meeting) {
		t.Error("a message sent during the meeting settled the follow-up after it")
	}
}
