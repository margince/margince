// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The noon task puts back the start an old outcome save moved to noon. Each
// meeting is saved through the activity update the dialog called.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	noonZone        = "Europe/Berlin"
	noonStartLayout = `YYYY-MM-DD"T"HH24:MI:SS.MS"Z"`
)

// bookedAtMorning is the start the meetings are booked at: 09:00:00.123 in
// Berlin on a summer day, which is 07:00:00.123 UTC.
var bookedAtMorning = time.Date(2026, 7, 15, 7, 0, 0, 123_000_000, time.UTC)

// berlinNoon is 12:00 on that same Berlin day.
var berlinNoon = time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)

type noonFixture struct {
	t *testing.T
	e *integration.Env
}

func (f noonFixture) contact(name string) ids.ContactID {
	f.t.Helper()
	created, err := f.e.Contacts.CreateContact(f.e.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual"})
	if err != nil {
		f.t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}

func (f noonFixture) lastActivity(contact ids.ContactID) string {
	f.t.Helper()
	return f.e.WsScalar(f.t, `SELECT to_char(last_activity_at AT TIME ZONE 'UTC', '`+noonStartLayout+`') FROM contact WHERE id = $1`, contact.UUID)
}

func (f noonFixture) book(start time.Time, contact ids.ContactID) ids.ActivityID {
	f.t.Helper()
	subject, booked := "Noon fixture", string(crmcontracts.ActivityMeetingStatusBooked)
	meeting, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start, Source: "manual",
		MeetingStatus: &booked, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}},
	})
	if err != nil {
		f.t.Fatalf("logging the meeting: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
}

// save is the outcome save the old dialog made: the status and a new start in one update.
func (f noonFixture) save(id ids.ActivityID, start time.Time) {
	f.t.Helper()
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), id, activities.UpdateActivityInput{MeetingStatus: &held, OccurredAt: &start}); err != nil {
		f.t.Fatalf("saving the outcome: %v", err)
	}
}

func (f noonFixture) start(id ids.ActivityID) string {
	f.t.Helper()
	return f.e.WsScalar(f.t, `SELECT to_char(occurred_at AT TIME ZONE 'UTC', '`+noonStartLayout+`') FROM activity WHERE id = $1`, id.UUID)
}

func (f noonFixture) version(id ids.ActivityID) int {
	f.t.Helper()
	return f.e.WsCount(f.t, `SELECT version FROM activity WHERE id = $1`, id.UUID)
}

func TestTheNoonTaskRestoresOnlyStartsAnOutcomeSaveMovedBeforeTheCutoff(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	shared, alone := f.contact("Shared Contact"), f.contact("Lone Contact")

	damaged := f.book(bookedAtMorning, alone)
	f.save(damaged, berlinNoon)
	reallyAtNoon := f.book(berlinNoon, shared)
	f.save(reallyAtNoon, berlinNoon)
	movedAfterwards := f.book(bookedAtMorning, shared)
	f.save(movedAfterwards, berlinNoon)
	afternoon := berlinNoon.Add(3 * time.Hour)
	f.save(movedAfterwards, afternoon)

	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `UPDATE schema_migrations_core SET applied_at = now() WHERE version = $1`,
		activities.MeetingNoonCutoffMigration); err != nil {
		t.Fatalf("setting the cutoff: %v", err)
	}
	afterCutoff := f.book(bookedAtMorning, shared)
	f.save(afterCutoff, berlinNoon)

	before := map[string]int{"damaged": f.version(damaged), "after": f.version(afterCutoff)}
	if got := f.lastActivity(alone); got != "2026-07-15T10:00:00.000Z" {
		t.Fatalf("the contact's last activity before the repair = %s, want the noon of the save", got)
	}

	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())

	if got, want := f.start(damaged), "2026-07-15T07:00:00.123Z"; got != want {
		t.Errorf("the damaged meeting starts at %s, want %s", got, want)
	}
	if got := f.version(damaged); got != before["damaged"]+1 {
		t.Errorf("the restore left version %d, want %d", got, before["damaged"]+1)
	}
	if got := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'
		AND (after ->> 'occurred_at')::timestamptz = $2 AND actor_type = 'system'`, damaged.UUID, bookedAtMorning); got != 1 {
		t.Errorf("system restore audit rows = %d, want 1", got)
	}
	for name, id := range map[string]ids.ActivityID{"a meeting booked at noon": reallyAtNoon, "a meeting after the cutoff": afterCutoff} {
		if got := f.start(id); got != "2026-07-15T10:00:00.000Z" {
			t.Errorf("%s was moved to %s", name, got)
		}
	}
	if got := f.start(movedAfterwards); got != "2026-07-15T13:00:00.000Z" {
		t.Errorf("the meeting a person moved afterwards starts at %s, want their 13:00Z", got)
	}
	if got := f.version(afterCutoff); got != before["after"] {
		t.Errorf("the meeting after the cutoff changed version to %d", got)
	}
	if got := f.lastActivity(alone); got != "2026-07-15T07:00:00.123Z" {
		t.Errorf("the contact's last activity after the repair = %s, want the restored start", got)
	}

	versions := f.version(damaged)
	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())
	if got := f.version(damaged); got != versions {
		t.Errorf("a second run changed the restored meeting to version %d", got)
	}
}
