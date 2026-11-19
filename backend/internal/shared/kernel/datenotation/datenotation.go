// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package datenotation writes a day and a clock time in the installation's
// notation, for server-written text a reader sees beside the app's own dates.
//
// The patterns mirror frontend/src/format/preferences.ts and the locale
// defaults the browser derives; backend/gates/frontenddatenotation_test.go
// fails when the two disagree.
package datenotation

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Notation is how one installation writes dates. It holds the date and time
// formats, the language a "locale" format follows, and the display zone.
type Notation struct {
	Lang textlang.Lang
	// Date is locale, dmy, mdy or ymd; anything else reads as locale.
	Date string
	// Time is locale, 24h or 12h; anything else reads as locale.
	Time string
	Zone *time.Location
}

// Pattern is the order of a day's parts and the mark between them.
type Pattern struct {
	Order     [3]string
	Separator string
}

const (
	day   = "day"
	month = "month"
	year  = "year"
)

var (
	dayFirstDot   = Pattern{Order: [3]string{day, month, year}, Separator: "."}
	dayFirstSlash = Pattern{Order: [3]string{day, month, year}, Separator: "/"}
)

// FixedPatterns are the patterns an installation can name.
func FixedPatterns() map[string]Pattern {
	return map[string]Pattern{
		"dmy": dayFirstDot,
		"mdy": {Order: [3]string{month, day, year}, Separator: "/"},
		"ymd": {Order: [3]string{year, month, day}, Separator: "-"},
	}
}

// LocalePatterns are what the "locale" format writes in each shipped language.
func LocalePatterns() map[textlang.Lang]Pattern {
	return map[textlang.Lang]Pattern{
		textlang.English:    dayFirstSlash,
		textlang.German:     dayFirstDot,
		textlang.Vietnamese: dayFirstSlash,
	}
}

// Day writes the calendar day an instant falls on in the notation's zone.
func (n Notation) Day(at time.Time) string {
	return n.CalendarDay(at.In(n.zone()))
}

// CalendarDay writes a stored day as it is, with no zone shift.
func (n Notation) CalendarDay(date time.Time) string {
	parts := map[string]string{
		day:   date.Format("02"),
		month: date.Format("01"),
		year:  date.Format("2006"),
	}
	pattern := n.pattern()
	written := make([]string, 0, len(pattern.Order))
	for _, part := range pattern.Order {
		written = append(written, parts[part])
	}
	return strings.Join(written, pattern.Separator)
}

// Clock writes the time of day an instant shows in the notation's zone. Every
// shipped language writes a 24-hour clock unless the installation names 12h.
func (n Notation) Clock(at time.Time) string {
	local := at.In(n.zone())
	if n.Time == "12h" {
		return local.Format("3:04 PM")
	}
	return local.Format("15:04")
}

func (n Notation) pattern() Pattern {
	if fixed, ok := FixedPatterns()[n.Date]; ok {
		return fixed
	}
	if byLocale, ok := LocalePatterns()[n.Lang]; ok {
		return byLocale
	}
	return LocalePatterns()[textlang.English]
}

func (n Notation) zone() *time.Location {
	if n.Zone == nil {
		return time.UTC
	}
	return n.Zone
}
