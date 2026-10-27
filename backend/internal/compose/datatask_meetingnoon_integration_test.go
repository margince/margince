// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The noon task puts back the start an old outcome save moved to noon. Each
// meeting is saved through the activity update the dialog called.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
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

// bookDay logs a booked meeting that has a date and no hour.
func (f noonFixture) bookDay(date time.Time, contact ids.ContactID) ids.ActivityID {
	f.t.Helper()
	subject, booked := "Noon fixture", string(crmcontracts.ActivityMeetingStatusBooked)
	meeting, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &date, Source: "manual",
		OccurredPrecision: activities.PrecisionDay, MeetingStatus: &booked,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}},
	})
	if err != nil {
		f.t.Fatalf("logging the day meeting: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
}

// cutoffNow marks the cutoff migration as run now, after the saves before it.
func (f noonFixture) cutoffNow() {
	f.t.Helper()
	if _, err := integration.OwnerConn(f.t).Exec(context.Background(), `UPDATE schema_migrations_core SET applied_at = now() WHERE version = $1`,
		activities.MeetingNoonCutoffMigration); err != nil {
		f.t.Fatalf("setting the cutoff: %v", err)
	}
}

// save is the outcome save the old dialog made: the status and a new start in one update.
func (f noonFixture) save(id ids.ActivityID, start time.Time) {
	f.t.Helper()
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), id, activities.UpdateActivityInput{MeetingStatus: &held, OccurredAt: &start}); err != nil {
		f.t.Fatalf("saving the outcome: %v", err)
	}
}

// move is a start change by whoever ctx is, with the status only when given.
func (f noonFixture) move(ctx context.Context, id ids.ActivityID, start time.Time, status *string) {
	f.t.Helper()
	if _, err := f.e.Activities.UpdateActivity(ctx, id, activities.UpdateActivityInput{MeetingStatus: status, OccurredAt: &start}); err != nil {
		f.t.Fatalf("moving the meeting: %v", err)
	}
}

func (f noonFixture) historyStart(id ids.ActivityID) string {
	f.t.Helper()
	return f.e.WsScalar(f.t, `SELECT to_char(scheduled_start AT TIME ZONE 'UTC', '`+noonStartLayout+`') FROM activity_meeting_history
		WHERE activity_id = $1 AND status = 'held'`, id.UUID)
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

	held := string(crmcontracts.ActivityMeetingStatusHeld)
	noStatusChange := f.book(bookedAtMorning, shared)
	f.move(e.Admin(), noStatusChange, berlinNoon, nil)
	backToNoon := f.book(bookedAtMorning, shared)
	f.save(backToNoon, berlinNoon)
	f.move(e.Admin(), backToNoon, afternoon, nil)
	f.move(e.Admin(), backToNoon, berlinNoon, nil)
	bySystem := f.book(bookedAtMorning, shared)
	f.move(ctx, bySystem, berlinNoon, &held)
	// Its stored noon UTC is 14:00 in Berlin, but a day meeting has no start to put back.
	fromADay := f.bookDay(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), shared)
	f.save(fromADay, berlinNoon)
	if got := f.historyStart(damaged); got != "2026-07-15T10:00:00.000Z" {
		t.Fatalf("the history start before the repair = %s, want the noon of the save", got)
	}

	f.cutoffNow()
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
	if got := f.historyStart(damaged); got != "2026-07-15T07:00:00.123Z" {
		t.Errorf("the history start after the repair = %s, want the restored start", got)
	}
	for name, id := range map[string]ids.ActivityID{
		"a meeting booked at noon": reallyAtNoon, "a meeting after the cutoff": afterCutoff,
		"a noon move without an outcome": noStatusChange, "a colleague's later move back to noon": backToNoon,
		"a noon save by the system": bySystem, "a day meeting given a time": fromADay,
	} {
		if got := f.start(id); got != "2026-07-15T10:00:00.000Z" {
			t.Errorf("%s was moved to %s", name, got)
		}
	}
	if got := f.start(movedAfterwards); got != "2026-07-15T13:00:00.000Z" {
		t.Errorf("the meeting a colleague moved afterwards starts at %s, want their 13:00Z", got)
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

// A day meeting is stored at noon UTC. In a UTC installation, an outcome save
// that made a timed meeting a day meeting looks like the old dialog's noon
// move. The task must leave it alone.
func TestTheNoonTaskLeavesADayMeetingAtItsNoon(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb('UTC'::text) WHERE key = 'installation.timezone'`)
	f := noonFixture{t: t, e: e}
	anchor := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	madeADay := f.book(bookedAtMorning, f.contact("Day Contact"))
	held, day := string(crmcontracts.ActivityMeetingStatusHeld), activities.PrecisionDay
	if _, err := e.Activities.UpdateActivity(e.Admin(), madeADay, activities.UpdateActivityInput{
		MeetingStatus: &held, OccurredAt: &anchor, OccurredPrecision: &day,
	}); err != nil {
		t.Fatalf("saving the outcome as a day: %v", err)
	}
	f.cutoffNow()
	version := f.version(madeADay)

	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())

	if got := f.start(madeADay); got != "2026-07-15T12:00:00.000Z" {
		t.Errorf("the day meeting was moved to %s", got)
	}
	if got := e.WsScalar(t, `SELECT occurred_precision FROM activity WHERE id = $1`, madeADay.UUID); got != day {
		t.Errorf("the day meeting's precision became %s", got)
	}
	if got := f.version(madeADay); got != version {
		t.Errorf("the day meeting changed version to %d", got)
	}
}

// A meeting damaged while live and archived later is restored too, so a later
// restore of the record shows the meeting when it was. It stays archived.
func TestTheNoonTaskRestoresAMeetingArchivedAfterTheDamage(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	archived := f.book(bookedAtMorning, f.contact("Archived Contact"))
	f.save(archived, berlinNoon)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
		t.Fatalf("archiving the meeting: %v", err)
	}
	f.cutoffNow()
	version := f.version(archived)

	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())

	if got := f.version(archived); got != version+1 {
		t.Errorf("the restore left version %d, want %d", got, version+1)
	}
	if got, want := f.start(archived), "2026-07-15T07:00:00.123Z"; got != want {
		t.Errorf("the archived meeting starts at %s, want %s", got, want)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, archived.UUID); got != 1 {
		t.Error("the restore brought the archived meeting back")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'
		AND (after ->> 'occurred_at')::timestamptz = $2 AND actor_type = 'system'`, archived.UUID, bookedAtMorning); got != 1 {
		t.Errorf("system restore audit rows = %d, want 1", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox o
		JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE o.envelope->>'type' = 'activity.updated' AND o.envelope->'entity'->>'id' = $1
		  AND a.actor_type = 'system'
		  AND o.envelope->'payload'->'changed_fields'->>'occurred_precision' = 'instant'
		  AND a.before->>'occurred_precision' = 'instant' AND a.after->>'occurred_precision' = 'instant'`, archived.String()); got != 1 {
		t.Errorf("activity.updated events riding the restore's audit row, with the precision named = %d, want 1", got)
	}
	if got := f.historyStart(archived); got != "2026-07-15T07:00:00.123Z" {
		t.Errorf("the history start after the repair = %s, want the restored start", got)
	}
}

// The batch itself pages, replays from a nil cursor without writing again, and
// refuses a cursor it cannot read.
func TestTheNoonBatchPagesReplaysAndRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	contact := f.contact("Paging Contact")
	for range 3 {
		f.save(f.book(bookedAtMorning, contact), berlinNoon)
	}
	f.cutoffNow()
	step := meetingNoonStartsDataTask().Steps[0]
	pass := func() (changed, pages int) {
		t.Helper()
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		for {
			var out datatask.BatchOut
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				var err error
				out, err = step.Batch(ctx, tx, in)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			changed += out.Changed
			pages++
			if out.Done {
				return changed, pages
			}
			in.Cursor = out.Cursor
		}
	}
	if changed, pages := pass(); changed != 3 || pages < 3 {
		t.Fatalf("the first pass restored %d meetings over %d pages, want 3 over at least 3", changed, pages)
	}
	if changed, _ := pass(); changed != 0 {
		t.Errorf("a replay from a nil cursor restored %d more meetings", changed)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := step.Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"not an id"`), Limit: 10})
		return err
	}); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}

// A batch that fails after its writes takes the archived restore back with it.
func TestTheNoonRepairOfAnArchivedMeetingRollsBackWithItsBatch(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	archived := f.book(bookedAtMorning, f.contact("Rollback Contact"))
	f.save(archived, berlinNoon)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
		t.Fatal(err)
	}
	f.cutoffNow()
	task := meetingNoonStartsDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runMachineAddressTask(ctx, t, e, task)

	if got := f.start(archived); got != "2026-07-15T10:00:00.000Z" {
		t.Errorf("the restore survived a rolled-back batch: %s", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND actor_type = 'system'`, archived.UUID); got != 0 {
		t.Errorf("restore audit rows escaped the rollback: %d", got)
	}
}

// An erased invitation no longer holds its meeting. The noon save landed while
// the invitation was canceled. Erasing the guest then archived the meeting and
// erased the invitation, and the task still restores the start.
func TestTheNoonTaskRestoresAMeetingWhoseInvitationWasErased(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, noonZone)
	f := noonFixture{t: t, e: e}
	invitedAt := time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
	inviting := newInvitingStore(memberSeat(e, e.Rep1, principal.SeatFull), t, e, invitedAt.AddDate(0, 0, -2))
	guest := e.SeedContact(t, "Erased guest", nil)
	meeting := inviting.invite(t, guest, invitedAt)
	inviting.cancel(t, meeting)
	f.save(ids.From[ids.ActivityKind](meeting), berlinNoon)
	inviting.eraseGuest(t, e, guest, meeting)
	f.cutoffNow()

	runMachineAddressTask(ctx, t, e, meetingNoonStartsDataTask())

	if got, want := f.start(ids.From[ids.ActivityKind](meeting)), "2026-07-15T09:00:00.000Z"; got != want {
		t.Errorf("the meeting whose invitation was erased starts at %s, want %s", got, want)
	}
}
