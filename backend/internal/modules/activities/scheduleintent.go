// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const scheduleInstantSQL = "COALESCE(public.resolve_intent_time(scheduled_wall, scheduled_tz), scheduled_at)"

func scheduleFromIntent(raw *crmcontracts.IntentTime) (*SendSchedule, error) {
	zone, err := values.ParseTimezone(raw.Timezone)
	if err != nil {
		return nil, &InvalidScheduleError{Field: "scheduled_intent.timezone", Reason: "must name an IANA timezone"}
	}
	intent, err := values.ParseIntentTime(raw.WallClock, zone)
	if err != nil {
		return nil, &InvalidScheduleError{Field: "scheduled_intent.wall_clock", Reason: "must be a valid local date and time"}
	}
	schedule, err := normalizeSchedule(SendSchedule{Intent: &intent})
	if err != nil {
		return nil, err
	}
	return &schedule, nil
}

func normalizeSchedule(schedule SendSchedule) (SendSchedule, error) {
	if schedule.Intent == nil {
		return schedule, nil
	}
	instant, err := schedule.Intent.Resolve()
	if err != nil {
		var parseError *values.ParseError
		if errors.As(err, &parseError) {
			return SendSchedule{}, &InvalidScheduleError{Field: "scheduled_intent.wall_clock", Reason: "must resolve to a supported date and time"}
		}
		return SendSchedule{}, err
	}
	schedule.At, schedule.TZ = instant, schedule.Intent.Timezone().String()
	return schedule, nil
}

func (schedule SendSchedule) wallClock() *string {
	if schedule.Intent == nil {
		return nil
	}
	wall := schedule.Intent.WallClock()
	return &wall
}

func (schedule SendSchedule) IntentWire() *crmcontracts.IntentTime {
	if schedule.Intent == nil {
		return nil
	}
	return &crmcontracts.IntentTime{WallClock: schedule.Intent.WallClock(), Timezone: schedule.Intent.Timezone().String()}
}

func (row ScheduledSend) IntentWire() *crmcontracts.IntentTime {
	if row.ScheduledWall == nil {
		return nil
	}
	return &crmcontracts.IntentTime{WallClock: row.ScheduledWall.Format(values.IntentWallClockLayout), Timezone: row.ScheduledTZ}
}

func readSchedule(at *time.Time, zone *string, wall *time.Time) (*SendSchedule, error) {
	if at == nil || zone == nil {
		return nil, nil
	} //nolint:nilnil // an absent schedule means send now
	schedule := &SendSchedule{At: *at, TZ: *zone}
	if wall != nil {
		timezone, err := values.ParseTimezone(*zone)
		if err != nil {
			return nil, err
		}
		intent, err := values.ParseIntentTime(wall.Format(values.IntentWallClockLayout), timezone)
		if err != nil {
			return nil, err
		}
		schedule.Intent = &intent
	}
	return schedule, nil
}

// validateSchedule refuses a due moment the server will not honour.
func validateSchedule(sched SendSchedule, now time.Time) error {
	if sched.TZ == "" {
		return &InvalidScheduleError{Field: FieldScheduledTZ, Reason: "is required when scheduling a send"}
	}
	// A zone NAME, never a numeric offset, which would freeze the DST rules of
	// the day it was written (AC-DS-TZ4). "Local" is the server's zone, so it
	// is refused, and so is a name with spaces around it.
	if err := values.ZoneName(sched.TZ); err != nil {
		return &InvalidScheduleError{Field: FieldScheduledTZ, Reason: "is not an IANA time zone name"}
	}
	if sched.At.Sub(now) > scheduleCeiling {
		return &InvalidScheduleError{
			Field:  FieldScheduledAt,
			Reason: fmt.Sprintf("is further ahead than the %d-day scheduling limit", int(scheduleCeiling.Hours()/24)),
		}
	}
	return nil
}
