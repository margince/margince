// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// How a date-only activity is written, read back and patched.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
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

// zonedStore is the store with the installation zone fixed, as compose binds it.
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

// A day entry is stored at noon UTC of its date, whatever the installation
// zone, and reads back as that date.
func TestADayEntryIsStoredAtNoonUTCOfItsDate(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	want := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	for _, name := range []string{"Pacific/Auckland", "America/New_York", "Pacific/Pago_Pago"} {
		store := e.zonedStore(zoneNamed(t, name))
		meeting := logDayMeeting(ctx, t, store, time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC))
		if !meeting.OccurredAt.Equal(want) {
			t.Errorf("%s: stored at %v, want %v", name, meeting.OccurredAt, want)
		}
		read, err := store.GetActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("GetActivity: %v", err)
		}
		if read.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionDay ||
			occurredday.Date(read.OccurredAt).Format(time.DateOnly) != "2026-03-09" {
			t.Errorf("%s: reads back %v as %q, want 2026-03-09 as a day", name, read.OccurredAt, read.OccurredPrecision)
		}
	}
}

// The writer's own offset names the date: late on 9 March in Honolulu is 10
// March in UTC, and the entry still files under the 9th.
func TestTheWritersOffsetNamesTheDate(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	lateEvening := time.Date(2026, 3, 9, 23, 0, 0, 0, time.FixedZone("HST", -10*3600))
	meeting := logDayMeeting(ctx, t, e.store(nil), lateEvening)
	if want := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC); !meeting.OccurredAt.Equal(want) {
		t.Errorf("stored at %v, want noon UTC of 9 March %v", meeting.OccurredAt, want)
	}
}

// An instant made a day takes the day it falls on in the installation zone, so
// a store without that zone refuses rather than guessing.
func TestAnInstantMadeADayOnAStoreWithoutTheZoneIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	note := loggedNote(ctx, t, e)
	day := PrecisionDay
	_, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(note.Id)), UpdateActivityInput{OccurredPrecision: &day})
	if !errors.Is(err, errNoRecordZone) {
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

// A day sent alone keeps the date the stored instant falls on in the
// installation zone, and moves the time to that date's anchor.
func TestAPatchedPrecisionAloneAnchorsTheStoredDate(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	zone := zoneNamed(t, "Pacific/Auckland")
	store := e.zonedStore(zone)
	note := loggedNote(ctx, t, e)
	day := PrecisionDay

	out := patchActivity(ctx, t, store, note, UpdateActivityInput{OccurredPrecision: &day})

	want := occurredday.AnchorIn(note.OccurredAt, zone)
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

// An instant given a new date as a day lands on that date's anchor, and the
// event names the precision that changed.
func TestAPatchedTimeAndDayMoveTogether(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.zonedStore(zoneNamed(t, "America/New_York"))
	note := loggedNote(ctx, t, e)
	moved, day := pastDay.Add(48*time.Hour), PrecisionDay

	out := patchActivity(ctx, t, store, note, UpdateActivityInput{OccurredAt: &moved, OccurredPrecision: &day})

	want := occurredday.Anchor(moved)
	if out.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionDay || !out.OccurredAt.Equal(want) {
		t.Errorf("got %v at %q, want %v at day", out.OccurredAt, out.OccurredPrecision, want)
	}
	if got := lastUpdatedPrecision(t, e, note); got != PrecisionDay {
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

// A day becomes an instant only with the time it happened. Its stored noon
// would otherwise read as a real 12:00.
func TestADayMadeAnInstantWithoutATimeIsRefused(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.zonedStore(time.UTC)
	meeting := logDayMeeting(ctx, t, store, pastDay)
	instant := PrecisionInstant
	_, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), UpdateActivityInput{OccurredPrecision: &instant})
	if _, ok := errors.AsType[*DayToInstantNeedsTimeError](err); !ok {
		t.Fatalf("err = %v, want DayToInstantNeedsTimeError", err)
	}
}

// A day moved to another date audits the precision beside the time. Its event
// names the precision beside the time too, so a consumer reads a date.
func TestADayMovedToAnotherDateKeepsItsPrecisionInTheTrail(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.zonedStore(time.UTC)
	meeting := logDayMeeting(ctx, t, store, pastDay)
	moved, day := pastDay.AddDate(0, 0, 3), PrecisionDay

	out := patchActivity(ctx, t, store, meeting, UpdateActivityInput{OccurredAt: &moved, OccurredPrecision: &day})

	if !out.OccurredAt.Equal(occurredday.Anchor(moved)) {
		t.Fatalf("moved to %v, want %v", out.OccurredAt, occurredday.Anchor(moved))
	}
	before, after := auditImagesFor(t, e, ids.UUID(meeting.Id))
	if before[fieldOccurredPrecision] != PrecisionDay || after[fieldOccurredPrecision] != PrecisionDay {
		t.Errorf("the audit says %v -> %v, want day on both sides", before[fieldOccurredPrecision], after[fieldOccurredPrecision])
	}
	if got := lastUpdatedPrecision(t, e, meeting); got != PrecisionDay {
		t.Errorf("activity.updated names occurred_precision %q beside the moved time, want day", got)
	}
}

// occurredday.EndSQL and occurredday.Ended agree: a day entry ends when its
// date ends in the installation zone, east and west of UTC.
func TestADayEntryEndsWithItsDateInSQLAsInGo(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	meeting := logDayMeeting(ctx, t, e.store(nil), pastDay)
	for _, name := range []string{"Pacific/Auckland", "America/Los_Angeles", "UTC"} {
		setInstallationZone(t, e, name)
		zone := zoneNamed(t, name)
		midnight := time.Date(2026, 3, 10, 0, 0, 0, 0, zone)
		for _, asOf := range []time.Time{midnight.Add(-time.Minute), midnight} {
			var ended bool
			if err := e.owner.QueryRow(context.Background(), fmt.Sprintf(
				`SELECT %s <= $1 FROM activity a WHERE a.id = $2`, occurredday.EndSQL("a")), asOf, ids.UUID(meeting.Id)).Scan(&ended); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if want := occurredday.Ended(meeting.OccurredAt, asOf, zone); ended != want {
				t.Errorf("%s at %v: SQL says ended = %v, Go says %v", name, asOf, ended, want)
			}
		}
	}
}

// occurredday.HappenedSQL answers the cases occurredday.Happened and the
// frontend's activityHasHappened share, so the clocks and the screens agree.
func TestHappenedSQLAnswersTheSharedCases(t *testing.T) {
	e := setupSend(t)
	raw, err := os.ReadFile("../../shared/kernel/occurredday/testdata/happened.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Kind, Precision, Zone string
		At, Now                     time.Time
		Happened                    bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("read no cases")
	}
	for _, c := range cases {
		setInstallationZone(t, e, c.Zone)
		var happened, ahead bool
		if err := e.owner.QueryRow(context.Background(), fmt.Sprintf(
			`SELECT %s, %s FROM (VALUES ($2::text, $3::timestamptz, $4::text, NULL::integer))
			   AS a(kind, occurred_at, occurred_precision, duration_seconds)`,
			occurredday.HappenedSQL("a", "$1"), occurredday.AheadSQL("a", "$1")), c.Now, c.Kind, c.At, c.Precision).Scan(&happened, &ahead); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if happened != c.Happened {
			t.Errorf("%s: SQL says happened = %v, want %v", c.Name, happened, c.Happened)
		}
		// A meeting is ahead until it takes place, and not after.
		if c.Kind == "meeting" && ahead == c.Happened {
			t.Errorf("%s: AheadSQL = %v beside a meeting that took place = %v", c.Name, ahead, c.Happened)
		}
	}
}

// setInstallationZone names the zone the owner's SQL reads which date is today in.
func setInstallationZone(t *testing.T, e *sendEnv, name string) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ('installation.timezone', to_jsonb($1::text))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, name); err != nil {
		t.Fatal(err)
	}
}

// occurredday.WithinSQL answers the shared period cases: a day entry falls in
// the period whose local dates hold its date, from UTC-12 to UTC+14.
func TestWithinSQLAnswersTheSharedCases(t *testing.T) {
	e := setupSend(t)
	raw, err := os.ReadFile("../../shared/kernel/occurredday/testdata/within.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Precision, Zone string
		At, From, To          time.Time
		Within                bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("read %d cases, want the whole table", len(cases))
	}
	for _, c := range cases {
		setInstallationZone(t, e, c.Zone)
		var within bool
		if err := e.owner.QueryRow(context.Background(), fmt.Sprintf(
			`SELECT %s FROM (VALUES ($3::timestamptz, $4::text)) AS a(occurred_at, occurred_precision)`,
			occurredday.WithinSQL("a", "$1", "$2")), c.From, c.To, c.At, c.Precision).Scan(&within); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		if within != c.Within {
			t.Errorf("%s: SQL says within = %v, want %v", c.Name, within, c.Within)
		}
	}
}

// occurredday.LatestPrecisionSQL names the precision behind max(occurred_at),
// instant on a tie with a day's noon, and nothing when the filter admits no row.
func TestLatestPrecisionNamesTheNewestAdmittedRow(t *testing.T) {
	e := setupSend(t)
	noon := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct{ name, filter, want string }{
		{"the newest row is a day entry", "", "day"},
		{"an instant ties a day entry's noon", "a.occurred_at <= $1", "instant"},
		{"the filter reads past a newer day entry", "a.direction = 'inbound' AND a.occurred_precision = 'instant'", "instant"},
		{"the filter admits no row", "a.direction = 'internal'", "none"},
	} {
		var got string
		if err := e.owner.QueryRow(context.Background(), fmt.Sprintf(
			`SELECT coalesce(%s, 'none') FROM (VALUES ($1::timestamptz, 'day', 'inbound'), ($1, 'instant', 'outbound'),
			   ($1 - interval '1 day', 'instant', 'inbound'), ($1 + interval '1 day', 'day', 'inbound'))
			   AS a(occurred_at, occurred_precision, direction)`,
			occurredday.LatestPrecisionSQL("a", c.filter)), noon).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: precision = %s, want %s", c.name, got, c.want)
		}
	}
}

// employment.InPlaceForSQL reads a day entry by its date. It is in place from
// the edge's start date, or once its date begins after capture saw the edge.
// In Auckland a day entry's stored noon falls on the next local date.
func TestAnEmploymentIsInPlaceForADayEntryByItsDate(t *testing.T) {
	e := setupSend(t)
	setInstallationZone(t, e, "Pacific/Auckland")
	auckland := zoneNamed(t, "Pacific/Auckland")
	date := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	day, morning := occurredday.Anchor(date), time.Date(2026, 3, 10, 9, 0, 0, 0, auckland)
	for name, c := range map[string]struct {
		at        time.Time
		precision string
		started   *time.Time
		observed  *time.Time
		inPlace   bool
	}{
		"started on its date":           {day, PrecisionDay, &date, nil, true},
		"started the date after":        {day, PrecisionDay, new(date.AddDate(0, 0, 1)), nil, false},
		"first seen during its date":    {day, PrecisionDay, nil, &morning, false},
		"first seen the evening before": {day, PrecisionDay, nil, new(morning.Add(-10 * time.Hour)), true},
		"an hour after it was seen":     {morning.Add(time.Hour), PrecisionInstant, nil, &morning, true},
		"an edge with no start":         {day, PrecisionDay, nil, nil, true},
	} {
		var inPlace bool
		if err := e.owner.QueryRow(context.Background(), `SELECT `+employment.InPlaceForSQL("r", "a")+`
			FROM (VALUES ($1::date, $2::timestamptz)) AS r(started_at, first_observed_at),
			     (VALUES ($3::timestamptz, $4::text)) AS a(occurred_at, occurred_precision)`,
			c.started, c.observed, c.at, c.precision).Scan(&inPlace); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if inPlace != c.inPlace {
			t.Errorf("%s: in place = %v, want %v", name, inPlace, c.inPlace)
		}
	}
}
