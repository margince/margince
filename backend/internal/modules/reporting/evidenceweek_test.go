// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"slices"
	"strconv"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func meetingAt(key string, at time.Time) Fact {
	return Fact{Metric: "meetings_held", ContextID: "interval", Row: crmcontracts.ReportingEvidenceRow{Key: key, OccurredAt: &at}}
}

func pageKeys(t *testing.T, frame crmcontracts.ReportingContext, facts []Fact, group string, cursor *string, limit int) ([]string, crmcontracts.ReportingEvidence) {
	t.Helper()
	page, err := evidencePage(frame, facts, "meetings_held", "interval", group, nil, cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, len(page.Rows))
	for i, row := range page.Rows {
		keys[i] = row.Key
	}
	return keys, page
}

func TestWeeklyEvidenceUsesLocalCalendarBoundariesAndPages(t *testing.T) {
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// The spring change falls inside this week, so it lasts 167 hours.
	start := time.Date(2026, 3, 2, 0, 0, 0, 0, zone)
	end := start.AddDate(0, 0, 7)
	placed := meetingAt("start", end)
	placed.PlacedAt = &start
	facts := []Fact{meetingAt("before", start.Add(-time.Nanosecond)), placed, meetingAt("last", end.Add(-time.Nanosecond)), meetingAt("end", end)}
	frame := crmcontracts.ReportingContext{Timezone: "America/New_York"}
	keys, first := pageKeys(t, frame, facts, "week:2026-03-02", nil, 1)
	if !slices.Equal(keys, []string{"start"}) || !first.Truncated || first.NextCursor == nil || *first.NextCursor != "1" {
		t.Fatalf("first page = %v %+v", keys, first)
	}
	keys, second := pageKeys(t, frame, facts, "week:2026-03-02", first.NextCursor, 1)
	if !slices.Equal(keys, []string{"last"}) || second.Truncated || second.NextCursor != nil {
		t.Fatalf("second page = %v %+v", keys, second)
	}
}

func TestWeeklyEvidenceKeepsAutumnAndPartialWeeksOnTheLocalCalendar(t *testing.T) {
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	frame := crmcontracts.ReportingContext{Timezone: "Europe/Berlin"}
	// The autumn change falls inside this week, so it lasts 169 hours.
	monday := time.Date(2026, 10, 19, 0, 0, 0, 0, zone)
	next := time.Date(2026, 10, 26, 0, 0, 0, 0, zone)
	facts := []Fact{meetingAt("last hour", next.Add(-30*time.Minute)), meetingAt("next week", next), meetingAt("monday", monday)}
	if keys, _ := pageKeys(t, frame, facts, "week:2026-10-19", nil, 50); !slices.Equal(keys, []string{"last hour", "monday"}) {
		t.Errorf("autumn week = %v, want the 169 local hours", keys)
	}
	// A week a period starts midweek runs from that day to the next Monday.
	if keys, _ := pageKeys(t, frame, facts, "week:2026-10-22", nil, 50); !slices.Equal(keys, []string{"last hour"}) {
		t.Errorf("partial week = %v, want only Sunday's meeting", keys)
	}
}

func TestWeeklyEvidenceKeepsUndatedGroupFallbackAndInvalidDates(t *testing.T) {
	at := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, zone, group string
	}{
		{"invalid zone", "invalid/location", "week:2026-03-02"},
		{"invalid date", "UTC", "week:invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dated := meetingAt("dated", at)
			dated.GroupKey = tc.group
			undated := Fact{Metric: "meetings_held", ContextID: "interval", GroupKey: tc.group, Row: crmcontracts.ReportingEvidenceRow{Key: "undated"}}
			if keys, _ := pageKeys(t, crmcontracts.ReportingContext{Timezone: tc.zone}, []Fact{undated, dated}, tc.group, nil, 50); !slices.Equal(keys, []string{"undated"}) {
				t.Fatalf("keys = %v, want only the undated fact", keys)
			}
		})
	}
}

func TestEvidencePagesClampTheLimitAndRefuseABadCursor(t *testing.T) {
	at := time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
	facts := make([]Fact, 120)
	for i := range facts {
		facts[i] = meetingAt(strconv.Itoa(i), at)
	}
	frame := crmcontracts.ReportingContext{Timezone: "UTC"}
	for limit, want := range map[int]int{0: 1, 100: 100, 101: 100} {
		if keys, page := pageKeys(t, frame, facts, "week:2026-03-02", nil, limit); len(keys) != want || !page.Truncated {
			t.Errorf("limit %d gave %d rows, want %d and a continuation", limit, len(keys), want)
		}
	}
	beyond := "500"
	if keys, page := pageKeys(t, frame, facts, "week:2026-03-02", &beyond, 50); len(keys) != 0 || page.Truncated || page.NextCursor != nil {
		t.Errorf("a cursor past the end gave %v", keys)
	}
	for _, bad := range []string{"", "-1", "x"} {
		if _, err := evidencePage(frame, facts, "meetings_held", "interval", "", nil, &bad, 50); err == nil {
			t.Errorf("cursor %q was accepted", bad)
		}
	}
	if keys, _ := pageKeys(t, frame, nil, "week:2026-03-02", nil, 50); len(keys) != 0 {
		t.Errorf("no facts gave %v", keys)
	}
}

func TestEvidenceRowsWithholdOwnerAndStageLabelsTheReaderMayNotSee(t *testing.T) {
	owner := openapi_types.UUID(ids.NewV7())
	fact := func(key string, withheld, restricted bool) Fact {
		return Fact{Metric: "meetings_held", ContextID: "interval", OwnerLabel: "Ada", StageLabel: "Proposal", OwnerWithheld: withheld, Row: crmcontracts.ReportingEvidenceRow{Key: key, OwnerId: &owner, Restricted: restricted}}
	}
	facts := []Fact{fact("open", false, false), fact("withheld", true, false), fact("restricted", false, true)}
	_, page := pageKeys(t, crmcontracts.ReportingContext{}, facts, "", nil, 50)
	open, withheld, restricted := page.Rows[0], page.Rows[1], page.Rows[2]
	if open.OwnerId == nil || open.OwnerLabel == nil || *open.OwnerLabel != "Ada" || open.StageLabel == nil || *open.StageLabel != "Proposal" {
		t.Errorf("open row = %+v", open)
	}
	if withheld.OwnerId != nil || withheld.OwnerLabel != nil || withheld.StageLabel == nil {
		t.Errorf("withheld row = %+v, want the stage without the owner", withheld)
	}
	if restricted.OwnerId != nil || restricted.OwnerLabel != nil || restricted.StageLabel != nil {
		t.Errorf("restricted row = %+v, want no owner and no stage", restricted)
	}
	facts[0].OwnerLabel = "changed"
	if *open.OwnerLabel != "Ada" {
		t.Error("a row shares its label with the evaluation's facts")
	}
}

func BenchmarkWeeklyEvidenceLatePage(b *testing.B) {
	for _, count := range []int{1000, 10000, 50000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			facts := make([]Fact, count)
			for i := range facts {
				facts[i] = meetingAt(strconv.Itoa(i), time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC))
			}
			cursor := strconv.Itoa(count - 50)
			frame := crmcontracts.ReportingContext{Timezone: "Europe/Berlin"}
			b.ReportAllocs()
			for b.Loop() {
				page, err := evidencePage(frame, facts, "meetings_held", "interval", "week:2026-03-02", nil, &cursor, 50)
				if err != nil || len(page.Rows) != 50 || page.Rows[0].Key != cursor || page.Truncated {
					b.Fatalf("late page = %+v, %v", page, err)
				}
			}
		})
	}
}
