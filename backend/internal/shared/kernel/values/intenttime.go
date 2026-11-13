// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"regexp"
	"time"
)

// IntentWallClockLayout is how a chosen local time is written: no zone, and up
// to microseconds.
const IntentWallClockLayout = "2006-01-02T15:04:05.999999"

var localIntentFormat = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}(:[0-9]{2}(\.[0-9]{1,6})?)?$`)

// IntentTime keeps a chosen local time through timezone rule changes.
// Elapsed-duration timers use instants instead.
type IntentTime struct {
	wall time.Time
	zone Timezone
}

// ParseIntentTime reads a local wall clock in zone. Seconds and fractions are
// optional, and the zone must be set.
func ParseIntentTime(wall string, zone Timezone) (IntentTime, error) {
	if zone.IsZero() || !localIntentFormat.MatchString(wall) {
		return IntentTime{}, invalidIntent()
	}
	layout := IntentWallClockLayout
	if len(wall) == 16 {
		layout = "2006-01-02T15:04"
	}
	parsed, err := time.Parse(layout, wall)
	if err != nil || parsed.Year() < 1 {
		return IntentTime{}, invalidIntent()
	}
	return IntentTime{wall: parsed, zone: zone}, nil
}

// IsZero reports an IntentTime that holds no choice.
func (i IntentTime) IsZero() bool { return i.zone.IsZero() }

// WallClock is the chosen local time in IntentWallClockLayout.
func (i IntentTime) WallClock() string { return i.wall.Format(IntentWallClockLayout) }

// Timezone is the zone the local time was chosen in.
func (i IntentTime) Timezone() Timezone { return i.zone }

// Resolve reloads current rules. Gaps move to their first valid instant;
// repeated local times use their first occurrence.
func (i IntentTime) Resolve() (time.Time, error) {
	if i.IsZero() {
		return time.Time{}, invalidIntent()
	}
	loc, err := i.zone.Location()
	if err != nil {
		return time.Time{}, err
	}
	instant, err := resolveIntent(i.wall, loc)
	if err != nil {
		return time.Time{}, err
	}
	if instant.Year() < 1 || instant.Year() > 9999 {
		return time.Time{}, invalidIntent()
	}
	return instant, nil
}

func invalidIntent() *ParseError {
	return &ParseError{Field: "wall_clock", Code: "datetime_malformed", Message: "choose a valid local date and time with an IANA timezone"}
}
