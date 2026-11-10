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

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
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
			INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, e.contact); err != nil {
			return err
		}
		if role == "" {
			return nil
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, $3)`, id, e.contact, role)
		return err
	}); err != nil {
		t.Fatalf("planting an inbound message filed under the contact (role %q): %v", role, err)
	}
}

// Being cc'd on a message somebody else wrote authorizes nothing.
//
// Mutation: drop the activity_participant join from inboundQualifyingEvent and
// this passes — the verdict turns allowed on a message the contact only received.
func TestBeingCopiedOnAMessageDoesNotMakeCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "cc")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) after the contact was merely cc'd, want not allowed — "+
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
		t.Fatalf("verdict %q (%s) on a message the contact is only FILED under, want not allowed — "+
			"activity_link carries no author concept, so a caller who may file an activity under a "+
			"contact could otherwise write their own evidence for mailing them", got.State, got.Reason)
	}
}

// And the arm still answers for the contact who actually wrote to us — the case
// the whole Art 6(1)(f) reading exists for.
//
// Without this the two refusals above are satisfied by an arm that refuses
// everybody, which is the failure mode a default-deny gate hides best.
func TestTheAuthorOfAnInboundMessageStillMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.filedInbound(t, "from")

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) for the contact who WROTE to us, want allowed — "+
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
// capturedBy, status and counterparty are parameters because they are the three
// things the arm refuses on, and a fixture that hard-coded the passing value
// would let a test prove the feature while proving nothing about its bounds.
func (e *qualifyingEnv) attendedMeeting(t *testing.T, offset time.Duration, capturedBy, status, counterparty string) {
	t.Helper()
	id := ids.NewV7()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, source, occurred_at, captured_by, meeting_status, counterparty_email)
			VALUES ($1, 'meeting', 'gcal', now() + $2::interval, $3, NULLIF($4, ''), NULLIF($5, ''))`,
			id, fmt.Sprintf("%d seconds", int(offset.Seconds())), capturedBy, status, counterparty); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'attendee')`, id, e.contact)
		return err
	}); err != nil {
		t.Fatalf("planting a meeting at %s: %v", offset, err)
	}
}

// attendedMeetingAt plants a captured meeting the subject was in at a stated
// instant, read as a day entry when day is set. The time is the fixture's,
// never the database's now(), so a test can read it on a clock of its own.
func (e *qualifyingEnv) attendedMeetingAt(t *testing.T, at time.Time, day bool) ids.UUID {
	t.Helper()
	precision := "instant"
	if day {
		at, precision = occurredday.AnchorIn(at, time.UTC), "day"
	}
	id := ids.NewV7()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, source, occurred_at, occurred_precision, captured_by)
			VALUES ($1, 'meeting', 'gcal', $2, $3, $4)`, id, at, precision, capturedMeeting); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, contact_id, role)
			VALUES ($1, $2, 'attendee')`, id, e.contact)
		return err
	}); err != nil {
		t.Fatalf("planting a meeting at %s: %v", at, err)
	}
	return id
}

// The ordinary case, spelled out at the call sites: a calendar the mailbox owner
// holds, synced by a connector, with no acceptance state recorded and no
// counterparty — attendance is a LIST, so the mapper leaves the field unset, and
// that shape is what tells a calendar record from mail wearing its kind.
const (
	capturedMeeting   = "connector:gcal:x"
	noAcceptanceState = ""
	namesNobody       = ""
)

// A captured meeting that already happened makes correspondence lawful.
//
// A meeting means both sides put time in a calendar, which nobody does by
// accident. The invitation itself does not prove it: the classifier reads the
// machine-generated calendar mail as transactional.
//
// Mutation: drop the meetingQualifyingEvent arm from latestQualifyingEvent and
// this fails, the verdict still unknown.
func TestAMeetingThatHappenedMakesCorrespondenceLawful(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, -24*time.Hour, capturedMeeting, noAcceptanceState, namesNobody)

	got := e.verdict(t)
	if got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) for somebody we met yesterday, want allowed — a meeting is "+
			"stronger evidence of a relationship than an email anybody may send unsolicited", got.State, got.Reason)
	}
	if got.Qualifying == nil || got.Qualifying.Kind != KindMeeting {
		t.Fatalf("qualifying event %+v, want a %s — the basis has to name the meeting it was read "+
			"from, or nothing can be stamped for Art 5(2)", got.Qualifying, KindMeeting)
	}
}

// A meeting still in the diary is not evidence yet, however soon it is. The
// derived row is stamped and read back without revalidating its source, so a
// booking would otherwise stand as ground even if it never happened.
//
// Mutation: drop the occurredday.MeetingTookPlaceSQL clause from
// capturedMeetingSQL and this fails.
func TestAMeetingStillInTheDiaryIsNotEvidence(t *testing.T) {
	for name, offset := range map[string]time.Duration{
		"in four days": 4 * 24 * time.Hour,
		"a year out":   365 * 24 * time.Hour,
	} {
		t.Run(name, func(t *testing.T) {
			e := setupQualifying(t)

			e.attendedMeeting(t, offset, capturedMeeting, noAcceptanceState, namesNobody)

			if got := e.verdict(t); got.State == VerdictAllowed {
				t.Fatalf("verdict %q (%s) on a meeting %s, want not allowed — a meeting counts "+
					"only once its time has passed", got.State, got.Reason, name)
			}
		})
	}
}

// A meeting a HUMAN logged is not evidence, however it is labelled.
//
// POST /activities takes kind, occurred_at and links from the request body, and
// the log path stamps a participant row for every linked contact. So without this
// bound any seat that can see a contact could log a "meeting" naming them and
// mail them on the strength of it — writing its own permission slip.
//
// Mutation: drop the captured_by clause and this passes, which is the whole
// forgery.
func TestAHandLoggedMeetingIsNotEvidence(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, -24*time.Hour, "human:"+e.user.String(), noAcceptanceState, namesNobody)

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a meeting a seat logged by hand, want not allowed — "+
			"the request body names the kind, the date and the contacts, so anybody who can see a "+
			"contact could otherwise authorize themselves to mail them", got.State, got.Reason)
	}
}

// A meeting that was declined or abandoned is not evidence either.
func TestADeclinedOrAbandonedMeetingIsNotEvidence(t *testing.T) {
	for _, status := range []string{"no_show", "canceled"} {
		t.Run(status, func(t *testing.T) {
			e := setupQualifying(t)

			e.attendedMeeting(t, -24*time.Hour, capturedMeeting, status, namesNobody)

			if got := e.verdict(t); got.State == VerdictAllowed {
				t.Fatalf("verdict %q (%s) on a %s meeting, want not allowed — an invitation "+
					"somebody declined is the opposite of evidence they welcome contact",
					got.State, got.Reason, status)
			}
		})
	}
}

// A record that NAMES a sender is not a meeting, whatever kind it wears.
//
// A connector reports the kind per message and the extension ingress copies it
// off a third-party unit's record with no vocabulary check in front of it, so a
// message a connector really did capture can arrive wearing this one. This arm
// applies no authorship test — every attendee of a meeting is a party to it —
// so without the counterparty bound, relabelling a message somebody was merely
// copied on makes writing to everyone on it lawful.
//
// Mutation: drop the counterparty_email clause and this passes.
func TestARecordNamingASenderIsNotEvidenceWhateverItsKind(t *testing.T) {
	e := setupQualifying(t)

	e.attendedMeeting(t, -24*time.Hour, capturedMeeting, noAcceptanceState, "sender@example.com")

	if got := e.verdict(t); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a record naming a counterparty, want not allowed — "+
			"attendance is a list, so a record that names one party is mail however it is "+
			"labelled, and the label is a word the caller chose", got.State, got.Reason)
	}
}

// The meeting is read on the verdict's own clock, not the database's. A
// meeting an hour after the database's now() is past for a clock two hours on.
//
// Mutation: render capturedMeetingSQL with now() and this fails.
func TestAMeetingIsJudgedOnTheVerdictsClock(t *testing.T) {
	e := setupQualifying(t)
	meetsAt := clocktest.Now(t).Add(time.Hour)
	e.attendedMeetingAt(t, meetsAt, false)

	after, before := meetsAt.Add(time.Hour), meetsAt.Add(-time.Hour)
	if got := e.verdictAt(t, after.Add(-defaultReplyWindow), after); got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a clock past the meeting, want allowed", got.State, got.Reason)
	}
	if got := e.verdictAt(t, before.Add(-defaultReplyWindow), before); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a clock before the meeting, want not allowed", got.State, got.Reason)
	}
}

// A retained stamp for a future meeting follows the actual meeting time.
func TestAMeetingStampedBeforeItHappensCountsOnlyOnceItIsPast(t *testing.T) {
	e := setupQualifying(t)
	meetsAt := clocktest.Now(t).Add(time.Hour)
	meeting := e.attendedMeetingAt(t, meetsAt, false)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return stampQualifyingEvent(e.ctx, tx, e.contact.String(), QualifyingEvent{
			Kind: KindMeeting, OccurredAt: meetsAt,
			SourceEntityType: "activity", SourceEntityID: meeting.String(),
		}, sourceKindDerived, "user:"+e.user.String())
	}); err != nil {
		t.Fatalf("stamping the meeting: %v", err)
	}

	after, before := meetsAt.Add(time.Hour), meetsAt.Add(-time.Hour)
	if got := e.verdictAt(t, before.Add(-defaultReplyWindow), before); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a clock before the stamped meeting, want not allowed", got.State, got.Reason)
	}
	if got := e.verdictAt(t, after.Add(-defaultReplyWindow), after); got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) on a clock past the stamped meeting, want allowed", got.State, got.Reason)
	}
}

// A meeting known only by its date has not taken place until the date is over.
// It is evidence from the next day on, and not before.
func TestADayMeetingIsEvidenceOnceItsDateEnds(t *testing.T) {
	e := setupQualifying(t)
	today := clocktest.Now(t).UTC()
	e.attendedMeetingAt(t, today, true)

	tomorrow := today.Truncate(24 * time.Hour).Add(25 * time.Hour)
	if got := e.verdictAt(t, today.Add(-defaultReplyWindow), today); got.State == VerdictAllowed {
		t.Fatalf("verdict %q (%s) on the meeting's own date, want not allowed", got.State, got.Reason)
	}
	if got := e.verdictAt(t, tomorrow.Add(-defaultReplyWindow), tomorrow); got.State != VerdictAllowed {
		t.Fatalf("verdict %q (%s) the day after, want allowed", got.State, got.Reason)
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
