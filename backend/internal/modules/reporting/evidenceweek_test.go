// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"runtime/debug"
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
	seeded, stored, shown := "Seeded owner", "Seeded stage", time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	type want struct {
		ownerID      bool
		owner, stage string
	}
	for _, tc := range []struct {
		name                 string
		withheld, restricted bool
		ownerLabel, stage    string
		want                 want
	}{
		{"open", false, false, "Ada", "Proposal", want{true, "Ada", "Proposal"}},
		{"open without labels keeps the row's own", false, false, "", "", want{true, seeded, stored}},
		{"withheld owner", true, false, "Ada", "Proposal", want{false, "", "Proposal"}},
		{"restricted row", false, true, "Ada", "Proposal", want{false, "", ""}},
		{"withheld and restricted", true, true, "Ada", "Proposal", want{false, "", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fact := Fact{
				OwnerLabel: tc.ownerLabel, StageLabel: tc.stage, OwnerWithheld: tc.withheld, EvidenceAt: &shown,
				Row: crmcontracts.ReportingEvidenceRow{OwnerId: &owner, OwnerLabel: &seeded, StageLabel: &stored, Restricted: tc.restricted},
			}
			row := evidenceRow(fact)
			got := want{ownerID: row.OwnerId != nil}
			if row.OwnerLabel != nil {
				got.owner = *row.OwnerLabel
			}
			if row.StageLabel != nil {
				got.stage = *row.StageLabel
			}
			if got != tc.want {
				t.Errorf("row shows %+v, want %+v", got, tc.want)
			}
			if row.OccurredAt == nil || !row.OccurredAt.Equal(shown) {
				t.Errorf("row date = %v, want the evidence date %v", row.OccurredAt, shown)
			}
		})
	}
}

func TestEvidenceRowsKeepTheirOwnLabels(t *testing.T) {
	facts := []Fact{
		{Metric: "meetings_held", ContextID: "interval", OwnerLabel: "Ada", StageLabel: "Proposal", Row: crmcontracts.ReportingEvidenceRow{Key: "first"}},
		{Metric: "meetings_held", ContextID: "interval", OwnerLabel: "Grace", StageLabel: "Won", Row: crmcontracts.ReportingEvidenceRow{Key: "second"}},
	}
	_, page := pageKeys(t, crmcontracts.ReportingContext{}, facts, "", nil, 50)
	facts[0].OwnerLabel, facts[0].StageLabel = "changed", "changed"
	first, second := page.Rows[0], page.Rows[1]
	if *first.OwnerLabel != "Ada" || *first.StageLabel != "Proposal" || *second.OwnerLabel != "Grace" || *second.StageLabel != "Won" {
		t.Errorf("rows show %s/%s and %s/%s, want Ada/Proposal and Grace/Won", *first.OwnerLabel, *first.StageLabel, *second.OwnerLabel, *second.StageLabel)
	}
}

// A late page skips most facts, so its cost must not grow with them. It looks
// up the zone once and allocates nothing for a fact it does not return.
func TestALateWeeklyEvidencePageAllocatesOnlyForItsRows(t *testing.T) {
	// AllocsPerRun counts the runtime's own objects too, and the process's
	// first GC cycle allocates some, so no cycle may start inside the count.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	frame := crmcontracts.ReportingContext{Timezone: "Europe/Berlin"}
	allocs := func(count int) float64 {
		facts := make([]Fact, count)
		for i := range facts {
			facts[i] = meetingAt(strconv.Itoa(i), time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC))
			facts[i].OwnerLabel = "Ada"
		}
		cursor := strconv.Itoa(count - 50)
		return testing.AllocsPerRun(5, func() {
			if _, err := evidencePage(frame, facts, "meetings_held", "interval", "week:2026-03-02", nil, &cursor, 50); err != nil {
				t.Fatal(err)
			}
		})
	}
	if few, many := allocs(100), allocs(2000); many != few {
		t.Errorf("a late page allocates %v times over 2000 facts and %v over 100, want the same", many, few)
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
