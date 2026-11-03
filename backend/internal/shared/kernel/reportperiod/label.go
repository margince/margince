// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportperiod

import (
	"regexp"
	"strconv"
	"time"
)

// Grain is how long one period bucket lasts.
type Grain string

// The three grains a period report cuts.
const (
	GrainYear    Grain = "year"
	GrainQuarter Grain = "quarter"
	GrainMonth   Grain = "month"
)

// A January year is spelled `2026`. Any other fiscal year names both calendar
// years it spans, `FY2025/26`, and its quarters follow it, `FY2025/26-Q4`.
var (
	yearLabel    = regexp.MustCompile(`^(?:(\d{4})|FY(\d{4})/(\d{2}))$`)
	quarterLabel = regexp.MustCompile(`^(?:(\d{4})|FY(\d{4})/(\d{2}))-Q([1-4])$`)
	monthLabel   = regexp.MustCompile(`^(\d{4})-(0[1-9]|1[0-2])$`)
)

// Label is a period bucket's label, read.
type Label struct {
	// Year is the calendar year the label's year begins in.
	Year int
	// Quarter is 1..4 on a quarter label and 0 otherwise.
	Quarter int
	// Month is 1..12 on a month label and 0 otherwise.
	Month int
	// Fiscal is true for the spanning `FY2025/26` spelling.
	Fiscal bool
}

// ParseLabel reads a label in the spellings a period bucket renders. A label
// no bucket can render is refused, such as `FY2025/99`, whose second year does
// not follow its first.
func ParseLabel(grain Grain, label string) (Label, bool) {
	switch grain {
	case GrainYear:
		parts := yearLabel.FindStringSubmatch(label)
		if parts == nil {
			return Label{}, false
		}
		return spannedYear(parts[1], parts[2], parts[3])
	case GrainQuarter:
		parts := quarterLabel.FindStringSubmatch(label)
		if parts == nil {
			return Label{}, false
		}
		read, ok := spannedYear(parts[1], parts[2], parts[3])
		read.Quarter = digits(parts[4])
		return read, ok
	case GrainMonth:
		parts := monthLabel.FindStringSubmatch(label)
		if parts == nil {
			return Label{}, false
		}
		return Label{Year: digits(parts[1]), Month: digits(parts[2])}, true
	}
	return Label{}, false
}

// spannedYear reads the year half of a label: a plain year, or a fiscal span
// whose second year must follow its first.
func spannedYear(plain, first, second string) (Label, bool) {
	if plain != "" {
		return Label{Year: digits(plain)}, true
	}
	start := digits(first)
	return Label{Year: start, Fiscal: true}, (start+1)%100 == digits(second)
}

// digits reads digits a label pattern already matched.
func digits(text string) int {
	n, err := strconv.Atoi(text)
	if err != nil {
		return -1
	}
	return n
}

// Span answers the half-open interval a label names in one calendar: the zone
// and the month its fiscal year begins. The spelling must be the one that
// calendar renders, so a fiscal label under a January start names nothing.
func Span(grain Grain, label string, zone *time.Location, fiscalStart int) (from, to time.Time, ok bool) {
	read, parsed := ParseLabel(grain, label)
	if !parsed || zone == nil || fiscalStart < 1 || fiscalStart > 12 {
		return from, to, false
	}
	if grain != GrainMonth && read.Fiscal != (fiscalStart != 1) {
		return from, to, false
	}
	month, months := fiscalStart, 12
	switch grain {
	case GrainQuarter:
		month, months = fiscalStart+3*(read.Quarter-1), 3
	case GrainMonth:
		month, months = read.Month, 1
	}
	from = time.Date(read.Year, time.Month(month), 1, 0, 0, 0, 0, zone)
	return from, from.AddDate(0, months, 0), true
}
