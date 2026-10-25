// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// How a date-only activity is written, read back and patched.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastDay is the date the web form sends for a day with no time, at noon UTC.
var pastDay = time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

func zoneNamed(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return loc
}

// zonedStore is the store with its record zone fixed, as compose binds it.
func (e *sendEnv) zonedStore(zone *time.Location) *Store {
	return e.store(nil).WithRecordZone(func(context.Context, pgx.Tx) (*time.Location, error) { return zone, nil })
}

// logDayMeeting logs a held meeting known only by the date at names.
func logDayMeeting(ctx context.Context, t *testing.T, store *Store, at time.Time) crmcontracts.Activity {
	t.Helper()
	subject := "Site visit"
	day := crmcontracts.OccurredPrecisionOccurredPrecisionDay
	held := crmcontracts.CreateActivityRequestMeetingStatusCreateActivityRequestMeetingStatusHeld
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: KindMeeting, Subject: &subject, OccurredAt: &at, OccurredPrecision: &day, MeetingStatus: &held, Source: "manual",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := store.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return activity
}

func patchActivity(ctx context.Context, t *testing.T, store *Store, id crmcontracts.Activity, in UpdateActivityInput) crmcontracts.Activity {
	t.Helper()
	out, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(id.Id)), in)
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	return out
}

// lastUpdatedPrecision is the occurred_precision the newest activity.updated
// event for the activity carries, "" when it names none.
func lastUpdatedPrecision(t *testing.T, e *sendEnv, id crmcontracts.Activity) string {
	t.Helper()
	var precision *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT envelope->'payload'->'changed_fields'->>'occurred_precision' FROM event_outbox
		  WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1::text
		  ORDER BY id DESC LIMIT 1`, ids.UUID(id.Id).String()).Scan(&precision); err != nil {
		t.Fatalf("reading the activity.updated event: %v", err)
	}
	if precision == nil {
		return ""
	}
	return *precision
}

// A day entry is stored at its date's noon in the record zone, whatever zone
// that is, and reads back as that date there.
func TestADayEntryIsStoredAtItsDatesNoonInTheRecordZone(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	date := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"Pacific/Auckland", "America/New_York", "Pacific/Pago_Pago"} {
		zone := zoneNamed(t, name)
		store := e.zonedStore(zone)
		meeting := logDayMeeting(ctx, t, store, pastDay)
		if want := occurredday.Anchor(date, zone); !meeting.OccurredAt.Equal(want) {
			t.Errorf("%s: stored at %v, want the date's noon %v", name, meeting.OccurredAt, want)
		}
		read, err := store.GetActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("GetActivity: %v", err)
		}
		if read.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionDay ||
			!occurredday.Date(read.OccurredAt, zone).Equal(date) {
			t.Errorf("%s: reads back %v as %q, want 2026-03-09 as a day", name, read.OccurredAt, read.OccurredPrecision)
		}
	}
}

// The writer's own offset names the date: late on 9 March in Honolulu is 10
// March in Berlin, and the entry still files under the 9th.
func TestTheWritersOffsetNamesTheDate(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	lateEvening := time.Date(2026, 3, 9, 23, 0, 0, 0, time.FixedZone("HST", -10*3600))
	meeting := logDayMeeting(ctx, t, e.zonedStore(zoneNamed(t, "Europe/Berlin")), lateEvening)
	if want := time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC); !meeting.OccurredAt.Equal(want) {
		t.Errorf("stored at %v, want Berlin's noon of 9 March %v", meeting.OccurredAt, want)
	}
}

// A store built without the record zone refuses a day rather than guessing.
func TestADayFromAStoreWithoutARecordZoneIsRefused(t *testing.T) {
	e := setupSend(t)
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "note", Subject: new("Met at the fair"), OccurredAt: &pastDay,
		OccurredPrecision: new(crmcontracts.OccurredPrecisionOccurredPrecisionDay), Source: "manual",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	if _, _, err := e.store(nil).LogActivity(e.as(principal.RowScopeAll), in); !errors.Is(err, errNoRecordZone) {
		t.Fatalf("err = %v, want errNoRecordZone", err)
	}
}

func TestAnActivityLoggedWithoutAPrecisionIsAnInstant(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	note := loggedNote(ctx, t, e)
	if note.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
		t.Errorf("occurred_precision = %q, want instant", note.OccurredPrecision)
	}
}

func TestADayWithNoDateIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	subject := "Call back"
	day := crmcontracts.OccurredPrecisionOccurredPrecisionDay
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "note", Subject: &subject, OccurredPrecision: &day, Source: "manual",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	_, _, err = e.zonedStore(time.UTC).LogActivity(ctx, in)
	if required, ok := errors.AsType[*RequiredFieldError](err); !ok || required.Field != "occurred_at" {
		t.Fatalf("err = %v, want occurred_at required", err)
	}
}

func TestAPatchedTimeAloneIsAnInstant(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.zonedStore(time.UTC)
	meeting := logDayMeeting(ctx, t, store, pastDay)
	moved := pastDay.Add(26 * time.Hour)

	out := patchActivity(ctx, t, store, meeting, UpdateActivityInput{OccurredAt: &moved})

	if out.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionInstant || !out.OccurredAt.Equal(moved) {
		t.Errorf("got %v as %q, want %v as an instant: a new time is a known time", out.OccurredAt, out.OccurredPrecision, moved)
	}
	before, after := auditImagesFor(t, e, ids.UUID(meeting.Id))
	if before[fieldOccurredPrecision] != PrecisionDay || after[fieldOccurredPrecision] != PrecisionInstant {
		t.Errorf("the audit says %v -> %v, want day -> instant",
			before[fieldOccurredPrecision], after[fieldOccurredPrecision])
	}
	if got := lastUpdatedPrecision(t, e, meeting); got != PrecisionInstant {
		t.Errorf("activity.updated carries occurred_precision %q, want instant", got)
	}
}

// A day sent alone keeps the date the stored time falls on in the record zone,
// and moves the time to that date's anchor.
func TestAPatchedPrecisionAloneAnchorsTheStoredDate(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	zone := zoneNamed(t, "Pacific/Auckland")
	store := e.zonedStore(zone)
	note := loggedNote(ctx, t, e)
	day := PrecisionDay

	out := patchActivity(ctx, t, store, note, UpdateActivityInput{OccurredPrecision: &day})

	want := occurredday.Anchor(occurredday.Date(note.OccurredAt, zone), zone)
	if out.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionDay || !out.OccurredAt.Equal(want) {
		t.Errorf("got %v as %q, want the stored date's anchor %v as a day", out.OccurredAt, out.OccurredPrecision, want)
	}
	before, after := auditImagesFor(t, e, ids.UUID(note.Id))
	if before[fieldOccurredPrecision] != PrecisionInstant || after[fieldOccurredPrecision] != PrecisionDay {
		t.Errorf("the audit says %v -> %v, want instant -> day",
			before[fieldOccurredPrecision], after[fieldOccurredPrecision])
	}
	if got := lastUpdatedPrecision(t, e, note); got != PrecisionDay {
		t.Errorf("activity.updated carries occurred_precision %q, want day", got)
	}
}

func TestAPatchedTimeAndDayMoveTogether(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	zone := zoneNamed(t, "America/New_York")
	store := e.zonedStore(zone)
	meeting := logDayMeeting(ctx, t, store, pastDay)
	moved, day := pastDay.Add(48*time.Hour), PrecisionDay

	out := patchActivity(ctx, t, store, meeting, UpdateActivityInput{OccurredAt: &moved, OccurredPrecision: &day})

	want := occurredday.Anchor(moved, zone)
	if out.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionDay || !out.OccurredAt.Equal(want) {
		t.Errorf("got %v at %q, want %v at day", out.OccurredAt, out.OccurredPrecision, want)
	}
	if got := lastUpdatedPrecision(t, e, meeting); got != PrecisionDay {
		t.Errorf("activity.updated carries occurred_precision %q, want day", got)
	}
}

func TestAPrecisionOutsideTheListIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	note := loggedNote(ctx, t, e)
	month := "month"
	_, err := e.zonedStore(time.UTC).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(note.Id)),
		UpdateActivityInput{OccurredPrecision: &month})
	if _, ok := errors.AsType[*InvalidPrecisionError](err); !ok {
		t.Fatalf("err = %v, want InvalidPrecisionError", err)
	}
}

// A meeting holding its host's slot is what the overlap guard compares hours
// on, so neither door may make it a day.
func TestAMeetingHoldingASlotCannotBeADay(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.zonedStore(time.UTC)
	subject, hour, at := "Booked call", 1800, pastDay
	booked := LogActivityInput{
		Kind: KindMeeting, Subject: &subject, OccurredAt: &at, DurationSeconds: &hour,
		ClaimsHostSlot: true, Source: "manual",
	}

	day := booked
	day.OccurredPrecision = PrecisionDay
	_, _, err := store.LogActivity(ctx, day)
	if _, ok := errors.AsType[*DayHeldSlotError](err); !ok {
		t.Fatalf("create err = %v, want DayHeldSlotError", err)
	}

	held, _, err := store.LogActivity(ctx, booked)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	precision := PrecisionDay
	_, err = store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(held.Id)),
		UpdateActivityInput{OccurredPrecision: &precision})
	if _, ok := errors.AsType[*DayHeldSlotError](err); !ok {
		t.Fatalf("update err = %v, want DayHeldSlotError", err)
	}
}

// A meeting an invitation went out for holds the host's hour as well. It is
// refused a day as a claimed slot is, before the invitation guard answers.
func TestAnInvitedMeetingCannotBeADay(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.zonedStore(time.UTC)
	subject := "Invited call"
	meeting, _, err := store.LogActivity(ctx, LogActivityInput{Kind: KindMeeting, Subject: &subject, OccurredAt: &pastDay, Source: "manual"})
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	// The invitation is a fixture: sending a real one needs a connected calendar.
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO meeting_invitation (activity_id, host_user_id, provider, calendar_id, appointment, status, management_hash, management_ref)
		VALUES ($1, $2, 'gcal', 'primary', '{}', 'canceled', $3, 'ref')`, ids.UUID(meeting.Id), e.rep, ids.NewV7().String()); err != nil {
		t.Fatalf("seeding the invitation: %v", err)
	}
	precision := PrecisionDay
	_, err = store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), UpdateActivityInput{OccurredPrecision: &precision})
	if _, ok := errors.AsType[*DayHeldSlotError](err); !ok {
		t.Fatalf("err = %v, want DayHeldSlotError", err)
	}
}
