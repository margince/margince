// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

// The dates a sentence can state, in the forms this product's three languages
// actually write them.
//
// Sources speak one dialect — RFC3339, because that is what the assemblers put
// in front of the model — and sentences speak the reader's. So the two sides
// are normalised to the same triple and compared there. Anything else drops a
// German brief's every dated sentence on the day it ships.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// datesIn returns the dates the text states, and the text with each of them
// blanked out so the number scan cannot read a date's parts as figures.
func datesIn(text string) ([]Specific, string) {
	rest := text
	var found []Specific
	for _, form := range dateForms {
		for _, at := range form.pattern.FindAllStringSubmatchIndex(rest, -1) {
			groups := groupsAt(rest, at)
			date, ok := form.read(groups)
			if !ok {
				continue
			}
			date.Text = strings.TrimSpace(rest[at[0]:at[1]])
			found = append(found, date)
		}
		rest = form.pattern.ReplaceAllString(rest, " ")
	}
	return found, rest
}

// groupsAt lifts a submatch index list into the strings it names, with an
// absent optional group reading as empty.
func groupsAt(text string, at []int) []string {
	groups := make([]string, 0, len(at)/2)
	for i := 0; i < len(at); i += 2 {
		if at[i] < 0 {
			groups = append(groups, "")
			continue
		}
		groups = append(groups, text[at[i]:at[i+1]])
	}
	return groups
}

// dateForm is one written shape and how to read a date out of it.
type dateForm struct {
	pattern *regexp.Regexp
	read    func(groups []string) (Specific, bool)
}

// dateForms are tried in order, most specific first: an RFC3339 instant is
// blanked before the numeric form could read "06-02" out of what is left of it.
var dateForms = []dateForm{
	// 2026-06-02, and the date half of 2026-06-02T09:00:00Z.
	{
		pattern: regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})`),
		read: func(g []string) (Specific, bool) {
			return dayKeys(g[1], g[2], g[3])
		},
	},
	// 02.06.2026, 2.6.2026, 02/06/2026 — day-first, which is what all three of
	// this product's locales write, and month-first as the alternative when the
	// two could be swapped. A checker that assumed one order would call a true
	// sentence a lie on every day of the month below the thirteenth.
	{
		pattern: regexp.MustCompile(`\b(\d{1,2})[./](\d{1,2})[./](\d{4})\b`),
		read: func(g []string) (Specific, bool) {
			return ambiguousOrder(g[1], g[2], g[3])
		},
	},
	// ngày 2 tháng 6 — Vietnamese, day before month, both spelled out.
	{
		pattern: regexp.MustCompile(`(?i)\bngày\s+(\d{1,2})\s+tháng\s+(\d{1,2})\b`),
		read: func(g []string) (Specific, bool) {
			return dayKeys("", g[2], g[1])
		},
	},
	// 2 June 2024, 2. Juni 2024 — the year first, so the day-and-month form
	// below cannot read the day out and leave the year behind as a stray
	// number, which is how a sentence that moved an event two years survives.
	{
		pattern: regexp.MustCompile(`(?i)\b(\d{1,2})\.?\s+(` + monthAlternatives + `)\s+(\d{4})\b`),
		read: func(g []string) (Specific, bool) {
			return dayKeys(g[3], monthNumber(g[2]), g[1])
		},
	},
	// June 2, 2024
	{
		pattern: regexp.MustCompile(`(?i)\b(` + monthAlternatives + `)\s+(\d{1,2}),?\s+(\d{4})\b`),
		read: func(g []string) (Specific, bool) {
			return dayKeys(g[3], monthNumber(g[1]), g[2])
		},
	},
	// 2 June, 2. Juni, 2 Jun
	{
		pattern: regexp.MustCompile(`(?i)\b(\d{1,2})\.?\s+(` + monthAlternatives + `)\b`),
		read: func(g []string) (Specific, bool) {
			return dayKeys("", monthNumber(g[2]), g[1])
		},
	},
	// June 2, Jun 2 — and never "June 2026", which the year guard below refuses
	// so it is read as the month it is.
	{
		pattern: regexp.MustCompile(`(?i)\b(` + monthAlternatives + `)\s+(\d{1,2})\b`),
		read: func(g []string) (Specific, bool) {
			return dayKeys("", monthNumber(g[1]), g[2])
		},
	},
	// tháng 6 — Vietnamese month with no day.
	{
		pattern: regexp.MustCompile(`(?i)\btháng\s+(\d{1,2})\b`),
		read: func(g []string) (Specific, bool) {
			return monthKey(g[1])
		},
	},
	// June, Juni — a month with no day is still a fact a sentence can invent.
	{
		pattern: regexp.MustCompile(`(?i)\b(` + monthAlternatives + `)\b`),
		read: func(g []string) (Specific, bool) {
			return monthKey(monthNumber(g[1]))
		},
	},
}

// dayKeys is one date read as day-of-month, with the year when it was written.
func dayKeys(year, month, day string) (Specific, bool) {
	m, d, ok := monthDay(month, day)
	if !ok {
		return Specific{}, false
	}
	if year == "" {
		return Specific{Keys: []string{fmt.Sprintf("date:%02d-%02d", m, d)}}, true
	}
	return Specific{Keys: []string{fmt.Sprintf("date:%s-%02d-%02d", year, m, d)}}, true
}

// ambiguousOrder reads a numeric date both ways round when both readings are
// possible, and one way when only one is.
func ambiguousOrder(first, second, year string) (Specific, bool) {
	dayFirst, dayFirstOK := dayKeys(year, second, first)
	monthFirst, monthFirstOK := dayKeys(year, first, second)
	switch {
	case dayFirstOK && monthFirstOK:
		return Specific{Keys: append(dayFirst.Keys, monthFirst.Keys...)}, true
	case dayFirstOK:
		return dayFirst, true
	case monthFirstOK:
		return monthFirst, true
	}
	return Specific{}, false
}

// monthKey is a month named with no day.
func monthKey(month string) (Specific, bool) {
	m, err := strconv.Atoi(month)
	if err != nil || m < 1 || m > 12 {
		return Specific{}, false
	}
	return Specific{Keys: []string{fmt.Sprintf("date:%02d", m)}}, true
}

// monthDay refuses anything that is not a real calendar position, so "45/13"
// is read as two numbers rather than admitted as a date nothing can match.
func monthDay(month, day string) (int, int, bool) {
	m, err := strconv.Atoi(month)
	if err != nil || m < 1 || m > 12 {
		return 0, 0, false
	}
	d, err := strconv.Atoi(day)
	if err != nil || d < 1 || d > 31 {
		return 0, 0, false
	}
	return m, d, true
}

// lessSpecific is what a SOURCE date also answers: a dated row answers a
// sentence naming the same day without its year, and one naming only the month.
func lessSpecific(key string) []string {
	parts := strings.Split(strings.TrimPrefix(key, "date:"), "-")
	if !strings.HasPrefix(key, "date:") {
		return nil
	}
	switch len(parts) {
	case 3:
		return []string{"date:" + parts[1] + "-" + parts[2], "date:" + parts[1]}
	case 2:
		return []string{"date:" + parts[0]}
	}
	return nil
}
