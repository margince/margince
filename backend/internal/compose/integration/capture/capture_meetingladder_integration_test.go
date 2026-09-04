// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Whether a meeting counts as having dealt with somebody.
//
// The counterparty ladder asked two questions about an address — did they write
// back, and did we write to them on two separate threads — and both are about
// MAIL. A meeting satisfied neither, and worse, it never reached the tiers at
// all: a meeting names no counterparty, because attendance is a LIST and the
// mapper leaves the field unset, so the ladder answered "named nobody" and
// stopped. A partner the workspace was meeting could not become a contact,
// while the invitation MAIL beside the meeting was read as machine-generated
// and judged that same partner noise.
//
// These run through compose.NewCaptureRegistry — the production wiring — for
// the reason this file's first draft got wrong: a bare sink has no ensurer, so
// every creation assertion against one passes or fails for a reason that has
// nothing to do with the ladder.
//
// The bounds carry most of the weight. A meeting is caller-supplied data unless
// something says otherwise: POST /activities takes the kind, the date and the
// people from a request body, so an unbounded arm would let any seat mint a
// contact by logging a "meeting".

import (
	"testing"
)

// meetingGuest is the external party on the meetings below.
const meetingGuest = "guest@partner.example"

// syncMeeting lands one record as a MEETING through the production registry,
// with the guest as its external party.
//
// syncAsKind rather than a calendar connector: what is under test is the ladder,
// which reads the activity kind and the addresses the record names, and driving
// it through the mail lane keeps the fixture to the one fact each test varies.
func syncMeeting(t *testing.T, env captureEnv, msgID string) {
	t.Helper()
	env.syncAsKind(t, map[string]string{msgID: "meeting"},
		email(meetingGuest, "Guest Partner", captureOwner, msgID, ""))
}

// The guest on a captured meeting becomes a contact.
//
// Mutation: drop the meetingKind arm from ladderSubjectTx and this fails with
// nobody created — which is the state that refused a partner the workspace was
// meeting.
func TestTheGuestOnACapturedMeetingBecomesAContact(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e

	syncMeeting(t, env, "meet1@partner.example")

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'guest@partner.example' AND p.archived_at IS NULL`); n != 1 {
		t.Fatalf("%d contacts for somebody the workspace is meeting, want 1 — mail is evidence "+
			"about intent, and a meeting is evidence about time: both sides put an hour in a "+
			"calendar, which nobody does by accident", n)
	}
}

// An ordinary message from the same address still does NOT create on its own.
//
// The admit case for the whole tier: one send is intent, and intent is often
// unreturned. Without this the test above is satisfied by a ladder that creates
// for everybody, which is what the exchange requirement exists to prevent.
func TestOneOrdinaryMessageStillCreatesNobody(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, email(meetingGuest, "Guest Partner", captureOwner, "mail1@partner.example", ""))

	if n := countRows(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		WHERE pe.email = 'guest@partner.example' AND p.archived_at IS NULL`); n != 0 {
		t.Fatalf("%d contacts from a single ordinary message, want 0 — one send is intent, "+
			"and a meeting is what this change admits, not any message at all", n)
	}
}
