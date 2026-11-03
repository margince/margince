// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import (
	"fmt"
	"testing"
	"time"
)

// quarterLabelOf renders the label a quarter bucket carries, in the spelling
// ParseLabel reads, so Containing can be held to Span.
func quarterLabelOf(from time.Time, fiscalStart int) string {
	year := from.Year()
	if int(from.Month()) < fiscalStart {
		year--
	}
	quarter := (int(from.Month())-fiscalStart+12)%12/3 + 1
	if fiscalStart == 1 {
		return fmt.Sprintf("%d-Q%d", year, quarter)
	}
	return fmt.Sprintf("FY%d/%02d-Q%d", year, (year+1)%100, quarter)
}

// Every day of two years is checked under every fiscal start. The period holds
// the day, opens on its fiscal boundary and runs its whole length. A quarter is
// the one Span reads from that quarter's own label.
func TestContainingAgreesWithSpanUnderEveryFiscalStart(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	lengths := map[Grain]int{GrainYear: 12, GrainQuarter: 3, GrainMonth: 1}
	for fiscalStart := 1; fiscalStart <= 12; fiscalStart++ {
		for day := time.Date(2025, 1, 1, 12, 0, 0, 0, zone); day.Year() < 2027; day = day.AddDate(0, 0, 1) {
			for grain, months := range lengths {
				from, to, ok := Containing(grain, day, zone, fiscalStart)
				if !ok || day.Before(from) || !day.Before(to) || !to.Equal(from.AddDate(0, months, 0)) {
					t.Fatalf("%s of %s from %d = [%s, %s) ok=%v", grain, day, fiscalStart, from, to, ok)
				}
				if grain != GrainMonth && (int(from.Month())-fiscalStart+12)%months != 0 {
					t.Fatalf("%s of %s from %d opens in month %d, off the fiscal boundary", grain, day, fiscalStart, from.Month())
				}
				if grain != GrainQuarter {
					continue
				}
				spanFrom, spanTo, read := Span(GrainQuarter, quarterLabelOf(from, fiscalStart), zone, fiscalStart)
				if !read || !spanFrom.Equal(from) || !spanTo.Equal(to) {
					t.Fatalf("quarter of %s from %d: Containing [%s, %s), Span(%s) [%s, %s)",
						day, fiscalStart, from, to, quarterLabelOf(from, fiscalStart), spanFrom, spanTo)
				}
			}
		}
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
