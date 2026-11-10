// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

// The dates and periods a cited activity's own words name. A close date the
// model proposes applies only inside one of them. A mail cannot move a deal
// to a date it does not state.

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// period is a span of calendar days, both ends included.
type period struct{ start, end time.Time }

func (p period) holds(day time.Time) bool { return !day.Before(p.start) && !day.After(p.end) }

var (
	isoDay     = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	dottedDay  = regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})\.(\d{4})\b`)
	monthName  = regexp.MustCompile(`(?i)\b(` + monthAlternation() + `)\.?(?:\s+(\d{4}))?\b`)
	quarter    = regexp.MustCompile(`(?i)\bQ([1-4])(?:\s*/?\s*(\d{4}))?\b`)
	nextMonths = regexp.MustCompile(`(?i)\b(next month|nächsten monat)\b`)
	// "May" is also a verb, and "Jan" a first name. These two count only
	// beside a day or a year.
	mayOrJan = regexp.MustCompile(`\b(?:\d{1,2}(?:st|nd|rd|th)?\s+(May|Jan)\b\.?(?:\s+(\d{4}))?|(May|Jan)\.?\s+(?:\d{1,2}(?:st|nd|rd|th)?,?\s+)?(\d{4})\b|(May|Jan)\.?\s+\d{1,2}(?:st|nd|rd|th)?\b)`)
)

func monthAlternation() string {
	words := textlang.MonthWords()
	for i, word := range words {
		words[i] = regexp.QuoteMeta(word)
	}
	return strings.Join(words, "|")
}

// periodsIn reads every date and period the text names. A month or quarter
// with no year is the next one on or after the day the text arrived.
func periodsIn(text string, arrived time.Time) []period {
	var out []period
	for _, m := range isoDay.FindAllStringSubmatch(text, -1) {
		out = appendDay(out, atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	for _, m := range dottedDay.FindAllStringSubmatch(text, -1) {
		out = appendDay(out, atoi(m[3]), atoi(m[2]), atoi(m[1]))
	}
	for _, m := range monthName.FindAllStringSubmatch(text, -1) {
		month, ok := textlang.MonthWord(strings.ToLower(m[1]))
		if !ok {
			continue
		}
		year := yearOrNext(m[2], int(month), arrived)
		start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
		out = append(out, period{start: start, end: start.AddDate(0, 1, -1)})
	}
	for _, m := range mayOrJan.FindAllStringSubmatch(text, -1) {
		word, year := firstNonEmpty(m[1], m[3], m[5]), firstNonEmpty(m[2], m[4])
		month := map[string]int{"May": 5, "Jan": 1}[word]
		start := time.Date(yearOrNext(year, month, arrived), time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		out = append(out, period{start: start, end: start.AddDate(0, 1, -1)})
	}
	for _, m := range quarter.FindAllStringSubmatch(text, -1) {
		first := (atoi(m[1])-1)*3 + 1
		year := yearOrNext(m[2], first+2, arrived)
		start := time.Date(year, time.Month(first), 1, 0, 0, 0, 0, time.UTC)
		out = append(out, period{start: start, end: start.AddDate(0, 3, -1)})
	}
	if nextMonths.MatchString(text) {
		start := time.Date(arrived.Year(), arrived.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		out = append(out, period{start: start, end: start.AddDate(0, 1, -1)})
	}
	return out
}

func appendDay(out []period, year, month, day int) []period {
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Day() != day || int(date.Month()) != month {
		return out
	}
	return append(out, period{start: date, end: date})
}

// yearOrNext is the stated year, or the year in which the month whose number
// is last is still ahead of the arrival.
func yearOrNext(stated string, last int, arrived time.Time) int {
	if stated != "" {
		return atoi(stated)
	}
	if last < int(arrived.Month()) {
		return arrived.Year() + 1
	}
	return arrived.Year()
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// dateInEvidence reports whether a cited activity's words name a date or
// period the day falls in.
func dateInEvidence(in Input, cited []string, day time.Time) bool {
	known := in.activityByID()
	for _, id := range cited {
		activity, ok := known[id]
		if !ok {
			continue
		}
		for _, p := range periodsIn(activity.Subject+"\n"+activity.Body, activity.ArrivedAt) {
			if p.holds(day) {
				return true
			}
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
