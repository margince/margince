// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Whether being COPIED on somebody else's message makes correspondence lawful.
//
// It does not, and the module already says so twice — authorizeevidence.go's
// authorIsTheSubject requires role 'from', and its own comment explains that
// counting a Cc "would let anyone create a lawful basis for writing to a third
// party by putting them in Cc".
//
// The Art 6(1)(f) arm underneath the send gate did not ask. It read
// activity_link, which is a FILING and carries no author concept, and that was
// harmless only while a message was filed under the party the ladder judged.
// Capture now files a message under every participant it resolves
// (capture/sinkmaillinks.go), so the filing stopped implying authorship — and
// without this the cc'd stranger becomes writable-to.
//
// An integration test because the defect is in a SQL join: a unit test over the
// Go around it passes with either query.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// filedInbound plants an inbound message filed under the subject, naming them
// in the given participant role — or in none at all when role is empty.
//
// Hand-inserted on purpose, unlike the fixtures that seed through a production
// writer. What is under test is what the READ counts, and the read must refuse
// this row's shape whatever wrote it: a caller may post an activity with
// direction=inbound and a link to any contact they can read, so the shape is
// reachable without capture being involved at all.
func (e *qualifyingEnv) filedInbound(t *testing.T, role string) {
	t.Helper()
	id := ids.NewV7()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
			VALUES ($1, 'email', 'inbound', 'gmail', now(), 'connector:gmail:x')`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, id, e.person); err != nil {
			return err
		}
		if role == "" {
			return nil
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, person_id, role)
			VALUES ($1, $2, $3)`, id, e.person, role)
		return err
	}); err != nil {
		t.Fatalf("planting an inbound message filed under the person (role %q): %v", role, err)
	}
}

// Being cc'd on a message somebody else wrote authorizes nothing.
//
// Mutation: drop the activity_participant join from inboundQualifyingEvent and
// this passes — the verdict turns allowed on a message the person only received.
func TestBeingCopiedOnAMessageDoesNotMakeCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "cc")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) after the person was merely cc'd, want not allowed — "+
			"a filing link says the message belongs on their record, never that they wrote it, "+
			"and counting it lets anyone manufacture a basis for writing to a third party by "+
			"putting them in Cc", got.State, got.Reason)
	}
}

// A message filed under somebody who is named in NO participant row authorizes
// nothing either. This is the same hole reached without a Cc: the link alone.
func TestAMessageFiledUnderSomebodyWhoWroteNothingAuthorizesNothing(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a message the person is only FILED under, want not allowed — "+
			"activity_link carries no author concept, so a caller who may file an activity under a "+
			"contact could otherwise write their own evidence for mailing them", got.State, got.Reason)
	}
}

// And the arm still answers for the person who actually wrote to us — the case
// the whole Art 6(1)(f) reading exists for.
//
// Without this the two refusals above are satisfied by an arm that refuses
// everybody, which is the failure mode a default-deny gate hides best.
func TestTheAuthorOfAnInboundMessageStillMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "from")

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) for the person who WROTE to us, want allowed — "+
			"they started the correspondence, which is the whole Art 6(1)(f) reading",
			got.State, got.Reason)
	}
	if got.Qualifying == nil || got.Qualifying.Kind != "inbound_message" {
		t.Errorf("qualifying event %+v, want an inbound_message — the basis has to name the "+
			"message it was read from, or nothing can be stamped for Art 5(2)", got.Qualifying)
	}
}

// attendedMeeting plants a meeting the subject was in, at the given offset from
// now — negative for one that has happened, positive for one in the diary.
//
// capturedBy and status are parameters because they are the two things the arm
// refuses on, and a fixture that hard-coded the passing value would let a test
// prove the feature while proving nothing about its bounds.
func (e *qualifyingEnv) attendedMeeting(t *testing.T, offset time.Duration, capturedBy, status string) {
	t.Helper()
	id := ids.NewV7()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, source, occurred_at, captured_by, meeting_status)
			VALUES ($1, 'meeting', 'gcal', now() + $2::interval, $3, NULLIF($4, ''))`,
			id, fmt.Sprintf("%d seconds", int(offset.Seconds())), capturedBy, status); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, person_id, role)
			VALUES ($1, $2, 'attendee')`, id, e.person)
		return err
	}); err != nil {
		t.Fatalf("planting a meeting at %s: %v", offset, err)
	}
}

// capturedMeeting is the ordinary case: a calendar the mailbox owner holds,
// synced by a connector, with no acceptance state recorded.
const capturedMeeting = "connector:gcal:x"

// A meeting we are about to have makes correspondence lawful.
//
// This is the case the whole arm exists for. A partner invited to a demo next
// week was refused as somebody who "has never written to you" — and the
// invitation made it worse, because the classifier read the machine-generated
// calendar mail as transactional and judged the person noise on it.
//
// Mutation: drop the meetingQualifyingEvent arm from latestQualifyingEvent and
// this fails, the verdict still unknown.
func TestAMeetingInTheDiaryMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, 4*24*time.Hour, capturedMeeting, "")

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) for somebody we are meeting in four days, want allowed — "+
			"a meeting means both sides put time in a calendar, which is stronger evidence of a "+
			"relationship than an email anybody may send unsolicited", got.State, got.Reason)
	}
	if got.Qualifying == nil || got.Qualifying.Kind != KindMeeting {
		t.Fatalf("qualifying event %+v, want a %s — the basis has to name the meeting it was read "+
			"from, or nothing can be stamped for Art 5(2)", got.Qualifying, KindMeeting)
	}
}

// A meeting that already happened counts too, while it is inside the window.
func TestAMeetingThatHappenedMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, -24*time.Hour, capturedMeeting, "")

	if got := e.verdict(t); got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) for somebody we met yesterday, want allowed", got.State, got.Reason)
	}
}

// A meeting a HUMAN logged is not evidence, however it is labelled.
//
// POST /activities takes kind, occurred_at and links from the request body, and
// the log path stamps a participant row for every linked person. So without this
// bound any seat that can see a contact could log a "meeting" naming them and
// mail them on the strength of it — writing its own permission slip.
//
// Mutation: drop the captured_by clause and this passes, which is the whole
// forgery.
func TestAHandLoggedMeetingIsNotEvidence(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, 4*24*time.Hour, "human:"+e.user.String(), "")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a meeting a seat logged by hand, want not allowed — "+
			"the request body names the kind, the date and the people, so anybody who can see a "+
			"contact could otherwise authorize themselves to mail them", got.State, got.Reason)
	}
}

// A meeting that was declined or abandoned is not evidence either.
func TestADeclinedOrAbandonedMeetingIsNotEvidence(t *testing.T) {
	for _, status := range []string{"no_show", "canceled"} {
		t.Run(status, func(t *testing.T) {
			e := setupQualifying(t)

			e.attendedMeeting(t, -24*time.Hour, capturedMeeting, status)

			if got := e.verdict(t); got.State == VerdictAllowed {
				t.Fatalf("verdict %q (%s) on a %s meeting, want not allowed — an invitation "+
					"somebody declined is the opposite of evidence they welcome contact",
					got.State, got.Reason, status)
			}
		})
	}
}

// A meeting dated beyond the horizon is not evidence.
//
// The derived row is stamped and read back afterwards without revalidating its
// source, so a meeting dated far enough ahead would authorize sending forever.
func TestAMeetingBeyondTheHorizonIsNotEvidence(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, 365*24*time.Hour, capturedMeeting, "")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a meeting a year out, want not allowed — the stamped row "+
			"outlives its source, so a far-future date would authorize sending indefinitely",
			got.State, got.Reason)
	}
}

// Somebody with no meeting and nothing else on the record is still refused.
//
// Without this the tests above are satisfied by an arm that allows nobody, which
// is the failure mode a default-deny gate hides best.
func TestSomebodyWithNoMeetingIsStillRefused(t *testing.T) {
	e := setupQualifying(t)

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) for somebody with nothing on the record, want not allowed — "+
			"default-deny is the whole posture", got.State, got.Reason)
	}
}
