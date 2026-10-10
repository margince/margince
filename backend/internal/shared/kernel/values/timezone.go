// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"strings"
	"time"
)

// Timezone is a validated IANA zone name. Parsing loads the location
// once, so a stored name can always be resolved again; "Local" is
// rejected because a persisted timezone must mean the same thing on
// every host.
type Timezone struct{ name string }

const zoneField = "timezone"

func ParseTimezone(name string) (Timezone, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || trimmed == "Local" {
		return Timezone{}, &ParseError{
			Field: zoneField, Code: "timezone_malformed",
			Message: "an IANA zone name is required (e.g. Europe/Berlin)",
		}
	}
	if _, err := time.LoadLocation(trimmed); err != nil {
		return Timezone{}, &ParseError{
			Field: zoneField, Code: "timezone_unknown",
			Message: trimmed + " is not a known IANA zone",
		}
	}
	return Timezone{name: trimmed}, nil
}

// ZoneName checks a zone name a writer stores as sent. ParseTimezone trims,
// and a stored name with a space around it would not load again.
func ZoneName(name string) error {
	zone, err := ParseTimezone(name)
	if err != nil {
		return err
	}
	if zone.String() != name {
		return &ParseError{
			Field: zoneField, Code: "timezone_malformed",
			Message: "an IANA zone name has no spaces around it",
		}
	}
	return nil
}

func (t Timezone) String() string { return t.name }
func (t Timezone) IsZero() bool   { return t.name == "" }

// Location resolves the zone; the name was validated at parse, so a
// failure here means the host lost its tzdata — a deployment fault
// surfaced loudly by the caller, not here.
func (t Timezone) Location() (*time.Location, error) {
	return time.LoadLocation(t.name)
}
