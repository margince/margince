// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// An event's wall clock is read in the zone its TZID names. Google names an
// IANA zone. Outlook names a Windows zone ("W. Europe Standard Time") that no
// zone database holds, and states the zone's rules in a VTIMEZONE beside the
// event. So a name the database cannot load is read from those rules, and a
// name neither resolves yields no time rather than a wrong one.

import (
	"strconv"
	"strings"
	"time"
)

const (
	icalDateTime = "20060102T150405"
	icalDate     = "20060102"
	// maxZoneDates bounds the RDATE onsets one rule keeps from a sender.
	maxZoneDates = 64
)

// zoneRule is one STANDARD or DAYLIGHT block: from its onset each year the
// wall clock runs at offset seconds east of UTC.
type zoneRule struct {
	start  time.Time // DTSTART, a wall clock read as UTC
	month  time.Month
	nth    int // BYDAY's ordinal: 2 is the second, -1 the last
	day    time.Weekday
	yearly bool
	until  time.Time   // the rule's end; zero when it runs on
	dates  []time.Time // RDATE onsets, wall clocks read as UTC
	offset int
	stated bool // a rule whose offset did not parse says nothing
}

// onsetIn is when the rule takes over in year, as a wall clock read as UTC.
func (r zoneRule) onsetIn(year int) time.Time {
	if !r.yearly {
		return r.start
	}
	clock := time.Duration(r.start.Hour())*time.Hour + time.Duration(r.start.Minute())*time.Minute
	if r.nth > 0 {
		first := time.Date(year, r.month, 1, 0, 0, 0, 0, time.UTC)
		shift := (int(r.day) - int(first.Weekday()) + 7) % 7
		return first.AddDate(0, 0, shift+7*(r.nth-1)).Add(clock)
	}
	last := time.Date(year, r.month+1, 0, 0, 0, 0, 0, time.UTC)
	shift := (int(last.Weekday()) - int(r.day) + 7) % 7
	return last.AddDate(0, 0, -shift-7*(-r.nth-1)).Add(clock)
}

// statedZones are the VTIMEZONE blocks of one calendar, by TZID.
type statedZones map[string][]zoneRule

// offsetAt is the offset the latest rule to take over before wall gives, and
// false when the zone states no rule that has begun.
func (z statedZones) offsetAt(tzid string, wall time.Time) (int, bool) {
	var best time.Time
	offset, found := 0, false
	consider := func(onset time.Time, rule zoneRule) {
		if !onset.After(wall) && (!found || onset.After(best)) {
			best, offset, found = onset, rule.offset, true
		}
	}
	for _, rule := range z[tzid] {
		for _, year := range []int{wall.Year() - 1, wall.Year()} {
			if onset := rule.onsetIn(year); rule.until.IsZero() || !onset.After(rule.until) {
				consider(onset, rule)
			}
		}
		for _, onset := range rule.dates {
			consider(onset, rule)
		}
	}
	return offset, found
}

// readZones collects every VTIMEZONE of a calendar body.
func readZones(lines []icalLine) statedZones {
	zones := statedZones{}
	tzid := ""
	var rule *zoneRule
	for _, line := range lines {
		value := strings.TrimSpace(line.value)
		switch {
		case line.is("BEGIN") && strings.EqualFold(value, "VTIMEZONE"):
			tzid = ""
		case line.is("TZID") && rule == nil:
			tzid = value
		case line.is("BEGIN") && (strings.EqualFold(value, "STANDARD") || strings.EqualFold(value, "DAYLIGHT")):
			rule = &zoneRule{}
		case rule != nil && line.is("END"):
			if rule.stated {
				zones[tzid] = append(zones[tzid], *rule)
			}
			rule = nil
		case rule != nil:
			rule.read(line, value)
		}
	}
	return zones
}

// read takes one property of a STANDARD or DAYLIGHT block into the rule.
func (r *zoneRule) read(line icalLine, value string) {
	switch {
	case line.is("DTSTART"):
		if start, err := time.Parse(icalDateTime, value); err == nil {
			r.start = start
		}
	case line.is("TZOFFSETTO"):
		r.offset, r.stated = utcOffset(value)
	case line.is("RRULE"):
		readYearlyRule(r, value)
	case line.is("RDATE"):
		for part := range strings.SplitSeq(value, ",") {
			if at, err := time.Parse(icalDateTime, strings.TrimSpace(part)); err == nil && len(r.dates) < maxZoneDates {
				r.dates = append(r.dates, at)
			}
		}
	}
}

// readYearlyRule reads FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU, the one shape
// zone rules are written in. Any other leaves the rule a one-off.
func readYearlyRule(rule *zoneRule, value string) {
	parts := map[string]string{}
	for part := range strings.SplitSeq(value, ";") {
		key, v, _ := strings.Cut(part, "=")
		parts[strings.ToUpper(key)] = strings.ToUpper(v)
	}
	month, err := strconv.Atoi(parts["BYMONTH"])
	byday := parts["BYDAY"]
	if parts["FREQ"] != "YEARLY" || err != nil || month < 1 || month > 12 || len(byday) < 3 {
		return
	}
	weekday, ok := icalWeekdays[byday[len(byday)-2:]]
	nth, err := strconv.Atoi(byday[:len(byday)-2])
	if !ok || err != nil || nth == 0 || nth < -5 || nth > 5 {
		return
	}
	rule.month, rule.nth, rule.day, rule.yearly = time.Month(month), nth, weekday, true
	if until, err := time.Parse(icalDateTime, strings.TrimSuffix(parts["UNTIL"], "Z")); err == nil {
		rule.until = until
	}
}

var icalWeekdays = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

// utcOffset reads +0100, -0500 or +053000 as seconds east of UTC.
func utcOffset(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 5 && len(value) != 7 {
		return 0, false
	}
	sign := 1
	switch value[0] {
	case '-':
		sign = -1
	case '+':
	default:
		return 0, false
	}
	digits, err := strconv.Atoi(value[1:])
	if err != nil {
		return 0, false
	}
	if len(value) == 5 {
		digits *= 100
	}
	return sign * (digits/10000*3600 + digits/100%100*60 + digits%100), true
}

// icalTime reads a DTSTART or DTEND. allDay is a VALUE=DATE: a date, kept at
// midnight UTC so the date is what is stored. ok is false for a value or zone
// nothing resolves.
func icalTime(line icalLine, zones statedZones) (at time.Time, allDay, ok bool) {
	value := strings.TrimSpace(line.value)
	if strings.EqualFold(line.param("VALUE"), "DATE") || len(value) == len(icalDate) {
		day, err := time.Parse(icalDate, value)
		return day, true, err == nil
	}
	if utc, ok := strings.CutSuffix(value, "Z"); ok {
		at, err := time.Parse(icalDateTime, utc)
		return at, false, err == nil
	}
	wall, err := time.Parse(icalDateTime, value)
	tzid := strings.TrimSpace(line.param("TZID"))
	if err != nil || tzid == "" {
		// A floating time names no zone, so it names no instant either.
		return time.Time{}, false, false
	}
	if zone, err := time.LoadLocation(strings.TrimPrefix(tzid, "/")); err == nil {
		return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), 0, zone).UTC(), false, true
	}
	offset, found := zones.offsetAt(tzid, wall)
	if !found {
		return time.Time{}, false, false
	}
	return wall.Add(-time.Duration(offset) * time.Second), false, true
}
