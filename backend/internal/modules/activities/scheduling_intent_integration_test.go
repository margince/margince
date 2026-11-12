// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The chosen local start governs the booked instant, travels to the provider,
// and stays on the meeting until a reschedule names only an instant.
func TestABookedMeetingKeepsTheHostsChosenClockUntilAnInstantMovesIt(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	clock := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	calendar := &invitationCalendar{}
	store := e.store(nil).WithClock(func() time.Time { return clock }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider, profile.Enabled, profile.BufferMinutes = "gcal", true, 0
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	contact := ids.NewV7()
	args := schedulingArgs{}
	if _, err := e.owner.Exec(ctx, `INSERT INTO contact(id,full_name,source,captured_by)VALUES(`+args.add(contact)+`,'Guest','manual','human:test')`, args...); err != nil {
		t.Fatal(err)
	}
	// The request's own instants are an hour off: the chosen clock wins and the
	// meeting keeps its half hour.
	chosen := &crmcontracts.IntentTime{WallClock: "2026-10-05T11:00", Timezone: "Europe/Berlin"}
	in := crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Project meeting", Description: "Plan delivery.",
		Start: clock.Add(4 * time.Hour), End: clock.Add(4*time.Hour + 30*time.Minute), StartIntent: chosen,
	}
	invited, err := store.CreateInvitation(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	if !invited.Start.Equal(want) || !invited.End.Equal(want.Add(30*time.Minute)) || invited.StartIntent == nil || *invited.StartIntent != *chosen {
		t.Fatalf("booked = %+v", invited)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	if len(calendar.requests) != 1 || calendar.requests[0].StartZone != "Europe/Berlin" || calendar.requests[0].StartWall != chosen.WallClock {
		t.Fatalf("provider request = %+v", calendar.requests)
	}
	meetingID := ids.From[ids.ActivityKind](ids.UUID(invited.Id))
	meeting, err := store.GetActivity(ctx, meetingID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if meeting.MeetingIntent == nil || meeting.MeetingIntent.Timezone != "Europe/Berlin" || meeting.MeetingIntent.WallClock != "2026-10-05T11:00:00.000000" || !meeting.OccurredAt.Equal(want) {
		t.Fatalf("meeting = %+v intent=%+v", meeting.OccurredAt, meeting.MeetingIntent)
	}

	booked, err := store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil {
		t.Fatal(err)
	}
	later := &crmcontracts.IntentTime{WallClock: "2026-10-05T12:00", Timezone: "Europe/Berlin"}
	start, end := want.Add(time.Hour), want.Add(90*time.Minute)
	if _, err := store.ChangeInvitation(ctx, ids.UUID(invited.Id), crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: booked.Version, Start: &start, End: &end, StartIntent: later}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	moved, err := store.GetActivity(ctx, meetingID, storekit.LiveOnly)
	if err != nil || moved.MeetingIntent == nil || moved.MeetingIntent.WallClock != "2026-10-05T12:00:00.000000" || !moved.OccurredAt.Equal(start) {
		t.Fatalf("rescheduled meeting = %+v intent=%+v err=%v", moved.OccurredAt, moved.MeetingIntent, err)
	}

	booked, err = store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil {
		t.Fatal(err)
	}
	start, end = want.Add(2*time.Hour), want.Add(150*time.Minute)
	if _, err := store.ChangeInvitation(ctx, ids.UUID(invited.Id), crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: booked.Version, Start: &start, End: &end}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	instantOnly, err := store.GetActivity(ctx, meetingID, storekit.LiveOnly)
	if err != nil || instantOnly.MeetingIntent != nil || !instantOnly.OccurredAt.Equal(start) {
		t.Fatalf("an instant-only reschedule kept a clock: %+v intent=%+v err=%v", instantOnly.OccurredAt, instantOnly.MeetingIntent, err)
	}
	view, err := store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil || view.StartIntent != nil || calendar.requests[len(calendar.requests)-1].StartZone != "" {
		t.Fatalf("an instant-only reschedule kept a clock on the invitation: %+v err=%v", view, err)
	}
}

func TestOnlyAnInstantMeetingTakesAChosenLocalStart(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	chosen := &crmcontracts.IntentTime{WallClock: "2027-03-28T02:30", Timezone: "Europe/Berlin"}
	if _, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Body: new("Notes"), MeetingIntent: chosen}); err == nil {
		t.Fatal("a note took a meeting start")
	}
	if _, _, err := store.LogActivity(ctx, LogActivityInput{Kind: KindMeeting, Subject: new("Workshop"), OccurredPrecision: PrecisionDay, MeetingIntent: chosen}); err == nil {
		t.Fatal("a whole-day meeting took a local start")
	}
	meeting, _, err := store.LogActivity(ctx, LogActivityInput{Kind: KindMeeting, Subject: new("Workshop"), MeetingIntent: chosen})
	if err != nil {
		t.Fatal(err)
	}
	// 02:30 does not exist that night in Berlin: the gap moves to 03:00 CEST.
	if gap := time.Date(2027, 3, 28, 1, 0, 0, 0, time.UTC); !meeting.OccurredAt.Equal(gap) || meeting.MeetingIntent == nil {
		t.Fatalf("meeting = %v intent=%+v", meeting.OccurredAt, meeting.MeetingIntent)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	kept, err := store.UpdateActivity(ctx, id, UpdateActivityInput{Body: new("Agenda")})
	if err != nil || kept.MeetingIntent == nil {
		t.Fatalf("an edit to the words dropped the chosen start: %+v err=%v", kept.MeetingIntent, err)
	}
	day := PrecisionDay
	whole, err := store.UpdateActivity(ctx, id, UpdateActivityInput{OccurredAt: &meeting.OccurredAt, OccurredPrecision: &day})
	if err != nil || whole.MeetingIntent != nil {
		t.Fatalf("a whole-day meeting kept a local start: %+v err=%v", whole.MeetingIntent, err)
	}
	// Noon UTC is also the date's anchor, so going whole-day leaves the instant where it was.
	noon, _, err := store.LogActivity(ctx, LogActivityInput{Kind: KindMeeting, Subject: new("Lunch"), MeetingIntent: &crmcontracts.IntentTime{WallClock: "2027-06-01T14:00", Timezone: "Europe/Berlin"}})
	if err != nil {
		t.Fatal(err)
	}
	atAnchor, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(noon.Id)), UpdateActivityInput{OccurredAt: &noon.OccurredAt, OccurredPrecision: &day})
	if err != nil || atAnchor.MeetingIntent != nil || !atAnchor.OccurredAt.Equal(noon.OccurredAt) {
		t.Fatalf("a whole-day meeting at its anchor kept a local start: %+v err=%v", atAnchor.MeetingIntent, err)
	}
}

// A zone rule change leaves a cached instant that no longer matches the chosen
// clock. The read and the delivery both resolve the clock again.
func TestAnInvitationReadsAndDeliversItsChosenClockUnderCurrentRules(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	clock := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	calendar := &invitationCalendar{}
	store := e.store(nil).WithClock(func() time.Time { return clock }).WithWorkingHours(func(context.Context, ids.UserID) (WorkingHours, error) { return fallbackWorkingHours(), nil }).WithSchedulingCalendar(calendar).WithMeetingVault(keyvault.NewMemory()).WithPublicBaseURL("https://crm.example.com")
	profile := defaultSchedulingProfile()
	profile.Provider, profile.Enabled, profile.BufferMinutes = "gcal", true, 0
	if _, err := store.SaveSchedulingProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	contact := ids.NewV7()
	args := schedulingArgs{}
	if _, err := e.owner.Exec(ctx, `INSERT INTO contact(id,full_name,source,captured_by)VALUES(`+args.add(contact)+`,'Guest','manual','human:test')`, args...); err != nil {
		t.Fatal(err)
	}
	chosen := &crmcontracts.IntentTime{WallClock: "2026-10-05T11:00", Timezone: "Europe/Berlin"}
	want := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	invited, err := store.CreateInvitation(ctx, crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Project meeting", Description: "Plan delivery.",
		Start: want, End: want.Add(30 * time.Minute), StartIntent: chosen,
	})
	if err != nil {
		t.Fatal(err)
	}
	args = schedulingArgs{}
	if _, err := e.owner.Exec(ctx, `UPDATE meeting_invitation SET appointment = appointment
	 || jsonb_build_object('Start', `+args.add(want.Add(time.Hour))+`::timestamptz, 'End', `+args.add(want.Add(90*time.Minute))+`::timestamptz)
	 WHERE activity_id = `+args.add(ids.UUID(invited.Id)), args...); err != nil {
		t.Fatal(err)
	}
	view, err := store.Invitation(ctx, ids.UUID(invited.Id))
	if err != nil || !view.Start.Equal(want) || !view.End.Equal(want.Add(30*time.Minute)) {
		t.Fatalf("the read kept a stale start: %v-%v err=%v", view.Start, view.End, err)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	if len(calendar.requests) != 1 || !calendar.requests[0].Start.Equal(want) || !calendar.requests[0].End.Equal(want.Add(30*time.Minute)) {
		t.Fatalf("the delivery sent a stale start: %+v", calendar.requests)
	}
}
