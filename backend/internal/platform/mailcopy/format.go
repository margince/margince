// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

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
	return c.Date(at) + ", " + c.clock(at) + " (" + zoneOffset(at) + ")"
}

// CountryInstant is Instant read on a clock in country, named for the reader as
// that country's time: "28/11/2026, 15:21 (giờ Việt Nam, UTC+07:00)". A code
// no language can name prints the plain Instant.
func (c Copy) CountryInstant(at time.Time, country string) string {
	region, err := language.ParseRegion(strings.ToUpper(country))
	if err != nil {
		return c.Instant(at)
	}
	name := display.Regions(language.Make(string(c.Language))).Name(region)
	if name == "" {
		return c.Instant(at)
	}
	return c.Date(at) + ", " + c.clock(at) + " (" + fmt.Sprintf(c.ZoneOfCountry, name) + ", " + zoneOffset(at) + ")"
}

func (c Copy) clock(at time.Time) string {
	if c.Language == German {
		return at.Format("15:04") + " Uhr"
	}
	return at.Format("15:04")
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
