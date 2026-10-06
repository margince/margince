// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

// periodicLog records what a schedule change asked of the runner, in order.
type periodicLog struct{ calls []string }

func (l *periodicLog) RemovePeriodic(id string) { l.calls = append(l.calls, "remove "+id) }

func (l *periodicLog) AddPeriodic(*river.PeriodicJob) error {
	l.calls = append(l.calls, "add")
	return nil
}

// bookWith schedules close_date_sweep and time_scan from a book holding the
// given values, the two kinds reading two different settings.
func bookWith(t *testing.T, closeDate, timeScan time.Duration) (*ScheduleBook, string, string) {
	t.Helper()
	closeSpec, scanSpec := specFor(t, CloseDateSweepArgs{}.Kind()), specFor(t, TimeScanArgs{}.Kind())
	book := newScheduleBook(map[string]time.Duration{
		closeSpec.Cadence.Setting: closeDate,
		scanSpec.Cadence.Setting:  timeScan,
	})
	book.schedule(closeSpec, CloseDateSweepArgs{}, nil)
	book.schedule(scanSpec, TimeScanArgs{}, nil)
	return book, closeSpec.Cadence.Setting, scanSpec.Cadence.Setting
}

func TestApplyMovesOnlyTheScheduleWhoseSettingChanged(t *testing.T) {
	book, closeDate, timeScan := bookWith(t, time.Hour, time.Hour)
	runner := &periodicLog{}
	if err := book.apply(map[string]time.Duration{closeDate: 2 * time.Hour, timeScan: time.Hour}, runner); err != nil {
		t.Fatal(err)
	}
	if want := []string{"remove close_date_sweep", "add"}; !slices.Equal(runner.calls, want) {
		t.Errorf("runner was asked %v, want %v", runner.calls, want)
	}
	if got := book.intervals[closeDate].get(); got != 2*time.Hour {
		t.Errorf("close_date_sweep now runs every %s, want 2h", got)
	}
}

func TestApplyLeavesTheRunnerAloneWhenNothingChanged(t *testing.T) {
	book, closeDate, timeScan := bookWith(t, time.Hour, time.Hour)
	runner := &periodicLog{}
	if err := book.apply(map[string]time.Duration{closeDate: time.Hour, timeScan: time.Hour}, runner); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("an unchanged read asked the runner %v; every recheck would restart every schedule", runner.calls)
	}
}

func TestApplySwitchesAScheduleOffAndBackOn(t *testing.T) {
	book, closeDate, timeScan := bookWith(t, time.Hour, time.Hour)
	runner := &periodicLog{}
	if err := book.apply(map[string]time.Duration{closeDate: 0, timeScan: time.Hour}, runner); err != nil {
		t.Fatal(err)
	}
	if want := []string{"remove close_date_sweep"}; !slices.Equal(runner.calls, want) {
		t.Errorf("switching off asked the runner %v, want %v", runner.calls, want)
	}
	runner.calls = nil
	if err := book.apply(map[string]time.Duration{closeDate: time.Hour, timeScan: time.Hour}, runner); err != nil {
		t.Fatal(err)
	}
	if want := []string{"remove close_date_sweep", "add"}; !slices.Equal(runner.calls, want) {
		t.Errorf("switching back on asked the runner %v, want %v", runner.calls, want)
	}
}
