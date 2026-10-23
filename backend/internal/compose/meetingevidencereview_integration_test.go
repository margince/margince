// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestStoredCalendarRepairRecoversAliasAttendanceWithoutPromotingTheRecorder(t *testing.T) {
	e := integration.Setup(t)
	seedCalendarConnection(t, e)
	at := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	present := seedCapturedMeeting(t, e, "legacy-attendance", at, attendee{email: backfillOwner, response: "accepted"})
	forwarded := seedCapturedMeeting(t, e, "legacy-forward", at.Add(2*time.Hour), attendee{email: "somebody@example.test", response: "accepted"})
	purged := seedCapturedMeeting(t, e, "legacy-purged", at.Add(3*time.Hour), attendee{email: backfillOwner, response: "accepted"})
	for _, id := range []ids.UUID{present, forwarded, purged} {
		e.WsExec(t, `UPDATE activity SET host_user_id=$2,raw_capture_id=NULL WHERE id=$1`, id, e.AdminUser)
		e.WsExec(t, `INSERT INTO activity_participant(activity_id,user_id,role) VALUES($1,$2,'from') ON CONFLICT DO NOTHING`, id, e.AdminUser)
		e.WsExec(t, `INSERT INTO activity_meeting_attendee_repair(activity_id,outcome,rule_version) VALUES($1,'attendees',1)`, id)
	}
	// Retention and privacy can remove original bytes and import membership.
	e.WsExec(t, `DELETE FROM capture_import WHERE activity_id=ANY($1)`, []ids.UUID{forwarded, purged})
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return capture.NewPendingStore(e.DB()).PurgeRawCaptureTx(e.Admin(), tx, []ids.UUID{purged})
	}); err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.AdminUser)
	assertPersonalMeetingIDs(t, e, owner, nil)
	if count, err := repairMeetingAttendeesBatch(e.Admin(), e.Pool, 20, slog.Default()); err != nil || count != 2 {
		t.Fatalf("repair=%d,error=%v", count, err)
	}
	assertPersonalMeetingIDs(t, e, owner, []ids.UUID{present})
	if count, err := repairMeetingAttendeesBatch(e.Admin(), e.Pool, 20, slog.Default()); err != nil || count != 0 {
		t.Fatalf("replayed repair=%d,error=%v", count, err)
	}
	row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](present), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if row.HostUserId != nil {
		t.Fatal("alias attendee became organizer")
	}
	unassigned, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{UnhostedMeetings: true, MeetingWorkOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(unassigned) != 0 {
		t.Fatalf("assigned or unsupported captured work is unassigned: %d", len(unassigned))
	}
	free, _, err := e.Activities.Availability(e.Admin(), owner, at, at.Add(time.Hour), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(free) != 0 {
		t.Fatal("attendee's occupied meeting time offered as free")
	}
	committed, err := (weeklyPlanCapacity{pool: e.Pool}).ForWeek(e.Admin(), e.AdminUser, at)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Meetings != 1 {
		t.Fatalf("weekly calendar count=%d,want1", committed.Meetings)
	}
}

func assertPersonalMeetingIDs(t *testing.T, e *integration.Env, owner ids.UserID, want []ids.UUID) {
	t.Helper()
	rows, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{OnMeetingOf: &owner})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(want) {
		t.Fatalf("personal meetings=%d,want%d", len(rows), len(want))
	}
	for _, row := range rows {
		found := false
		for _, id := range want {
			if ids.UUID(row.Id) == id {
				found = true
			}
		}
		if !found {
			t.Fatal("unsupported personal meeting")
		}
	}
}

func TestAWithheldCalendarCopyFromAnUnknownProviderDoesNotProveAttendance(t *testing.T) {
	e := integration.Setup(t)
	ref, err := capture.NewSink(e.DB()).Upsert(connectorOwnerCtx(e, e.AdminUser, "future-calendar"), connector.NormalizedRecord{
		EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: "future-calendar", SourceID: "private-forward"},
		Fields: capture.ActivityFields{Kind: "meeting", Subject: "Private calendar copy", OccurredAt: time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)},
		Source: "future-calendar:private-forward", CapturedBy: "connector:future-calendar",
	})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE activity SET host_user_id=$2,raw_capture_id=NULL WHERE id=$1`, ref.ID, e.AdminUser)
	e.WsExec(t, `INSERT INTO activity_participant(activity_id,user_id,role) VALUES($1,$2,'from') ON CONFLICT DO NOTHING`, ref.ID, e.AdminUser)
	e.WsExec(t, `DELETE FROM capture_import WHERE activity_id=$1`, ref.ID)
	assertPersonalMeetingIDs(t, e, ids.From[ids.UserKind](e.AdminUser), nil)
	row, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](ref.ID), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if row.HostUserId != nil || (row.ColleagueIds != nil && len(*row.ColleagueIds) > 0) {
		t.Fatal("withheld original made recorder attendance authoritative")
	}
}
