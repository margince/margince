// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A meeting that did not happen is not contact.
//
// Every instant here is relative to one pinned moment.
//
// Nothing under test reads the clock. The completion and the reopen compare an
// activity against the reminder's anchor.
//
// A fixture reading the clock would only move the dates in a failure
// message.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// pinnedQuiet is the instant the records in this file last heard from anybody.
// Every later fixture time is an offset from it.
var pinnedQuiet = time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)

// A booked meeting answers a quiet reminder; called off, the reminder reopens.
//
// The path in the order it happens. A record goes quiet, a reminder reports
// it, a meeting logged while booked closes it, and the meeting is called off.
// Nobody was ever in touch.
//
// Reopening rather than re-raising, because the scan cannot raise a
// replacement. Its anchor falls back to the older activity, and a run for that
// anchor already exists.
func TestACalledOffMeetingReopensTheQuietReminderItAnswered(t *testing.T) {
	e := setupRecency(t)
	quietSince := pinnedQuiet
	deal := e.seedQuietDeal(t, quietSince)
	reminder := e.seedQuietReminder(t, deal, quietSince)

	// A meeting logged while still booked is contact, so it closes the
	// reminder. This is the behaviour the fix must keep.
	meeting := e.seedMeeting(t, deal, "booked", pinnedQuiet.AddDate(0, 0, 89))
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	closed, err := store.CompleteQuietRemindersReachedBy(e.asResolver(), ids.From[ids.ActivityKind](meeting))
	if err != nil {
		t.Fatalf("completing the reminder a booked meeting answered: %v", err)
	}
	if closed != 1 {
		t.Fatalf("a booked meeting closed %d reminders, want 1 — the rest of this "+
			"test would then prove nothing about reopening", closed)
	}
	if !e.reminderIsDone(t, reminder) {
		t.Fatal("the reminder is open after the completion reported closing it")
	}

	e.exec(t, `UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`, meeting)

	reopened, err := store.ReopenQuietRemindersStillQuietAround(e.asResolver(), ids.From[ids.ActivityKind](meeting))
	if err != nil {
		t.Fatalf("reopening after the cancellation: %v", err)
	}
	if reopened != 1 {
		t.Errorf("a called-off meeting reopened %d reminders, want 1", reopened)
	}
	if e.reminderIsDone(t, reminder) {
		t.Error("the reminder is still closed, so the record reads as touched by a " +
			"meeting that never happened")
	}
}

// A record touched for real since keeps its reminder closed.
//
// The admitting half. A reopen that asked only whether a meeting was called
// off would re-raise a reminder the rep has since answered.
//
// The assertion above would pass against it.
func TestACalledOffMeetingLeavesAReminderAnsweredSinceAlone(t *testing.T) {
	e := setupRecency(t)
	quietSince := pinnedQuiet
	deal := e.seedQuietDeal(t, quietSince)
	reminder := e.seedQuietReminder(t, deal, quietSince)

	meeting := e.seedMeeting(t, deal, "booked", pinnedQuiet.AddDate(0, 0, 88))
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	if _, err := store.CompleteQuietRemindersReachedBy(e.asResolver(), ids.From[ids.ActivityKind](meeting)); err != nil {
		t.Fatalf("completing the reminder: %v", err)
	}

	// A call, after the meeting and after the anchor.
	e.seedCall(t, deal, pinnedQuiet.AddDate(0, 0, 89).Add(time.Hour))
	e.exec(t, `UPDATE activity SET meeting_status = 'no_show' WHERE id = $1`, meeting)

	reopened, err := store.ReopenQuietRemindersStillQuietAround(e.asResolver(), ids.From[ids.ActivityKind](meeting))
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if reopened != 0 {
		t.Errorf("a no-show reopened %d reminders on a record touched since, want 0", reopened)
	}
	if !e.reminderIsDone(t, reminder) {
		t.Error("a reminder the rep answered for real was reopened by an unrelated no-show")
	}
}

// A called-off meeting does not answer a quiet reminder in the first place.
//
// The resolver and the scan read one predicate, so this is the same guard the
// scan's anchor uses, asked where it is cheapest to see.
func TestACalledOffMeetingAnswersNoQuietReminder(t *testing.T) {
	e := setupRecency(t)
	quietSince := pinnedQuiet
	deal := e.seedQuietDeal(t, quietSince)
	reminder := e.seedQuietReminder(t, deal, quietSince)

	meeting := e.seedMeeting(t, deal, "canceled", pinnedQuiet.AddDate(0, 0, 89))
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	closed, err := store.CompleteQuietRemindersReachedBy(e.asResolver(), ids.From[ids.ActivityKind](meeting))
	if err != nil {
		t.Fatalf("asking whether a called-off meeting answers the reminder: %v", err)
	}
	if closed != 0 {
		t.Errorf("a called-off meeting closed %d reminders, want 0", closed)
	}
	if e.reminderIsDone(t, reminder) {
		t.Error("a meeting that was called off before anyone attended closed the reminder")
	}
}

// Mail from a sending system is not contact, so it ends no silence, and
// neither does an invitation: the meeting it books is the touch. The same mail
// from a human sender does end it.
func TestMailFromASendingSystemAnswersNoQuietReminder(t *testing.T) {
	for _, test := range []struct {
		name    string
		machine bool
		method  *string
		closed  int
	}{
		{"a file-share notice", true, nil, 0},
		{"an invitation", false, new("REQUEST"), 0},
		{"a human reply", false, nil, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := setupRecency(t)
			deal := e.seedQuietDeal(t, pinnedQuiet)
			reminder := e.seedQuietReminder(t, deal, pinnedQuiet)
			mail := ids.NewV7()
			e.exec(t, `INSERT INTO activity
				  (id, kind, direction, subject, occurred_at, audience, source, captured_by, origin, sender_is_machine, calendar_method)
				VALUES ($1, 'email', 'inbound', 'Shared with you', $2, 'workspace', 'gmail', 'connector:gmail', 'human', $3, $4)`,
				mail, pinnedQuiet.AddDate(0, 0, 89), test.machine, test.method)
			e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
				VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), mail, deal)

			store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
			closed, err := store.CompleteQuietRemindersReachedBy(e.asResolver(), ids.From[ids.ActivityKind](mail))
			if err != nil {
				t.Fatal(err)
			}
			if closed != test.closed || e.reminderIsDone(t, reminder) != (test.closed == 1) {
				t.Errorf("closed %d reminders, want %d", closed, test.closed)
			}
		})
	}
}

// seedQuietReminder files the reminder the time scan would have filed, with
// the anchor it would have carried: the record's last touch.
func (e *recencyEnv) seedQuietReminder(t *testing.T, deal ids.UUID, anchor time.Time) ids.UUID {
	t.Helper()
	reminder := ids.NewV7()
	// The key the automation mints, separator and all. The resolver reads the
	// anchor back out of it.
	//
	// A key in any other shape parses as no anchor at all, which makes every
	// older activity count as a later touch.
	sourceID := "no_activity_reminder:deal:" + deal.String() +
		provenance.ReminderAnchorSeparator + anchor.UTC().Format(time.RFC3339Nano)
	e.exec(t, `INSERT INTO activity
		  (id, kind, subject, occurred_at, audience, source, captured_by,
		   source_system, source_id, is_done, origin)
		VALUES ($1, 'task', 'No activity for a while', now(), 'workspace', 'system',
		        'system:time-scan', 'no_activity_reminder', $2, false, 'system_remediation')`,
		reminder, sourceID)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), reminder, deal)
	return reminder
}

// seedMeeting files one meeting in a named status, linked to the deal.
func (e *recencyEnv) seedMeeting(t *testing.T, deal ids.UUID, status string, at time.Time) ids.UUID {
	t.Helper()
	meeting := ids.NewV7()
	e.exec(t, `INSERT INTO activity
		  (id, kind, subject, occurred_at, audience, source, captured_by, meeting_status, origin)
		VALUES ($1, 'meeting', 'Intro call', $2, 'workspace', 'manual', 'human:rep', $3, 'human')`,
		meeting, at, status)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), meeting, deal)
	return meeting
}

// seedCall files a human call, which is contact.
func (e *recencyEnv) seedCall(t *testing.T, deal ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	call := ids.NewV7()
	e.exec(t, `INSERT INTO activity
		  (id, kind, subject, occurred_at, audience, source, captured_by, origin)
		VALUES ($1, 'call', 'Spoke to the buyer', $2, 'workspace', 'manual', 'human:rep', 'human')`,
		call, at)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), call, deal)
	return call
}

func (e *recencyEnv) reminderIsDone(t *testing.T, reminder ids.UUID) bool {
	t.Helper()
	var done bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT is_done FROM activity WHERE id = $1`, reminder).Scan(&done); err != nil {
		t.Fatalf("reading the reminder's state: %v", err)
	}
	return done
}

// asResolver is the engine's own posture: it completes and reopens system
// tasks, so it holds activity:update where recencyEnv.as() holds only create.
//
// A context of its own rather than a wider shared one. The tests that share
// as() assert what a create-only caller can do, and a grant added there is one
// their assertions stop noticing.
func (e *recencyEnv) asResolver() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"manager"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true, Update: true},
				"deal":     {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}
