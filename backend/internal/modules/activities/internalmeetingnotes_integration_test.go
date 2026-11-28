// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func capturedMeetingForNotes(t *testing.T, e *sendEnv, customer ids.UUID) ids.ActivityID {
	t.Helper()
	id := ids.From[ids.ActivityKind](ids.NewV7())
	e.exec(t, `INSERT INTO activity(id,kind,subject,body,source,source_system,captured_by,host_user_id,occurred_at,meeting_status,audience)
 VALUES($1,'meeting','Customer review','Provider agenda','gcal','gcal','connector:gcal',$2,$3,'held','workspace')`, id.UUID, e.rep, clocktest.Now(t).Add(-time.Hour))
	e.exec(t, `INSERT INTO activity_link(activity_id,entity_type,contact_id) VALUES($1,'contact',$2)`, id.UUID, customer)
	return id
}

func TestInternalMeetingNotesStaySeparateFromCalendarText(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	id := capturedMeetingForNotes(t, e, customer)
	store, ctx := meetingStore(e), meetingCtx(e)
	out, err := store.UpdateActivity(ctx, id, UpdateActivityInput{MeetingNotes: new("Buyer approved renewal")})
	if err != nil {
		t.Fatal(err)
	}
	if out.Body == nil || *out.Body != "Provider agenda" || out.MeetingNotes == nil || *out.MeetingNotes != "Buyer approved renewal" {
		t.Fatalf("notes overwrote the calendar: %+v", out)
	}
	var ready bool
	if err := e.owner.QueryRow(context.Background(), `SELECT coalesce((envelope->'payload'->'changed_fields'->>'meeting_recap_became_ready')::boolean,false)
 FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=$1::text ORDER BY id DESC LIMIT 1`, id.String()).Scan(&ready); err != nil || !ready {
		t.Fatalf("first rep notes did not announce recap readiness: %v", err)
	}
	// Sync owns the description and leaves the rep's notes alone.
	e.exec(t, `UPDATE activity SET body='Updated provider agenda' WHERE id=$1`, id.UUID)
	reread, err := store.GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil || reread.MeetingNotes == nil || *reread.MeetingNotes != "Buyer approved renewal" {
		t.Fatalf("notes after sync = %v, %v", reread.MeetingNotes, err)
	}
	cleared, err := store.UpdateActivity(ctx, id, UpdateActivityInput{Clear: []string{fieldMeetingNotes}})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.MeetingNotes == nil || *cleared.MeetingNotes != "" || meetingReadyForRecap(cleared) {
		t.Fatalf("cleared notes exposed the provider description: %+v", cleared)
	}
	var events int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_id=$1`, id.UUID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events < 2 {
		t.Fatalf("only %d audit entries for two edits", events)
	}
}

func TestManualMeetingNotesKeepLegacyClientsConsistent(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	held := "held"
	id := customerMeeting(t, e, customer, "Customer review", clocktest.Now(t).Add(-time.Hour), &held)
	store, ctx := meetingStore(e), meetingCtx(e)
	out, err := store.UpdateActivity(ctx, id, UpdateActivityInput{MeetingNotes: new("First notes")})
	if err != nil {
		t.Fatal(err)
	}
	if out.Body == nil || *out.Body != "First notes" {
		t.Fatalf("legacy body = %v", out.Body)
	}
	out, err = store.UpdateActivity(ctx, id, UpdateActivityInput{Body: new("Old client edit")})
	if err != nil {
		t.Fatal(err)
	}
	if out.MeetingNotes == nil || *out.MeetingNotes != "Old client edit" {
		t.Fatalf("new notes = %v", out.MeetingNotes)
	}
}

func TestMeetingRecordBodyDoesNotFallBackAfterAnExplicitClear(t *testing.T) {
	meeting := crmcontracts.Activity{Kind: crmcontracts.ActivityKindMeeting, Body: new("Provider description"), MeetingNotes: new("")}
	if got := MeetingRecordBody(meeting); got == nil || *got != "" {
		t.Fatalf("record body = %v", got)
	}
}

func TestEarlierCalendarBodyNotesResolveThroughTheirRepEdit(t *testing.T) {
	e := setupSend(t)
	id := capturedMeetingForNotes(t, e, seedCustomer(t, e))
	store, ctx := meetingStore(e), meetingCtx(e)
	before, err := store.GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil || before.MeetingNotes != nil || MeetingRecordBody(before) != nil {
		t.Fatalf("provider agenda became rep notes: %+v, %v", before, err)
	}
	if _, err := store.UpdateActivity(ctx, id, UpdateActivityInput{Body: new("Earlier rep notes")}); err != nil {
		t.Fatal(err)
	}
	earlier, err := store.GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil || earlier.MeetingNotes == nil || *earlier.MeetingNotes != "Earlier rep notes" {
		t.Fatalf("earlier notes missing from API: %+v, %v", earlier, err)
	}
	if _, err := store.UpdateActivity(ctx, id, UpdateActivityInput{MeetingNotes: new("Earlier rep notes\n\nNew internal detail")}); err != nil {
		t.Fatal(err)
	}
	rows, _, err := store.ListActivities(ctx, ListActivitiesInput{Query: new("New internal detail")})
	if err != nil || len(rows) != 1 || ids.UUID(rows[0].Id) != id.UUID {
		t.Fatalf("notes search returned %d rows, %v", len(rows), err)
	}
}
