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

// windowedMeetings answers each Today window with the meetings anchored in it.
type windowedMeetings struct{ all []Meeting }

func (w *windowedMeetings) Today(_ context.Context, from, until time.Time, _ int, _ TaskScope, _ ids.UUID) ([]Meeting, error) {
	var out []Meeting
	for _, meeting := range w.all {
		if !meeting.StartsAt.Before(from) && meeting.StartsAt.Before(until) {
			out = append(out, meeting)
		}
	}
	return out, nil
}

// Today's day meeting stays on the meetings lane after its stored noon, while a
// timed meeting that started at noon has left it.
func TestTodaysDayMeetingStaysAheadAllDay(t *testing.T) {
	t.Parallel()
	york, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	noon := time.Date(2026, 3, 10, 12, 0, 0, 0, york)
	day := crmcontracts.OccurredPrecisionOccurredPrecisionDay
	meetings := &windowedMeetings{all: []Meeting{
		{ID: ids.NewV7(), Subject: "Trade fair", StartsAt: noon.UTC(), Precision: day},
		{ID: ids.NewV7(), Subject: "Lunch call", StartsAt: noon.UTC()},
		{ID: ids.NewV7(), Subject: "Review", StartsAt: noon.Add(5 * time.Hour).UTC()},
	}}
	svc := &Service{meetings: meetings, zone: func(context.Context) (*time.Location, error) { return york, nil }}
	asOf := noon.Add(3 * time.Hour)
	ahead, err := svc.meetingsAhead(context.Background(), asOf, time.Date(2026, 3, 11, 0, 0, 0, 0, york))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, meeting := range ahead {
		got = append(got, meeting.Subject)
	}
	if len(got) != 2 || got[0] != "Trade fair" || got[1] != "Review" {
		t.Errorf("the lane at 15:00 holds %v, want [Trade fair Review]", got)
	}
}

// A day meeting asks what happened only once its date is over in the record zone.
func TestADayMeetingOwesAnOutcomeOnlyAfterItsDate(t *testing.T) {
	t.Parallel()
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	noon := time.Date(2026, 3, 10, 12, 0, 0, 0, auckland).UTC()
	over := []MeetingAwaitingOutcome{
		{ID: ids.NewV7(), Subject: "Trade fair", StartedAt: noon, Precision: crmcontracts.OccurredPrecisionOccurredPrecisionDay},
		{ID: ids.NewV7(), Subject: "Lunch call", StartedAt: noon},
	}
	evening := time.Date(2026, 3, 10, 23, 0, 0, 0, auckland)
	if kept := stillOwingAnOutcome(over, evening, auckland); len(kept) != 1 || kept[0].Subject != "Lunch call" {
		t.Errorf("on its own date the outcome lane keeps %v, want only the timed call", kept)
	}
	nextMorning := time.Date(2026, 3, 11, 0, 30, 0, 0, auckland)
	if kept := stillOwingAnOutcome(over, nextMorning, auckland); len(kept) != 2 {
		t.Errorf("after its date the outcome lane keeps %d rows, want both", len(kept))
	}
}
