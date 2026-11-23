// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deadline

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var noon = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) *time.Time {
	t := noon.Add(d)
	return &t
}

// The boundary is the whole point of this package: every surface that decides
// lateness agrees here or the same record reads two ways.
func TestPassedIsStrict(t *testing.T) {
	for _, tc := range []struct {
		name string
		due  *time.Time
		want bool
	}{
		{"nothing promised cannot have been missed", nil, false},
		{"due tomorrow", at(24 * time.Hour), false},
		{"due one nanosecond from now", at(time.Nanosecond), false},
		{"due exactly now is DUE, not late", at(0), false},
		{"due one nanosecond ago is late", at(-time.Nanosecond), true},
		{"due yesterday", at(-24 * time.Hour), true},
	} {
		if got := Passed(tc.due, noon); got != tc.want {
			t.Errorf("%s: Passed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDayPassedIsOverOnlyAfterTheWholeDayInTheZone(t *testing.T) {
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	endOfNov20 := time.Date(2026, 11, 20, 23, 59, 59, 0, time.UTC)
	endOfNov20InAuckland := time.Date(2026, 11, 20, 23, 59, 59, 0, auckland)
	for name, c := range map[string]struct {
		due, now time.Time
		loc      *time.Location
		want     bool
	}{
		"last fraction of the day": {endOfNov20, time.Date(2026, 11, 20, 23, 59, 59, 500_000_000, time.UTC), time.UTC, false},
		"first moment after":       {endOfNov20, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), time.UTC, true},
		"the day is over there":    {endOfNov20InAuckland, time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), auckland, true},
	} {
		if got := DayPassed(c.due, c.now, c.loc); got != c.want {
			t.Errorf("%s: DayPassed = %v, want %v", name, got, c.want)
		}
	}
}

// The frontend's taskDueOnDay reads the same table
// (frontend/src/format/dueintent.test.ts), so the two sides count a picked day
// alike.
func TestWallDayOffsetFollowsThePickedDate(t *testing.T) {
	raw, err := os.ReadFile("testdata/pickedday.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		WallClock string    `json:"wall_clock"`
		Now       time.Time `json:"now"`
		Zone      string    `json:"zone"`
		Offset    int       `json:"offset"`
		EndUTC    string    `json:"end_utc"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("the shared table is empty, so this proves nothing")
	}
	for _, tc := range cases {
		loc, err := time.LoadLocation(tc.Zone)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := WallDayOffset(tc.WallClock, tc.Now, loc)
		if !ok || got != tc.Offset {
			t.Errorf("%s at %s in %s: offset %d (%v), want %d", tc.WallClock, tc.Now, tc.Zone, got, ok, tc.Offset)
		}
		if tc.EndUTC == "" {
			continue
		}
		want, err := time.Parse(time.RFC3339Nano, tc.EndUTC)
		if err != nil {
			t.Fatal(err)
		}
		due := tc.Now
		if end := TaskDueOn(&due, false, tc.WallClock, loc); !end.Equal(want) {
			t.Errorf("%s in %s is due until %s, want %s", tc.WallClock, tc.Zone, end.UTC().Format(time.RFC3339Nano), tc.EndUTC)
		}
	}
	if _, ok := WallDayOffset("tomorrow", noon, time.UTC); ok {
		t.Error("text that is not a date must report false")
	}
}

func TestTaskDueOnJudgesADateOnlyTaskAtTheEndOfItsChosenDate(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 11, 17, 7, 59, 59, 0, time.UTC)
	got := TaskDueOn(&due, false, "2026-11-16T23:59:59", berlin)
	if want := time.Date(2026, 11, 17, 0, 0, 0, 0, berlin).Add(-time.Nanosecond); !got.Equal(want) {
		t.Errorf("a date-only task is judged at %v, want %v", got, want)
	}
	if TaskDueOn(&due, true, "2026-11-16T22:30", berlin) != &due {
		t.Error("a timed task keeps its instant")
	}
	if TaskDueOn(&due, false, "", berlin) != &due || TaskDueOn(&due, false, "2026-11-16T23:59:59", nil) != &due {
		t.Error("a task with no chosen clock, or no zone, keeps its instant")
	}
}
