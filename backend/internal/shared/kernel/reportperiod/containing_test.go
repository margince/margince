// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import (
	"fmt"
	"testing"
	"time"
)

// labelOf renders the label a period bucket carries, in the spelling ParseLabel
// reads, so Containing can be held to Span in every grain.
func labelOf(grain Grain, from time.Time, fiscalStart int) string {
	if grain == GrainMonth {
		return fmt.Sprintf("%d-%02d", from.Year(), from.Month())
	}
	year := from.Year()
	if int(from.Month()) < fiscalStart {
		year--
	}
	label := fmt.Sprintf("%d", year)
	if fiscalStart != 1 {
		label = fmt.Sprintf("FY%d/%02d", year, (year+1)%100)
	}
	if grain == GrainQuarter {
		label += fmt.Sprintf("-Q%d", (int(from.Month())-fiscalStart+12)%12/3+1)
	}
	return label
}

// Every day of two years is checked under every fiscal start, at its first and
// its last local second. The period holds the instant, opens on its fiscal
// boundary and runs its whole length. Span reads the same bounds from its label.
func TestContainingAgreesWithSpanUnderEveryFiscalStart(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	lengths := map[Grain]int{GrainYear: 12, GrainQuarter: 3, GrainMonth: 1}
	for fiscalStart := 1; fiscalStart <= 12; fiscalStart++ {
		for day := time.Date(2025, 1, 1, 0, 0, 0, 0, zone); day.Year() < 2027; day = day.AddDate(0, 0, 1) {
			for _, at := range []time.Time{day, day.AddDate(0, 0, 1).Add(-time.Second)} {
				for grain, months := range lengths {
					checkContaining(t, grain, months, at, zone, fiscalStart)
				}
			}
		}
	}
}

func checkContaining(t *testing.T, grain Grain, months int, at time.Time, zone *time.Location, fiscalStart int) {
	t.Helper()
	from, to, ok := Containing(grain, at, zone, fiscalStart)
	if !ok || at.Before(from) || !at.Before(to) || !to.Equal(from.AddDate(0, months, 0)) {
		t.Fatalf("%s of %s from %d = [%s, %s) ok=%v", grain, at, fiscalStart, from, to, ok)
	}
	if grain != GrainMonth && (int(from.Month())-fiscalStart+12)%months != 0 {
		t.Fatalf("%s of %s from %d opens in month %d, off the fiscal boundary", grain, at, fiscalStart, from.Month())
	}
	label := labelOf(grain, from, fiscalStart)
	spanFrom, spanTo, read := Span(grain, label, zone, fiscalStart)
	if !read || !spanFrom.Equal(from) || !spanTo.Equal(to) {
		t.Fatalf("%s of %s from %d: Containing [%s, %s), Span(%s) [%s, %s)",
			grain, at, fiscalStart, from, to, label, spanFrom, spanTo)
	}
}

func TestContainingRefusesWhatSpanRefuses(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name        string
		grain       Grain
		zone        *time.Location
		fiscalStart int
	}{
		{"no zone", GrainQuarter, nil, 1},
		{"fiscal start below 1", GrainQuarter, time.UTC, 0},
		{"fiscal start above 12", GrainQuarter, time.UTC, 13},
		{"an unknown grain", Grain("week"), time.UTC, 1},
	} {
		if _, _, ok := Containing(c.grain, at, c.zone, c.fiscalStart); ok {
			t.Errorf("%s: Containing answered a period", c.name)
		}
	}
}
