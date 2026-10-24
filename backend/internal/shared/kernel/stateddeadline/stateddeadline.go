// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package stateddeadline reads the one day a message states as its deadline.
//
// It reads words and never guesses. A day the text does not name gives no day.
// Neither does a slashed date whose day and month could swap, or a range wider
// than one week. The caller decides when that day ends.
package stateddeadline

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Day is a calendar day with no zone; the caller ends it in its own zone.
type Day struct {
	Year  int
	Month time.Month
	Day   int
}

// String is the day as YYYY-MM-DD.
func (d Day) String() string {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

func dayOf(t time.Time) Day { return Day{t.Year(), t.Month(), t.Day()} }

var months = map[string]time.Month{
	"jan": time.January, "january": time.January, "januar": time.January, "jänner": time.January,
	"feb": time.February, "february": time.February, "februar": time.February,
	"mar": time.March, "march": time.March, "mär": time.March, "märz": time.March, "maerz": time.March,
	"apr": time.April, "april": time.April,
	"may": time.May, "mai": time.May,
	"jun": time.June, "june": time.June, "juni": time.June,
	"jul": time.July, "july": time.July, "juli": time.July,
	"aug": time.August, "august": time.August,
	"sep": time.September, "sept": time.September, "september": time.September,
	"oct": time.October, "october": time.October, "okt": time.October, "oktober": time.October,
	"nov": time.November, "november": time.November,
	"dec": time.December, "december": time.December, "dez": time.December, "dezember": time.December,
}

const monthWord = `([a-zäöü]{3,9})\.?`

// keyword is a word that introduces the deadline itself. "Before" is left
// out: "before Friday" names the day after the deadline.
const keyword = `\b(?:deadline|due(?:\s+(?:date|on|by))?|by|until|till|no\s+later\s+than|bis(?:\s+(?:zum|spätestens))?|spätestens|frist|fällig(?:\s+am)?|hạn(?:\s+chót)?)\b`

var (
	isoDate     = regexp.MustCompile(`(?i)` + keyword + `\s*[:\-–]?\s*(?:\w+,?\s+)?(\d{4})-(\d{2})-(\d{2})\b`)
	dottedDate  = regexp.MustCompile(`(?i)` + keyword + `\s*[:\-–]?\s*(?:\w+,?\s+)?(\d{1,2})\.(\d{1,2})\.(\d{4})\b`)
	dayMonth    = regexp.MustCompile(`(?i)` + keyword + `\s*[:\-–]?\s*(?:(?:the\s+)?\w+,?\s+)?(\d{1,2})(?:st|nd|rd|th|\.)?\s+(?:of\s+)?` + monthWord + `(?:,?\s+(\d{4}))?`)
	monthDay    = regexp.MustCompile(`(?i)` + keyword + `\s*[:\-–]?\s*(?:\w+,?\s+)?` + monthWord + `\s+(\d{1,2})(?:st|nd|rd|th)?(?:,?\s+(\d{4}))?`)
	thisWeekEnd = regexp.MustCompile(`(?i)\b(?:this\s+week|(?:by\s+)?(?:the\s+)?end\s+of\s+(?:this|the)\s+week|diese\s+woche|(?:bis\s+)?ende\s+(?:dieser|der)\s+woche|tuần\s+này|cuối\s+tuần\s+này)\b`)
)

// In reads the deadline a subject or body states after a deadline word, as of
// the day the message was sent. A day with no year is the first such day on
// or after the sent day. False when the text states no single valid day.
func In(text string, sent Day) (Day, bool) {
	text = strings.ToLower(text)
	if m := isoDate.FindStringSubmatch(text); m != nil {
		return valid(atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	if m := dottedDate.FindStringSubmatch(text); m != nil {
		return valid(atoi(m[3]), atoi(m[2]), atoi(m[1]))
	}
	if m := dayMonth.FindStringSubmatch(text); m != nil {
		if month, ok := months[m[2]]; ok {
			return withYear(sent, m[3], int(month), atoi(m[1]))
		}
	}
	if m := monthDay.FindStringSubmatch(text); m != nil {
		if month, ok := months[m[1]]; ok {
			return withYear(sent, m[3], int(month), atoi(m[2]))
		}
	}
	return Day{}, false
}

// ThisWeek is the last working day of the week the message was sent, when the
// words promise it "this week". Weeks start on Monday; workdays are ISO
// weekday numbers (1 = Monday), and none means Monday to Friday.
func ThisWeek(words string, sent Day, workdays []int) (Day, bool) {
	if !thisWeekEnd.MatchString(strings.ToLower(words)) {
		return Day{}, false
	}
	last := 5
	if len(workdays) > 0 {
		last = slices.Max(workdays)
	}
	day := time.Date(sent.Year, sent.Month, sent.Day, 12, 0, 0, 0, time.UTC)
	sinceMonday := (int(day.Weekday()) + 6) % 7
	return dayOf(day.AddDate(0, 0, last-1-sinceMonday)), true
}

// SentDay is the calendar day an instant falls on in zone.
func SentDay(at time.Time, zone *time.Location) Day { return dayOf(at.In(zone)) }

func withYear(sent Day, year string, month, day int) (Day, bool) {
	if year != "" {
		return valid(atoi(year), month, day)
	}
	got, ok := valid(sent.Year, month, day)
	if ok && got.before(sent) {
		got, ok = valid(sent.Year+1, month, day)
	}
	return got, ok
}

func (d Day) before(other Day) bool {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Before(time.Date(other.Year, other.Month, other.Day, 0, 0, 0, 0, time.UTC))
}

// valid refuses a day the calendar does not have, such as 31 April, rather
// than letting time.Date roll it into May.
func valid(year, month, day int) (Day, bool) {
	if month < 1 || month > 12 || day < 1 {
		return Day{}, false
	}
	at := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if at.Day() != day || int(at.Month()) != month {
		return Day{}, false
	}
	return dayOf(at), true
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
