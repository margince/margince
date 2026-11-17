// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// appointmentAhead plants a meeting two days ahead that the contact attends,
// captured by capturedBy with the given status.
func (e *resolveEnv) appointmentAhead(t *testing.T, capturedBy, status string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, source, occurred_at, captured_by, meeting_status)
		VALUES ($1, 'meeting', 'gcal', $2, $3, NULLIF($4, ''))`,
		id, time.Now().Add(48*time.Hour), capturedBy, status); err != nil {
		t.Fatalf("planting the appointment: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'attendee')`,
		id, e.contact); err != nil {
		t.Fatalf("planting the attendee: %v", err)
	}
	return id
}

// A message about moving an appointment the contact agreed to continues it:
// the requested follow-up is supported by the calendar meeting still to come.
func TestMovingAnAgreedAppointmentIsARequestedFollowup(t *testing.T) {
	e := setupResolve(t)
	meeting := e.appointmentAhead(t, "connector:gcal", "booked")

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
	cases := map[string]func(e *resolveEnv) commsauthz.Evidence{
		"typed by hand": func(e *resolveEnv) commsauthz.Evidence {
			e.appointmentAhead(t, "human:rep", "booked")
			return commsauthz.Evidence{}
		},
		"called off": func(e *resolveEnv) commsauthz.Evidence {
			e.appointmentAhead(t, "connector:gcal", "canceled")
			return commsauthz.Evidence{}
		},
		"another meeting named": func(e *resolveEnv) commsauthz.Evidence {
			e.appointmentAhead(t, "connector:gcal", "booked")
			return commsauthz.Evidence{ActivityID: ids.NewV7()}
		},
	}
	for name, plant := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, Evidence: plant(e)})
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
	e.appointmentAhead(t, "connector:gcal", "booked")

	if d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryMarketing}); d.Resolved == commsauthz.CategoryRequestedFollowup || d.Verdict == commsauthz.VerdictAllow {
		t.Errorf("marketing beside an appointment = %s %q resolved %q, want refused as marketing", d.Verdict, d.ReasonCode, d.Resolved)
	}
	if d := e.decide(t, commsauthz.Request{}); d.ReasonCode != commsauthz.ReasonUnknownPurpose {
		t.Errorf("no reason beside an appointment = %s %q, want %q", d.Verdict, d.ReasonCode, commsauthz.ReasonUnknownPurpose)
	}
}
