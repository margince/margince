// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// An all-day calendar event states a date, so it lands as a day meeting. The
// meeting lane hands that on to the Worklist row.
func TestAnAllDayCalendarEventIsADayMeeting(t *testing.T) {
	e := integration.Setup(t)
	allDay, ok := meetingmap.AllDayStart(laneDay.Format(time.DateOnly))
	if !ok {
		t.Fatal("the lane day does not parse as a date")
	}
	host := meetingmap.Actor{Email: seatEmail(t, e.Rep1)}
	guest := []meetingmap.Actor{{Email: "buyer@customer.test"}}
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-fair", Subject: "Trade fair", StartsAt: allDay, EndsAt: allDay.Add(24 * time.Hour),
		AllDay: true, Organizer: host, Attendees: guest,
	})
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-review", Subject: "Timed review", StartsAt: laneDay.Add(2 * time.Hour), EndsAt: laneDay.Add(3 * time.Hour),
		Organizer: host, Attendees: guest,
	})

	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	rows, err := lane.Today(e.As(e.Rep1, nil, integration.AdminPerms), laneDay,
		laneDay.Truncate(24*time.Hour).Add(24*time.Hour), 12, attention.TasksMine, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	got := map[string]crmcontracts.OccurredPrecision{}
	for _, row := range rows {
		got[row.Subject] = row.Precision
	}
	want := map[string]crmcontracts.OccurredPrecision{
		"Trade fair":   crmcontracts.OccurredPrecisionOccurredPrecisionDay,
		"Timed review": crmcontracts.OccurredPrecisionOccurredPrecisionInstant,
	}
	for subject, precision := range want {
		if got[subject] != precision {
			t.Errorf("%s reads as %q, want %q (lane rows: %v)", subject, got[subject], precision, got)
		}
	}
}

// readMeetingTime is a meeting's stored start and precision.
func readMeetingTime(t *testing.T, e *integration.Env, id ids.UUID) (time.Time, string) {
	t.Helper()
	var start time.Time
	var precision string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT occurred_at, occurred_precision FROM activity WHERE id = $1`, id).Scan(&start, &precision)
	}); err != nil {
		t.Fatalf("reading the meeting time: %v", err)
	}
	return start.UTC(), precision
}

// A calendar event turned into a whole day, and back into an hour, moves the
// meeting's anchor and its precision together. Both calendar doors do it: the
// replay of a captured event and the move of one past the horizon.
func TestAMeetingFollowsItsEventBetweenAnHourAndAWholeDay(t *testing.T) {
	e := integration.Setup(t)
	id := captureMeeting(t, e, e.AdminUser)
	ctx := calendarOwnerCtx(e, e.AdminUser)
	sink := movingCalendarSink(e)
	day, _ := meetingmap.AllDayStart(meetingStart.Format(time.DateOnly))
	replay := func(at time.Time, allDay bool) {
		t.Helper()
		if _, err := sink.Upsert(ctx, connector.NormalizedRecord{
			EntityType: "activity", NaturalKey: meetingKey,
			Fields: capture.ActivityFields{
				Kind: "meeting", Subject: "Consulting Monthly", Body: "Organizer: client@acme.test",
				OccurredAt: at, DateOnly: allDay,
			},
			Source: calendarSystem + ":" + calendarEvent, CapturedBy: "connector:" + calendarSystem,
			Raw: []byte(`{"id":"` + calendarEvent + `"}`),
		}); err != nil {
			t.Fatalf("replaying the event: %v", err)
		}
	}
	steps := []struct {
		name  string
		write func()
		at    time.Time
		want  string
	}{
		{"replayed as a whole day", func() { replay(day, true) }, day, "day"},
		{"replayed as an hour", func() { replay(meetingStart, false) }, meetingStart, "instant"},
		{"moved to a whole day", func() {
			if err := sink.MoveMeeting(ctx, meetingKey, connector.MeetingSlot{Start: day, AllDay: true}); err != nil {
				t.Fatalf("moving: %v", err)
			}
		}, day, "day"},
		{"moved to an hour", func() {
			if err := sink.MoveMeeting(ctx, meetingKey, connector.MeetingSlot{Start: meetingStart}); err != nil {
				t.Fatalf("moving: %v", err)
			}
		}, meetingStart, "instant"},
	}
	for _, step := range steps {
		step.write()
		if at, precision := readMeetingTime(t, e, id); !at.Equal(step.at) || precision != step.want {
			t.Errorf("%s: the meeting reads %v as %q, want %v as %q", step.name, at, precision, step.at, step.want)
		}
	}

	// An hour at the date's anchor moved to the whole day changes only the
	// precision, and the event names the precision alone.
	replay(day, false)
	if err := sink.MoveMeeting(ctx, meetingKey, connector.MeetingSlot{Start: day, AllDay: true}); err != nil {
		t.Fatalf("moving: %v", err)
	}
	if changed := lastUpdate(t, e, id); changed["occurred_precision"] != "day" || changed["occurred_at"] != nil {
		t.Errorf("the move to a whole day announced %v, want occurred_precision day alone", changed)
	}
}

// lastUpdate is the changed_fields of the newest activity.updated event.
func lastUpdate(t *testing.T, e *integration.Env, id ids.UUID) map[string]any {
	t.Helper()
	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT envelope->'payload'->'changed_fields' FROM event_outbox
			  WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1::text
			  ORDER BY id DESC LIMIT 1`, id.String()).Scan(&raw)
	}); err != nil {
		t.Fatalf("reading the activity.updated event: %v", err)
	}
	var changed map[string]any
	if err := json.Unmarshal(raw, &changed); err != nil {
		t.Fatalf("decoding changed_fields: %v", err)
	}
	return changed
}

// An all-day event is stored at its date's noon in the record zone. Noon UTC is
// the previous date in Auckland, so a capture that kept it would misfile it.
func TestAnAllDayEventIsAnchoredInTheRecordZone(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Pacific/Auckland"'::jsonb WHERE key = 'installation.timezone'`)
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	allDay, _ := meetingmap.AllDayStart("2026-03-09")
	captureCalendarEvent(t, e, e.Rep1, meetingmap.Event{
		ID: "evt-nz-fair", Subject: "Auckland fair", StartsAt: allDay, EndsAt: allDay.Add(24 * time.Hour), AllDay: true,
		Organizer: meetingmap.Actor{Email: seatEmail(t, e.Rep1)}, Attendees: []meetingmap.Actor{{Email: "buyer@customer.test"}},
	})
	var start time.Time
	var precision string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT occurred_at, occurred_precision FROM activity WHERE subject = 'Auckland fair'`).Scan(&start, &precision)
	}); err != nil {
		t.Fatalf("reading the captured meeting: %v", err)
	}
	if local := start.In(auckland); precision != "day" || local.Format(time.DateOnly) != "2026-03-09" || local.Hour() != 12 {
		t.Errorf("captured at %v as %q, want 9 March at Auckland's noon as a day", local, precision)
	}
}
