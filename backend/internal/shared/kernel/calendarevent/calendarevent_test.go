// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package calendarevent_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
)

func clock(hour int) *time.Time {
	at := time.Date(2026, 11, 14, hour, 0, 0, 0, time.UTC)
	return &at
}

func meeting() calendarevent.Event {
	return calendarevent.Event{
		Title: "Pricing review", Location: "Teams", Start: clock(14), End: clock(15),
		Attendees: []calendarevent.Attendee{{Email: "anna@partner.example"}, {Email: "owner@myco.com"}},
	}
}

func TestASendIsComparedFieldByField(t *testing.T) {
	t.Parallel()
	moved := meeting()
	moved.Start, moved.End = clock(16), clock(17)
	placed := meeting()
	placed.Location = "Office Berlin"
	unplaced := meeting()
	unplaced.Location = ""
	renamed := meeting()
	renamed.Title = "Pricing and rollout"
	guests := meeting()
	guests.Attendees = []calendarevent.Attendee{{Email: "ANNA@partner.example"}, {Email: "ben@partner.example"}}
	for name, tc := range map[string]struct {
		after calendarevent.Event
		want  calendarevent.Difference
	}{
		"nothing":           {meeting(), calendarevent.Difference{}},
		"moved":             {moved, calendarevent.Difference{Time: true}},
		"a new place":       {placed, calendarevent.Difference{Location: true}},
		"the place removed": {unplaced, calendarevent.Difference{Location: true}},
		"renamed":           {renamed, calendarevent.Difference{Title: true}},
		"a guest swapped":   {guests, calendarevent.Difference{Added: []calendarevent.Attendee{{Email: "ben@partner.example"}}, Removed: []calendarevent.Attendee{{Email: "owner@myco.com"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := calendarevent.Compare(meeting(), tc.after)
			if got.Time != tc.want.Time || got.Location != tc.want.Location || got.Title != tc.want.Title ||
				len(got.Added) != len(tc.want.Added) || len(got.Removed) != len(tc.want.Removed) {
				t.Fatalf("Compare = %+v, want %+v", got, tc.want)
			}
			for i := range got.Added {
				if got.Added[i].Email != tc.want.Added[i].Email {
					t.Errorf("added %v, want %v", got.Added, tc.want.Added)
				}
			}
			if got.Any() != (tc.want.Time || tc.want.Location || tc.want.Title || len(tc.want.Added) > 0) {
				t.Errorf("Any() = %v for %+v", got.Any(), got)
			}
		})
	}
}

// What one side does not know is not a change: a time no zone resolved, an end
// left out, or a guest list cut short.
func TestWhatOneSideDoesNotKnowIsNoChange(t *testing.T) {
	t.Parallel()
	noTime := meeting()
	noTime.Start, noTime.End = nil, nil
	noEnd := meeting()
	noEnd.End = nil
	cut := meeting()
	cut.Attendees, cut.AttendeesCut = cut.Attendees[:1], true
	for name, after := range map[string]calendarevent.Event{"no time": noTime, "no end": noEnd, "a cut list": cut} {
		if diff := calendarevent.Compare(meeting(), after); diff.Any() {
			t.Errorf("%s: Compare = %+v, want nothing changed", name, diff)
		}
	}
}

func TestAPartstatIsAnAnswerOnlyWhenItAnswers(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"ACCEPTED": "accepted", " tentative ": "tentative", "Declined": "declined",
		"NEEDS-ACTION": "", "DELEGATED": "", "": "",
	} {
		if got := calendarevent.AnswerOf(in); got != want {
			t.Errorf("AnswerOf(%q) = %q, want %q", in, got, want)
		}
	}
}
