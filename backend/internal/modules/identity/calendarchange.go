// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// CalendarChange is what else must commit with a move of the reporting zone
// or the fiscal start. It is handed the calendar in force before the move.
type CalendarChange func(ctx context.Context, tx pgx.Tx, before ReportingCalendar) error

// ReportingCalendar is the zone and fiscal start a period report is cut in.
type ReportingCalendar struct {
	Timezone             string
	FiscalYearStartMonth int
}

// ZoneChange is what else must commit with a move of the reporting zone alone:
// whatever was derived from today's date in the zone being replaced.
type ZoneChange func(ctx context.Context, tx pgx.Tx) error

// WithZoneChange wires the hook a zone move runs, before the calendar hook.
func (s *InstallationSettingsStore) WithZoneChange(hook ZoneChange) *InstallationSettingsStore {
	out := *s
	out.onZoneChange = hook
	return &out
}

// WithCalendarChange wires the hook a calendar move runs. Without it a move
// changes the setting and nothing else.
func (s *InstallationSettingsStore) WithCalendarChange(hook CalendarChange) *InstallationSettingsStore {
	out := *s
	out.onCalendarChange = hook
	return &out
}

// changeCalendarTx runs the calendar hook under both settings' write locks,
// before either is written, so the hook reads the calendar being replaced.
func (s *InstallationSettingsStore) changeCalendarTx(ctx context.Context, tx pgx.Tx, zone *string, fiscalStart *int) error {
	if (s.onCalendarChange == nil && s.onZoneChange == nil) || (zone == nil && fiscalStart == nil) {
		return nil
	}
	for _, key := range []string{Timezone.Key(), FiscalYearStartMonth.Key()} {
		if err := settings.LockForWrite(ctx, tx, key); err != nil {
			return err
		}
	}
	var before ReportingCalendar
	var err error
	if before.Timezone, err = TimezoneOf(ctx, tx); err != nil {
		return err
	}
	if before.FiscalYearStartMonth, err = FiscalYearStartMonthOf(ctx, tx); err != nil {
		return err
	}
	zoneMoves := zone != nil && *zone != before.Timezone
	startMoves := fiscalStart != nil && *fiscalStart != before.FiscalYearStartMonth
	if zoneMoves && s.onZoneChange != nil {
		if err := s.onZoneChange(ctx, tx); err != nil {
			return err
		}
	}
	if (!zoneMoves && !startMoves) || s.onCalendarChange == nil {
		return nil
	}
	return s.onCalendarChange(ctx, tx, before)
}
