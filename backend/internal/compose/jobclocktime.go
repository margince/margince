// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Extension jobs that run at a local clock time, such as "weekdays at 07:30".
// The clock is the installation's time zone, which the schedule book holds
// and moves when an admin changes it.

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/pkg/extension"
)

// clockTimeSchedule is a River schedule for one extension.TimeOfDay.
type clockTimeSchedule struct {
	at   extension.TimeOfDay
	zone func() *time.Location
}

// Next is the first slot after current. A run at a slot asks again with that
// slot, so a time the clock shows twice when it falls back runs once.
func (s clockTimeSchedule) Next(current time.Time) time.Time {
	zone := s.zone()
	y, m, d := current.In(zone).Date()
	// Today and the next seven days hold a slot of every valid TimeOfDay.
	for offset := range 8 {
		if !s.at.RunsOn(time.Date(y, m, d+offset, 12, 0, 0, 0, zone).Weekday()) {
			continue
		}
		if slot := clockTimeOn(y, m, d+offset, s.at.Minute, zone); slot.After(current) {
			return slot
		}
	}
	return current.AddDate(0, 0, 7)
}

// clockTimeOn is the moment the local clock shows minute on that day. On the
// day the clock springs past it, the slot moves to the end of the gap, the
// first minute after it.
func clockTimeOn(y int, m time.Month, d, minute int, zone *time.Location) time.Time {
	slot := time.Date(y, m, d, minute/60, minute%60, 0, 0, zone)
	if slot.Hour()*60+slot.Minute() == minute {
		return slot
	}
	start, end := slot.ZoneBounds()
	if end.Sub(slot) < slot.Sub(start) {
		return end
	}
	return start
}

// runsAtClockTimes reports whether a schedule's runs belong to fixed clock
// times. Such a pass never runs early at a start: only a missed slot is
// caught up.
func runsAtClockTimes(every river.PeriodicSchedule) bool {
	_, ok := every.(clockTimeSchedule)
	return ok
}

// zoneScheduleKey tracks the clock-time kinds in the book, so a changed time
// zone moves them the way a changed setting moves an interval.
var zoneScheduleKey = identity.Timezone.Key()

// location is the zone clock-time passes count in, UTC until one is read.
func (b *ScheduleBook) location() *time.Location {
	if zone := b.zone.Load(); zone != nil {
		return zone
	}
	return time.UTC
}

// scheduleClockTime builds a clock-time pass and records it under the zone.
func (b *ScheduleBook) scheduleClockTime(kind string, at extension.TimeOfDay, ctor river.PeriodicJobConstructor) *river.PeriodicJob {
	b.mu.Lock()
	defer b.mu.Unlock()
	job, catchUp := b.periodicJob(kind, kind, clockTimeSchedule{at: at, zone: b.location}, ctor)
	b.kinds[kind] = tracked{setting: zoneScheduleKey, job: job, catchUp: catchUp}
	return job
}

// applyZone moves every clock-time pass to a changed zone. It hands each back
// to River without a catch-up, so the move itself runs nothing: the next run
// is the next slot in the new zone.
func (b *ScheduleBook) applyZone(zone *time.Location, runner periodicRegistry) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if zone.String() == b.location().String() && !b.pending[zoneScheduleKey] {
		return nil
	}
	b.zone.Store(zone)
	for _, kind := range slices.Sorted(maps.Keys(b.kinds)) {
		entry := b.kinds[kind]
		if entry.setting != zoneScheduleKey {
			continue
		}
		runner.RemovePeriodic(kind)
		if err := runner.AddPeriodic(entry.job); err != nil {
			b.pending[zoneScheduleKey] = true
			return err
		}
	}
	delete(b.pending, zoneScheduleKey)
	return nil
}

// readInstallationZone is the zone clock-time passes count in. An installation
// not yet provisioned counts in UTC, the setting's default.
func readInstallationZone(ctx context.Context, pool *pgxpool.Pool) (*time.Location, error) {
	ws, err := singletonWorkspace(ctx, pool)
	if err != nil {
		return nil, err
	}
	if ws.IsZero() {
		return time.UTC, nil
	}
	readCtx := bootCtx(ctx, ws, scheduleReadActor)
	var zone *time.Location
	err = database.WithWorkspaceTx(readCtx, pool, func(tx pgx.Tx) (err error) {
		zone, err = identity.ZoneAppliedTx(readCtx, tx)
		return err
	})
	return zone, err
}
