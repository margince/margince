// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const paramTaskTimezone = "timezone"

func taskTimezoneSchema() map[string]any {
	return map[string]any{
		schemaKeyType:        schemaTypeString,
		schemaKeyFormat:      "timezone",
		schemaKeyDescription: "IANA timezone for calendar-day deadlines. Without it, the existing deadline is preserved.",
	}
}

func validateTaskTimezone(params map[string]any) error {
	value, present := params[paramTaskTimezone]
	if !present {
		return nil
	}
	name, ok := value.(string)
	if !ok {
		return &ParamError{Field: "params." + paramTaskTimezone, Reason: "must be an IANA timezone name"}
	}
	if err := values.ZoneName(name); err != nil {
		return &ParamError{Field: "params." + paramTaskTimezone, Reason: "must be a valid IANA timezone name"}
	}
	return nil
}

func taskDueAt(params json.RawMessage, received time.Time, defaultDays int) (time.Time, error) {
	days, err := DueInDays(params, defaultDays)
	if err != nil {
		return time.Time{}, err
	}
	var deadline struct {
		Timezone *string `json:"timezone"`
	}
	if len(params) != 0 {
		if err := json.Unmarshal(params, &deadline); err != nil {
			return time.Time{}, fmt.Errorf("automation: reading deadline: %w", err)
		}
	}
	if deadline.Timezone == nil {
		return received.AddDate(0, 0, days), nil
	}
	if err := validateTaskTimezone(map[string]any{paramTaskTimezone: *deadline.Timezone}); err != nil {
		return time.Time{}, err
	}
	zone, err := time.LoadLocation(*deadline.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("automation: loading deadline timezone: %w", err)
	}
	local := received.In(zone)
	followingDay := time.Date(local.Year(), local.Month(), local.Day()+days+1, 0, 0, 0, 0, zone)
	// PostgreSQL stores microseconds; a nanosecond before midnight can round into the next date.
	return followingDay.Add(-time.Microsecond).UTC(), nil
}
