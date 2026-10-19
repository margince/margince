// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Date formats a display date without changing its reporting boundary.
func (c Copy) Date(at time.Time) string {
	switch c.Language {
	case German:
		return at.Format("02.01.2006")
	case Vietnamese:
		return at.Format("02/01/2006")
	default:
		return at.Format("2 Jan 2006")
	}
}

// Instant retains the actual deadline and its timezone.
func (c Copy) Instant(at time.Time) string {
	clock := at.Format("15:04")
	if c.Language == German {
		clock += " Uhr"
	}
	return c.Date(at) + ", " + clock + " (" + zoneOffset(at) + ")"
}

// Money groups the canonical minor-unit conversion without floating-point rounding.
func (c Copy) Money(minor int64, currency string) string {
	whole, fraction, decimal := strings.Cut(values.MajorUnits(minor, currency), ".")
	sign := ""
	if strings.HasPrefix(whole, "-") {
		sign, whole = "-", whole[1:]
	}
	group, point := ",", "."
	if c.Language == German || c.Language == Vietnamese {
		group, point = ".", ","
	}
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + group + whole[i:]
	}
	if decimal {
		whole += point + fraction
	}
	return sign + whole + " " + currency
}

func zoneOffset(at time.Time) string {
	_, offset := at.Zone()
	if offset == 0 {
		return "UTC"
	}
	return "UTC" + at.Format("-07:00")
}

// DateRange keeps display ranges in the recipient's language.
func (c Copy) DateRange(start, end time.Time) string {
	return c.Date(start) + c.rangeSeparator() + c.Date(end)
}

func (c Copy) rangeSeparator() string {
	switch c.Language {
	case German:
		return " bis "
	case Vietnamese:
		return " đến "
	default:
		return " to "
	}
}

func (c Copy) meetingInterval(start, end time.Time) string {
	sy, sm, sd := start.Date()
	ey, em, ed := end.Date()
	if sy != ey || sm != em || sd != ed || zoneOffset(start) != zoneOffset(end) {
		return c.Instant(start) + c.rangeSeparator() + c.Instant(end)
	}
	clocks := start.Format("15:04") + c.rangeSeparator() + end.Format("15:04")
	if c.Language == German {
		clocks += " Uhr"
	}
	return c.Date(start) + ", " + clocks + " (" + zoneOffset(start) + ")"
}
