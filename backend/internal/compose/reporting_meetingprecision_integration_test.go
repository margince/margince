// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The held-meetings reading places a meeting on the day its held transition
// recorded, with the precision that transition recorded. The workspace is in
// Auckland, where noon UTC is already the next local day.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// redate moves a meeting to a new date or time through the activity update.
func (m meetingWeeks) redate(t *testing.T, meeting ids.ActivityID, at time.Time, precision string) {
	t.Helper()
	if _, err := activities.NewStore(InstallationDB(m.e.Pool)).WithClock(m.clock).UpdateActivity(m.human, meeting,
		activities.UpdateActivityInput{OccurredAt: &at, OccurredPrecision: &precision}); err != nil {
		t.Fatalf("re-dating the meeting: %v", err)
	}
}

// heldPrecision is the precision the held transition recorded, or "unknown".
func (m meetingWeeks) heldPrecision(t *testing.T, meeting ids.ActivityID) string {
	t.Helper()
	var precision *string
	if err := m.e.owner.QueryRow(context.Background(), `SELECT scheduled_precision FROM activity_meeting_history
		WHERE activity_id = $1 AND status = 'held'`, meeting.UUID).Scan(&precision); err != nil {
		t.Fatal(err)
	}
	if precision == nil {
		return "unknown"
	}
	return *precision
}

func (m meetingWeeks) heldCoverage(t *testing.T) crmcontracts.ReportingCoverage {
	t.Helper()
	result, err := newReportingService(m.e.Pool, m.clock).Evaluate(m.human, crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Metrics[0].Coverage
}

// A date-only meeting held on Sunday 11 October and then moved to Tuesday
// stays in the week of its held date. Read as noon UTC it would be Monday in
// Auckland, the next week.
func TestADayMeetingMovedLaterStaysOnTheDayItWasHeld(t *testing.T) {
	m := setupMeetingWeeks(t)
	sunday := occurredday.Anchor(time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC))
	meeting := m.held(t, sunday, activities.PrecisionDay)
	m.redate(t, meeting, occurredday.Anchor(time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)), activities.PrecisionDay)

	if got := m.heldPrecision(t, meeting); got != activities.PrecisionDay {
		t.Fatalf("the held transition recorded %s, want day", got)
	}
	weeks, rows := m.weeks(t, "2026-10-05")
	if weeks["2026-10-05"] != 1 || weeks["2026-10-12"] != 0 {
		t.Errorf("weeks %v, want the meeting in the week of 5 October only", weeks)
	}
	if len(rows) != 1 || rows[0].OccurredPrecision == nil || !occurredday.IsDay(*rows[0].OccurredPrecision) {
		t.Errorf("evidence rows %+v, want one row read as a date", rows)
	}
	if coverage := m.heldCoverage(t); coverage.Status == reportingPartial {
		t.Errorf("a recorded precision was reported as a guess: %+v", coverage)
	}
}

// A meeting held at 12:00 UTC, then made a day entry for the same
// date, keeps the instant it was held at: Monday 01:00 in Auckland.
func TestANoonInstantStaysAnInstantWhenItsMeetingBecomesADay(t *testing.T) {
	m := setupMeetingWeeks(t)
	noon := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	meeting := m.held(t, noon, activities.PrecisionInstant)
	m.redate(t, meeting, noon, activities.PrecisionDay)

	if got := m.heldPrecision(t, meeting); got != activities.PrecisionInstant {
		t.Fatalf("the held transition recorded %s, want instant", got)
	}
	weeks, _ := m.weeks(t, "2026-10-12")
	if weeks["2026-10-12"] != 1 || weeks["2026-10-05"] != 0 {
		t.Errorf("weeks %v, want the held instant in the week of 12 October only", weeks)
	}
}

// An older transition with no recorded precision and a noon UTC start could
// be a time or a date. The reading says so. Any other start is a time of day,
// because a date is only ever stored at noon UTC.
func TestAnOldNoonStartWithNoPrecisionMarksTheReadingPartial(t *testing.T) {
	m := setupMeetingWeeks(t)
	morning := m.held(t, time.Date(2026, 10, 6, 9, 15, 0, 0, time.UTC), activities.PrecisionInstant)
	forgetMeetingPrecision(t, m.e.owner, morning)
	if coverage := m.heldCoverage(t); coverage.Status != "ok" {
		t.Fatalf("an old 09:15 UTC start made the reading %+v; it can only be a time", coverage)
	}

	noon := m.held(t, time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), activities.PrecisionInstant)
	forgetMeetingPrecision(t, m.e.owner, noon)
	coverage := m.heldCoverage(t)
	want := reportingCoverageWords.LegacyMeetingDay.En
	if coverage.Status != reportingPartial || coverage.Reason == nil || *coverage.Reason != want {
		t.Errorf("coverage %+v, want partial with %q", coverage, want)
	}
}

// This week runs from Monday 12 October, local midnight, to the cutoff. A
// month target beside it counts every meeting of October up to the cutoff.
func TestThisWeekCountsMeetingsFromLocalMondayAndTheMonthTargetCountsTheMonth(t *testing.T) {
	m := setupMeetingWeeks(t)
	m.held(t, time.Date(2026, 10, 8, 22, 0, 0, 0, time.UTC), activities.PrecisionInstant)
	m.held(t, time.Date(2026, 10, 11, 10, 59, 0, 0, time.UTC), activities.PrecisionInstant)
	m.held(t, time.Date(2026, 10, 11, 11, 0, 0, 0, time.UTC), activities.PrecisionInstant)
	m.held(t, occurredday.Anchor(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)), activities.PrecisionDay)

	result, err := newReportingService(m.e.Pool, m.clock).Evaluate(m.human, crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_week", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 11, 11, 0, 0, 0, time.UTC); !result.Context.Interval.StartAt.Equal(want) || !result.Context.Interval.EndAt.Equal(m.clock()) {
		t.Errorf("interval %+v, want %s to the cutoff", result.Context.Interval, want)
	}
	metric := result.Metrics[0]
	if metric.Value == nil || *metric.Value != 2 {
		t.Errorf("this week counts %v meetings held, want the Monday-midnight one and the Monday day entry", metric.Value)
	}
	if metric.TargetActual == nil || *metric.TargetActual != 4 {
		t.Errorf("the month target counts %v, want all four October meetings", metric.TargetActual)
	}
}

// A row written now must say how much of its start is known. IN over a NULL
// is NULL, which a CHECK would let through, so the rule names NULL itself.
func TestANewTransitionWithAStartAndNoPrecisionIsRefused(t *testing.T) {
	m := setupMeetingWeeks(t)
	meeting := m.held(t, time.Date(2026, 10, 6, 9, 15, 0, 0, time.UTC), activities.PrecisionInstant)
	_, err := m.e.owner.Exec(context.Background(), `INSERT INTO activity_meeting_history
		(activity_id, status, scheduled_start, actor, effective_at) VALUES ($1, 'held', $2, 'test', now())`,
		meeting.UUID, time.Date(2026, 10, 6, 9, 15, 0, 0, time.UTC))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "activity_meeting_history_start_precision" {
		t.Fatalf("inserting a start without a precision answered %v, want the precision rule to refuse it", err)
	}
}

// An old date-only Sunday, 31 January, stored at noon UTC: as a date it is
// Sunday, as an instant it is Monday 01:00 in Auckland. Its unrecorded
// precision reads it as the Sunday, outside this week and this month. The
// week starting that Monday is still partial, because the meeting may be in it.
func TestAnOldNoonStartTaintsThePeriodItsGuessLeftOut(t *testing.T) {
	m := setupMeetingWeeks(t)
	cutoff := time.Date(2027, 2, 3, 0, 0, 0, 0, time.UTC)
	m.clock = func() time.Time { return cutoff }
	sunday := m.held(t, occurredday.Anchor(time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)), activities.PrecisionDay)
	forgetMeetingPrecision(t, m.e.owner, sunday)

	result, err := newReportingService(m.e.Pool, m.clock).Evaluate(m.human, crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_week", TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{},
	})
	if err != nil {
		t.Fatal(err)
	}
	metric := result.Metrics[0]
	if metric.Value == nil || *metric.Value != 0 {
		t.Errorf("this week counts %v meetings, want 0: the guess puts the meeting on the Sunday before", metric.Value)
	}
	want := reportingCoverageWords.LegacyMeetingDay.En
	if metric.Coverage.Status != reportingPartial || metric.Coverage.Reason == nil || *metric.Coverage.Reason != want {
		t.Errorf("coverage %+v, want partial with %q rather than a complete zero", metric.Coverage, want)
	}
}
