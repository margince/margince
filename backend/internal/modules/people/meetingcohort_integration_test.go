// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// A meeting reaching the person who attended it, when the person arrived after
// the meeting did.
//
// The repair's mail arm cannot do this and never could: it finds work by
// counterparty_email, and a calendar meeting has none — attendance is a LIST, so
// the mapper leaves the field unset and the only thing naming an attendee is a
// participant row. For a while that difference was invisible, because a meeting
// usually has mail about it and the page reads the mail instead. A meeting
// nobody emailed about was reachable from nowhere.
//
// So these tests seed a meeting the way a calendar connector writes one — no
// counterparty, attendees by address — and assert the repair files it anyway.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedCapturedMeeting writes one connector-captured meeting the way a calendar
// sync writes it before any attendee is a contact: NO counterparty_email, an
// address-only participant row per attendee, and no link.
func (e *dedupeEnv) seedCapturedMeeting(
	ctx context.Context, t *testing.T, attendees ...string,
) ids.ActivityID {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, direction,
			                      source_system, source_id, source, captured_by)
			VALUES ($1, 'meeting', 'coffee', 'inbound', 'gcal', $2, 'gcal:seed', 'connector:gcal')`,
			activityID, activityID.String()); err != nil {
			return err
		}
		for _, attendee := range attendees {
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_participant (id, activity_id, address, role)
				VALUES ($1, $2, $3, 'attendee')`, ids.NewV7(), activityID, attendee); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return activityID
}

// The case from the field: a meeting synced, the attendee became a contact
// later, and the meeting has to arrive on their record.
func TestTheRepairFilesAMeetingUnderAnAttendeeWhoArrivedLater(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	const email = "attendee@meeting.test"
	meeting := e.seedCapturedMeeting(ctx, t, email)

	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, email, "Late Attendee", "meeting.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	// The sweep must OFFER them. A repair nothing selects is a repair that
	// never runs, and the job reports a drained backlog either way.
	owed, err := e.store.PeopleOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatalf("listing the owed: %v", err)
	}
	if !containsPerson(owed, res.PersonID) {
		t.Fatalf("the sweep does not see %s, who attended a meeting they are not filed under", res.PersonID)
	}

	if _, err := e.store.RepairPersonCohort(ctx, res.PersonID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if linked, named := e.cohortStateOf(ctx, t, meeting, res.PersonID); !linked || !named {
		t.Errorf("after the repair: linked=%v named=%v, want both — a meeting whose attendee "+
			"is now a contact must reach their page, and the participant row alone does not do that",
			linked, named)
	}

	// And then nothing, or the sweep never drains.
	after, err := e.store.PeopleOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatalf("listing the owed again: %v", err)
	}
	if containsPerson(after, res.PersonID) {
		t.Errorf("%s is still owed a repair after one ran — the selector asks a different "+
			"question than the write answers, so every tick redoes this work", res.PersonID)
	}
}

// Two attendees, two links. The mail arm refuses an activity that any person is
// already linked to, because attaching mail to a second party would relabel
// somebody's message. A meeting is the opposite shape: everyone in it belongs on
// it, so the guard has to be per-person or the second attendee is locked out by
// the first.
func TestAMeetingReachesEveryAttendeeNotOnlyTheFirst(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	const first, second = "one@meeting.test", "two@meeting.test"
	meeting := e.seedCapturedMeeting(ctx, t, first, second)

	one, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, first, "Attendee One", "meeting.test"))
	if err != nil {
		t.Fatalf("ensure one: %v", err)
	}
	two, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, second, "Attendee Two", "meeting.test"))
	if err != nil {
		t.Fatalf("ensure two: %v", err)
	}
	if _, err := e.store.RepairPersonCohort(ctx, one.PersonID); err != nil {
		t.Fatalf("repair one: %v", err)
	}
	if _, err := e.store.RepairPersonCohort(ctx, two.PersonID); err != nil {
		t.Fatalf("repair two: %v", err)
	}

	if linked, _ := e.cohortStateOf(ctx, t, meeting, one.PersonID); !linked {
		t.Errorf("the first attendee is not filed under the meeting they attended")
	}
	if linked, _ := e.cohortStateOf(ctx, t, meeting, two.PersonID); !linked {
		t.Errorf("the second attendee is not filed under the meeting — a guard that refuses an " +
			"activity already linked to anyone reads a meeting as taken by whoever was repaired first")
	}
}
