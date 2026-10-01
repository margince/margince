// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func reportingPrevious(frame crmcontracts.ReportingContext) crmcontracts.ReportingWindow {
	start, end := frame.Interval.StartAt, frame.Interval.EndAt
	kind := frame.PeriodKind
	if kind == "custom" || kind == "" {
		switch {
		case start.Day() == 1 && end.Equal(start.AddDate(0, 1, 0)):
			kind = "last_month"
		case start.Weekday() == time.Monday && end.Equal(start.AddDate(0, 0, 7)):
			kind = "last_week"
		default:
			return crmcontracts.ReportingWindow{}
		}
	}
	if kind == "last_week" {
		return crmcontracts.ReportingWindow{StartAt: start.AddDate(0, 0, -7), EndAt: start}
	}
	months := 1
	if kind == "this_quarter" {
		months = 3
	}
	previousStart := start.AddDate(0, -months, 0)
	if end.Equal(start.AddDate(0, months, 0)) {
		return crmcontracts.ReportingWindow{StartAt: previousStart, EndAt: start}
	}
	monthStart := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location()).AddDate(0, -months, 0)
	days := monthStart.AddDate(0, 1, -1).Day()
	previousEnd := monthStart.AddDate(0, 1, 0)
	if end.Day() <= days {
		previousEnd = time.Date(monthStart.Year(), monthStart.Month(), end.Day(), end.Hour(), end.Minute(), end.Second(), end.Nanosecond(), end.Location())
	}
	return crmcontracts.ReportingWindow{StartAt: previousStart, EndAt: previousEnd}
}

func reportingEarliest(frame crmcontracts.ReportingContext) time.Time {
	earliest := frame.Interval.StartAt
	previous := reportingPrevious(frame)
	if !previous.StartAt.IsZero() && previous.StartAt.Before(earliest) {
		earliest = previous.StartAt
	}
	if frame.TargetInterval != nil && frame.TargetInterval.StartAt.Before(earliest) {
		earliest = frame.TargetInterval.StartAt
	}
	return earliest
}

func reportingCohorts(frame crmcontracts.ReportingContext, facts []reporting.Fact) []reporting.Fact {
	previous := reportingPrevious(frame)
	out := make([]reporting.Fact, 0, len(facts)*2)
	for _, fact := range facts {
		if fact.Row.OccurredAt == nil {
			fact.ContextID = reportingState
			out = append(out, fact)
			continue
		}
		at := *fact.Row.OccurredAt
		if reportingInWindow(at, frame.Interval) {
			fact.ContextID = reportingInterval
			out = append(out, fact)
		}
		if reportingInWindow(at, previous) {
			fact.ContextID = "previous"
			out = append(out, fact)
		}
		if frame.TargetInterval != nil && reportingInWindow(at, *frame.TargetInterval) {
			fact.ContextID = reportingTarget
			out = append(out, fact)
		}
	}
	return out
}

func reportingInWindow(at time.Time, window crmcontracts.ReportingWindow) bool {
	return !at.Before(window.StartAt) && at.Before(window.EndAt)
}
