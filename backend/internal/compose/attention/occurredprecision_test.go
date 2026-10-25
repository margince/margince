// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A meeting's time reaches its row with the precision it was stored with, and
// every other row still states one, because the wire requires it.
func TestEveryWorklistRowStatesHowToReadItsTime(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 3, 10, 15, 0, 0, 0, time.UTC)
	day := crmcontracts.OccurredPrecisionOccurredPrecisionDay
	started := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	rows := []ranked{
		classifyUnansweredMeeting(meetingAwaitingOutcomeItem(MeetingAwaitingOutcome{
			ID: ids.New[ids.ActivityKind]().UUID, Subject: "Trade fair", StartedAt: started, Precision: day,
		}), asOf),
		classifyMeeting(meetingItem(Meeting{
			ID: ids.New[ids.ActivityKind]().UUID, Subject: "Fair day two", StartsAt: started.Add(24 * time.Hour), Precision: day,
		}), asOf),
		classifyMeeting(meetingItem(Meeting{
			ID: ids.New[ids.ActivityKind]().UUID, Subject: "Review", StartsAt: asOf.Add(time.Hour),
		}), asOf),
		{item: crmcontracts.WorklistItem{Id: "no-time", Source: crmcontracts.WorklistItemSourceBatch}},
	}

	got := map[string]crmcontracts.OccurredPrecision{}
	for _, item := range renderInOrder(rows, ids.UUID{}) {
		title := item.Id
		if item.Title != nil {
			title = *item.Title
		}
		got[title] = item.OccurredPrecision
	}
	want := map[string]crmcontracts.OccurredPrecision{
		"Trade fair":   day,
		"Fair day two": day,
		"Review":       crmcontracts.OccurredPrecisionOccurredPrecisionInstant,
		"no-time":      crmcontracts.OccurredPrecisionOccurredPrecisionInstant,
	}
	for title, precision := range want {
		if got[title] != precision {
			t.Errorf("%s states %q, want %q", title, got[title], precision)
		}
	}
}

// A day meeting has no start to be near or past. It never raises meeting_soon
// and is never overdue, read before or after its stored noon.
func TestADayMeetingIsNeverAboutToStartOrLate(t *testing.T) {
	t.Parallel()
	noon := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	for _, asOf := range []time.Time{noon.Add(-time.Hour), noon.Add(3 * time.Hour)} {
		row := classifyMeeting(meetingItem(Meeting{
			ID: ids.New[ids.ActivityKind]().UUID, Subject: "Trade fair", StartsAt: noon,
			Precision: crmcontracts.OccurredPrecisionOccurredPrecisionDay,
		}), asOf)
		for _, because := range row.item.Because {
			if because.Kind == "meeting_soon" {
				t.Errorf("at %v a day meeting says meeting_soon", asOf)
			}
		}
		if row.item.Overdue != nil && *row.item.Overdue {
			t.Errorf("at %v a day meeting is marked overdue", asOf)
		}
	}
}

// laneWindows answers each meeting lane with rows and keeps the window it was asked.
type laneWindows struct {
	rows        []Meeting
	today, over *MeetingWindow
}

func (w *laneWindows) Today(_ context.Context, window MeetingWindow, _ int, _ TaskScope, _ ids.UUID) ([]Meeting, error) {
	w.today = &window
	return w.rows, nil
}

func (w *laneWindows) Since(_ context.Context, window MeetingWindow, _ int, _ TaskScope, _ ids.UUID) ([]MeetingAwaitingOutcome, error) {
	w.over = &window
	return nil, nil
}

func zoneOf(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// The meetings lane asks for today's day meetings by their date, which the
// installation zone names. A day meeting stays on it for its whole date, east
// and west of UTC. Timed meetings keep the window from now.
func TestTheMeetingsLaneAsksForTodaysDayMeetingsByDate(t *testing.T) {
	t.Parallel()
	tenth := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		zone string
		hour int
	}{{"Pacific/Auckland", 23}, {"America/Los_Angeles", 8}, {"Asia/Tokyo", 23}, {"UTC", 0}} {
		loc := zoneOf(t, c.zone)
		lanes := &laneWindows{}
		svc := &Service{meetings: lanes, zone: func(context.Context) (*time.Location, error) { return loc, nil }}
		asOf := time.Date(2026, 3, 10, c.hour, 30, 0, 0, loc)
		if _, err := svc.meetingsAhead(context.Background(), asOf, asOf.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		got := *lanes.today
		if !got.DaysFrom.Equal(tenth) || !got.DaysUntil.Equal(tenth.AddDate(0, 0, 1)) || !got.From.Equal(asOf) {
			t.Errorf("%s at %v asks %+v, want 10 March's anchor and timed meetings from now", c.zone, asOf, got)
		}
	}
}

// A day meeting is drawn above the timed ones, as a calendar's all-day row.
func TestTheMeetingsLanePutsDayMeetingsFirst(t *testing.T) {
	t.Parallel()
	asOf := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	lanes := &laneWindows{rows: []Meeting{
		{ID: ids.NewV7(), Subject: "Review", StartsAt: asOf.Add(time.Hour)},
		{ID: ids.NewV7(), Subject: "Trade fair", StartsAt: asOf.Add(3 * time.Hour), Precision: crmcontracts.OccurredPrecisionOccurredPrecisionDay},
		{ID: ids.NewV7(), Subject: "Lunch call", StartsAt: asOf.Add(4 * time.Hour)},
	}}
	svc := &Service{meetings: lanes}
	ahead, err := svc.meetingsAhead(context.Background(), asOf, asOf.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, meeting := range ahead {
		got = append(got, meeting.Subject)
	}
	if len(got) != 3 || got[0] != "Trade fair" || got[1] != "Review" || got[2] != "Lunch call" {
		t.Errorf("the lane holds %v, want [Trade fair Review Lunch call]", got)
	}
}

// A day meeting asks what happened only once its date is over in the
// installation zone: the outcome lane's day range ends at today's date.
func TestADayMeetingOwesAnOutcomeOnlyAfterItsDate(t *testing.T) {
	t.Parallel()
	auckland := zoneOf(t, "Pacific/Auckland")
	tenth := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		asOf  time.Time
		owing bool
	}{
		{time.Date(2026, 3, 10, 23, 0, 0, 0, auckland), false},
		{time.Date(2026, 3, 11, 0, 30, 0, 0, auckland), true},
	} {
		lanes := &laneWindows{}
		svc := &Service{meetingsAwaitingOutcome: lanes, zone: func(context.Context) (*time.Location, error) { return auckland, nil }}
		if _, err := svc.meetingsOwingAnOutcome(context.Background(), c.asOf); err != nil {
			t.Fatal(err)
		}
		got := *lanes.over
		owing := !tenth.Before(got.DaysFrom) && tenth.Before(got.DaysUntil)
		if owing != c.owing || !got.Until.Equal(c.asOf) {
			t.Errorf("at %v the lane asks %+v: 10 March owing = %v, want %v", c.asOf, got, owing, c.owing)
		}
	}
}
