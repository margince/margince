// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type duePeriods struct {
	Recent      []time.Time
	Next        time.Time
	Skipped     int
	LastSkipped time.Time
}

func catchUp(schedule crmcontracts.ReportingSchedule, now time.Time) (duePeriods, error) {
	out := duePeriods{Next: schedule.NextDueAt}
	for !out.Next.After(now) {
		if out.Skipped > 12000 {
			return out, invalid("review the schedule's overdue starting date")
		}
		out.Recent = append(out.Recent, out.Next)
		if len(out.Recent) > 12 {
			out.LastSkipped = out.Recent[0]
			out.Recent = out.Recent[1:]
			out.Skipped++
		}
		next, err := NextDue(schedule.Definition, schedule.Timezone, out.Next)
		if err != nil {
			return out, err
		}
		out.Next = next
	}
	return out, nil
}

func (s *Service) materializeSchedule(ctx context.Context, tx pgx.Tx, schedule crmcontracts.ReportingSchedule) (int, error) {
	report, err := s.report(ctx, tx, ids.UUID(schedule.ReportId), false)
	if err != nil {
		return 0, err
	}
	selection, _, err := s.reportRevision(ctx, tx, report, schedule.Definition.ReportRevision)
	if err != nil {
		return 0, err
	}
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return 0, err
	}
	if calendar.Timezone != schedule.Timezone {
		return 0, s.suspendSchedule(ctx, tx, schedule, "Reporting timezone changed; review and save the schedule")
	}
	periods, err := catchUp(schedule, s.now())
	if err != nil {
		return 0, err
	}
	selection.Period = "last_week"
	if schedule.Definition.Frequency == "monthly" {
		selection.Period = "last_month"
	}
	selection.Interval = nil
	dates := periods.Recent
	if periods.Skipped > 0 {
		dates = append([]time.Time{periods.LastSkipped}, dates...)
	}
	for i, due := range dates {
		interval, err := Interval(selection, calendar, due)
		if err != nil {
			return 0, err
		}
		version := schedule.Version
		intent := executionIntent{Report: report, Revision: schedule.Definition.ReportRevision, Owner: ids.UUID(schedule.OwnerId), Due: due, Interval: interval, ScheduleID: (*ids.UUID)(&schedule.Id), ScheduleVersion: &version, Key: fmt.Sprintf("schedule:%s:%d:%s", schedule.Id, version, due.UTC().Format(time.RFC3339Nano)), Hash: fmt.Sprint(version)}
		if i == 0 && periods.Skipped > 0 {
			intent.Status = "skipped"
			intent.Reason = fmt.Sprintf("Skipped %d overdue periods from %s through %s; newest 12 retained", periods.Skipped, schedule.NextDueAt.Format(time.RFC3339), periods.LastSkipped.Format(time.RFC3339))
		}
		if _, err := s.insertExecution(ctx, tx, intent); err != nil {
			return 0, err
		}
	}
	before := schedule
	schedule.NextDueAt = periods.Next
	var b bindings
	if _, err := tx.Exec(ctx, "UPDATE report_schedule SET next_due_at="+b.add(periods.Next)+" WHERE id="+b.add(schedule.Id), b.values...); err != nil {
		return 0, err
	}
	return len(dates), recordChange(ctx, tx, "report_schedule", ids.UUID(schedule.Id), "schedule", &before, schedule)
}
