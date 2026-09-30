// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func location(calendar Calendar) (*time.Location, error) {
	if calendar.FiscalStartMonth < 1 || calendar.FiscalStartMonth > 12 {
		return nil, invalid("reporting fiscal calendar is unavailable")
	}
	zone, err := time.LoadLocation(calendar.Timezone)
	if err != nil {
		return nil, invalid("reporting timezone is unavailable")
	}
	return zone, nil
}

// Interval resolves relative periods in the installation calendar.
func Interval(selection crmcontracts.ReportingSelection, calendar Calendar, at time.Time) (crmcontracts.ReportingWindow, error) {
	zone, err := location(calendar)
	if err != nil {
		return crmcontracts.ReportingWindow{}, err
	}
	day := at.In(zone)
	start := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, zone)
	end := at
	switch selection.Period {
	case reportingThisMonth:
	case "last_month":
		end = start
		start = start.AddDate(0, -1, 0)
	case "last_week":
		monday := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -(int(day.Weekday())+6)%7)
		start, end = monday.AddDate(0, 0, -7), monday
	case "this_quarter":
		start = quarterStart(day, calendar.FiscalStartMonth)
	case "custom":
		if selection.Interval == nil {
			return crmcontracts.ReportingWindow{}, invalid("choose the reporting interval")
		}
		start, end = selection.Interval.StartAt.In(zone), selection.Interval.EndAt.In(zone)
	default:
		return crmcontracts.ReportingWindow{}, invalid("choose a supported reporting period")
	}
	if !end.After(start) || end.After(at) || end.After(start.AddDate(1, 0, 0)) {
		return crmcontracts.ReportingWindow{}, invalid("choose a past interval of no more than twelve months")
	}
	return crmcontracts.ReportingWindow{StartAt: start, EndAt: end}, nil
}

func quarterStart(day time.Time, fiscalStart int) time.Time {
	offset := (int(day.Month()) - fiscalStart + 12) % 12
	return time.Date(day.Year(), day.Month()-time.Month(offset%3), 1, 0, 0, 0, 0, day.Location())
}

// TargetWindow uses the complete containing period for target attainment.
func TargetWindow(interval crmcontracts.ReportingWindow, basis string, calendar Calendar) (crmcontracts.ReportingWindow, error) {
	zone, err := location(calendar)
	if err != nil {
		return crmcontracts.ReportingWindow{}, err
	}
	day := interval.EndAt.Add(-time.Nanosecond).In(zone)
	start := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, zone)
	months := 1
	switch basis {
	case "month":
	case reportingFiscalQuarter:
		start = quarterStart(day, calendar.FiscalStartMonth)
		months = 3
	default:
		return crmcontracts.ReportingWindow{}, invalid("choose month or fiscal quarter for targets")
	}
	return crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, months, 0)}, nil
}

// NextDue resolves local run times across DST gaps and repeated hours.
func NextDue(rule crmcontracts.ReportingScheduleInput, timezone string, after time.Time) (time.Time, error) {
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, invalid("choose a valid reporting timezone")
	}
	minutes, err := scheduleMinutes(rule)
	if err != nil {
		return time.Time{}, err
	}
	local := after.In(zone)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	for i := 0; i < 63; i++ {
		candidateDay := day.AddDate(0, 0, i)
		if !scheduledDay(rule, candidateDay) {
			continue
		}
		candidate := localRunTime(candidateDay, minutes)
		if candidate.After(after) {
			return candidate, nil
		}
	}
	return time.Time{}, invalid("the schedule has no next run")
}

func scheduleMinutes(rule crmcontracts.ReportingScheduleInput) (int, error) {
	if rule.Day < 1 || (rule.Frequency == reportingWeekly && rule.Day > 7) || rule.Day > 31 {
		return 0, invalid("choose a valid run day")
	}
	if rule.Frequency != reportingWeekly && rule.Frequency != "monthly" {
		return 0, invalid("choose weekly or monthly")
	}
	return reportingClockMinutes(rule.LocalTime)
}

func scheduledDay(rule crmcontracts.ReportingScheduleInput, day time.Time) bool {
	if rule.Frequency == reportingWeekly {
		return (int(day.Weekday())+6)%7+1 == rule.Day
	}
	last := time.Date(day.Year(), day.Month()+1, 0, 0, 0, 0, 0, day.Location()).Day()
	return day.Day() == min(rule.Day, last)
}

// Walking instants selects the first repeated clock time and the next valid
// minute after a DST gap; time.Date alone does not promise either policy.
func localRunTime(day time.Time, minute int) time.Time {
	for instant := day; instant.Before(day.AddDate(0, 0, 1)); instant = instant.Add(time.Minute) {
		local := instant.In(day.Location())
		if local.Hour()*60+local.Minute() >= minute {
			return instant
		}
	}
	return day.AddDate(0, 0, 1)
}

func invalid(message string) error {
	return fmt.Errorf("%s: %w", message, apperrors.ErrInvalidArgument)
}

func reportingClockMinutes(localTime string) (int, error) {
	clock := strings.Split(localTime, ":")
	if len(clock) != 2 || len(clock[0]) != 2 || len(clock[1]) != 2 {
		return 0, invalid("enter a time as HH:MM")
	}
	hour, err := strconv.Atoi(clock[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, invalid("enter a valid hour")
	}
	minute, err := strconv.Atoi(clock[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, invalid("enter a valid minute")
	}
	return hour*60 + minute, nil
}
