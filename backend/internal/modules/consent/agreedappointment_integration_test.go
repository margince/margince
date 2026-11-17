// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// appointment describes a calendar meeting with the contact, as a test plants it.
type appointment struct {
	capturedBy, status, response string
	in                           time.Duration
	relinked                     bool
}

// accepted is the meeting a moved appointment rests on: captured from the
// calendar, booked two days ahead, accepted by the contact.
var accepted = appointment{capturedBy: "connector:gcal", status: "booked", response: "accepted", in: 48 * time.Hour}

// plantAppointment writes the meeting and the contact's place on it.
func (e *resolveEnv) plantAppointment(t *testing.T, a appointment) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, source, occurred_at, captured_by, meeting_status)
		VALUES ($1, 'meeting', 'gcal', $2, $3, NULLIF($4, ''))`,
		id, clocktest.Now(t).Add(a.in), a.capturedBy, a.status); err != nil {
		t.Fatalf("planting the appointment: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, contact_id, role, response, relinked_at)
		VALUES ($1, $2, 'attendee', NULLIF($3, ''), CASE WHEN $4 THEN now() END)`,
		id, e.contact, a.response, a.relinked); err != nil {
		t.Fatalf("planting the attendee: %v", err)
	}
	return id
}

// A message about moving an appointment the contact agreed to continues it:
// the requested follow-up is supported by the calendar meeting still to come.
func TestMovingAnAgreedAppointmentIsARequestedFollowup(t *testing.T) {
	e := setupResolve(t)
	meeting := e.plantAppointment(t, accepted)

	for name, evidence := range map[string]commsauthz.Evidence{
		"found by the engine": {},
		"named by the draft":  {ActivityID: meeting},
	} {
		d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, Evidence: evidence})
		if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryRequestedFollowup {
			t.Errorf("%s: %s %q resolved %q, want an allowed requested follow-up", name, d.Verdict, d.ReasonCode, d.Resolved)
		}
	}
}

// An appointment is evidence only when it is real, still on, and the one named.
func TestOnlyAnAgreedAppointmentStillToComeSupportsTheFollowup(t *testing.T) {
	with := func(change func(*appointment)) appointment {
		a := accepted
		change(&a)
		return a
	}
	cases := map[string]struct {
		planted appointment
		named   bool
	}{
		"typed by hand":         {planted: with(func(a *appointment) { a.capturedBy = "human:rep" })},
		"called off":            {planted: with(func(a *appointment) { a.status = "canceled" })},
		"declined":              {planted: with(func(a *appointment) { a.response = "declined" })},
		"never answered":        {planted: with(func(a *appointment) { a.response = "needs_action" })},
		"relinked onto them":    {planted: with(func(a *appointment) { a.relinked = true })},
		"held years ago":        {planted: with(func(a *appointment) { a.in = -5 * 365 * 24 * time.Hour; a.status = "" })},
		"another meeting named": {planted: accepted, named: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			e.plantAppointment(t, tc.planted)
			var evidence commsauthz.Evidence
			if tc.named {
				evidence.ActivityID = ids.NewV7()
			}
			d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, Evidence: evidence})
			if d.Verdict == commsauthz.VerdictAllow || d.ReasonCode != commsauthz.ReasonNoEvidence {
				t.Errorf("%s %q, want refused for %q", d.Verdict, d.ReasonCode, commsauthz.ReasonNoEvidence)
			}
		})
	}
}

// A meeting proves the appointment, never the sender's purpose. Marketing about
// it stays marketing, and a message that names no reason is not given one.
func TestAnAppointmentLendsNoReasonToAMessageThatClaimsAnother(t *testing.T) {
	e := setupResolve(t)
	e.plantAppointment(t, accepted)

	if d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryMarketing}); d.Resolved == commsauthz.CategoryRequestedFollowup || d.Verdict == commsauthz.VerdictAllow {
		t.Errorf("marketing beside an appointment = %s %q resolved %q, want refused as marketing", d.Verdict, d.ReasonCode, d.Resolved)
	}
	if d := e.decide(t, commsauthz.Request{}); d.ReasonCode != commsauthz.ReasonUnknownPurpose {
		t.Errorf("no reason beside an appointment = %s %q, want %q", d.Verdict, d.ReasonCode, commsauthz.ReasonUnknownPurpose)
	}
}
