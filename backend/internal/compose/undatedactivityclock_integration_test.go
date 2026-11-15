// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An activity logged without a date has happened for every reader at once,
// whichever clock the reader judges by. The app and the database keep their
// own clocks, and they disagree. Here the activities store and the transcript
// reader run on an injected clock that plays the app host's. It is an hour
// away from the database's now(), in either direction. A capture takes the
// process clock, so its tests move the pool's now() instead.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// appClock is a clock shift away from the database's, read once from it. The
// skew stays exact whatever offset the fixture lane gives the test's clock.
func appClock(t *testing.T, e *integration.Env, shift time.Duration) func() time.Time {
	t.Helper()
	var dbNow time.Time
	if err := e.Pool.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&dbNow); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	read := time.Now()
	return func() time.Time { return dbNow.Add(shift + time.Since(read)) }
}

// A meeting logged without a date and read at once is a held meeting, even when
// the app's clock is behind the database's. Stamped with the database's now()
// alone, it was in the reader's future and the promise was withheld as
// "meeting not yet held".
func TestAnUndatedMeetingIsHeldForAReaderWhoseClockIsBehindTheDatabase(t *testing.T) {
	e := setupTranscript(t)
	behind := appClock(t, e.Env, -time.Hour)
	e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
	body, source := "Mira: Priya Raman agreed to send the pricing by Friday.", "transcript"
	meeting, _, err := e.Activities.WithClock(behind).LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Body: &body, SourceSystem: &source, Source: "manual",
		AttendeeUserIDs: []ids.UserID{ids.From[ids.UserKind](e.Rep2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	reply, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": "Send pricing", "owner": "Priya Raman", "promiser_ref": "seat:" + e.Rep2.String(),
		"source_lines": []int{1}, "due_date": "", "confidence": 0.95, "commitment": openCommitmentMeaning("", ""),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	done := e.readAt(t, cannedBrain{reply: string(reply)}, behind)
	if len(done.ProposalIDs) != 1 {
		t.Fatalf("an undated meeting read by a clock behind the database's staged %d proposals, want one: %v",
			len(done.ProposalIDs), deref(done.StatusDetail))
	}
}

// An undated inbound message is on the waiting list the moment it is logged,
// even when the app's clock is behind the database's.
func TestAnUndatedInboundMessageWaitsForAReplyAtOnceWhenTheAppClockIsBehind(t *testing.T) {
	e := setupTranscript(t)
	store := e.Activities.WithClock(appClock(t, e.Env, -time.Hour))
	contact := e.SeedContact(t, "Wanda Waiting", &e.Rep1)
	subject, body := "Contract?", "Where is the contract"
	mail, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Direction: new("inbound"), Subject: &subject, Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entity := "contact"
	rows, _, err := store.ListActivities(e.Admin(), activities.ListActivitiesInput{
		WaitingReply: true, EntityType: &entity, EntityID: &contact,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Id == mail.Id {
			return
		}
	}
	t.Fatalf("an undated inbound message is missing from the waiting list read by a clock behind the database's (%d rows)", len(rows))
}

// An undated activity counts as the contact's last activity at once, even when
// the app's clock is ahead of the database's. The last-activity clock counts
// only occurred_at <= now(), so a row stamped with the app's clock alone would
// not count until the database caught up.
func TestAnUndatedActivityIsTheLastActivityAtOnceWhenTheAppClockIsAhead(t *testing.T) {
	e := setupTranscript(t)
	contact := e.SeedContact(t, "Arno Ahead", &e.Rep1)
	subject := "Call notes"
	note, _, err := e.Activities.WithClock(appClock(t, e.Env, time.Hour)).LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	read, err := e.Contacts.GetContact(e.Admin(), ids.From[ids.ContactKind](contact), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.LastActivityAt == nil || !read.LastActivityAt.Equal(note.OccurredAt) {
		t.Fatalf("the contact's last activity is %v, want the undated note at %v", read.LastActivityAt, note.OccurredAt)
	}
}

// A capture without a usable timestamp has happened for the database's clock
// and the app's, whichever is ahead. A replay keeps its time.
func TestAnUndatedCaptureHasHappenedForTheDatabaseAndTheApp(t *testing.T) {
	for name, shift := range map[string]time.Duration{"database behind": -time.Hour, "database ahead": time.Hour} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			pool := shiftedPool(t, shift)
			sink := undatedCaptureSink(e, pool)
			ctx := e.ConnectorOwnerCtx(e.Rep1, calendarSystem)
			first, err := sink.Upsert(ctx, undatedCapture("undated-capture"))
			if err != nil {
				t.Fatal(err)
			}
			app := time.Now()
			var db time.Time
			if err := pool.QueryRow(ctx, `SELECT now()`).Scan(&db); err != nil {
				t.Fatal(err)
			}
			stored := capturedOccurredAt(t, e, first.ID)
			if stored.After(db) || stored.After(app) {
				t.Fatalf("the undated capture is dated %v, after the database's now() %v or the app's %v", stored, db, app)
			}
			again, err := sink.Upsert(ctx, undatedCapture("undated-capture"))
			if err != nil {
				t.Fatal(err)
			}
			if again.ID != first.ID || !capturedOccurredAt(t, e, again.ID).Equal(stored) {
				t.Fatalf("the replay landed %v at %v, want %v at %v", again.ID, capturedOccurredAt(t, e, again.ID), first.ID, stored)
			}
		})
	}
}

func undatedCaptureSink(e *integration.Env, pool *pgxpool.Pool) *capture.Sink {
	return capture.NewSink(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.WS)))
}

// undatedCapture is a calendar event whose provider sent no start time.
func undatedCapture(event string) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: calendarSystem, SourceID: event},
		Fields: capture.ActivityFields{Kind: "meeting", Subject: "Undated review"},
		Source: calendarSystem + ":" + event, CapturedBy: "connector:" + calendarSystem,
		Raw: []byte(`{"id":"` + event + `"}`),
	}
}

func capturedOccurredAt(t *testing.T, e *integration.Env, id ids.UUID) time.Time {
	t.Helper()
	var at time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT occurred_at FROM activity WHERE id = $1`, id).Scan(&at)
	}); err != nil {
		t.Fatal(err)
	}
	return at
}
