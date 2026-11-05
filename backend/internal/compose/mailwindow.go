// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
)

type routineWindow struct {
	Open  time.Time
	Close time.Time
	Day   time.Time
}

func routineWindowFor(ctx context.Context, users *identity.Service, now time.Time, settings identity.Delivery) (routineWindow, bool, error) {
	hours, _, err := users.MyWorkingHours(ctx)
	if err != nil {
		return routineWindow{}, false, err
	}
	zone, err := hours.Location()
	if err != nil {
		return routineWindow{}, false, err
	}
	local := now.In(zone)
	minute := hours.StartMinute
	if settings.RoutineTime != nil && *settings.RoutineTime != "working" {
		if len(*settings.RoutineTime) != 5 {
			return routineWindow{}, false, errors.New("invalid stored routine time")
		}
		hour, err := strconv.Atoi((*settings.RoutineTime)[:2])
		if err != nil {
			return routineWindow{}, false, err
		}
		minute = hour * 60
	}
	open := time.Date(local.Year(), local.Month(), local.Day(), minute/60, minute%60, 0, 0, zone)
	if local.Before(open) {
		open = open.AddDate(0, 0, -1)
	}
	window := routineWindow{Open: open, Close: open.Add(3 * time.Hour), Day: time.Date(open.Year(), open.Month(), open.Day(), 0, 0, 0, 0, zone)}
	weekday := (int(open.Weekday())+6)%7 + 1
	return window, hours.Works(weekday) && !now.Before(window.Open) && now.Before(window.Close), nil
}

func routineMailWindow(ctx context.Context, users *identity.Service, now time.Time, settings identity.Delivery) (bool, error) {
	_, allowed, err := routineWindowFor(ctx, users, now, settings)
	return allowed, err
}

func weeklyMailDay(hours identity.WorkingHours, chosen int) int {
	for offset := range 7 {
		day := (chosen-1+offset)%7 + 1
		if hours.Works(day) {
			return day
		}
	}
	return chosen
}

func (w *briefGenerateWorker) morningWindow(ctx context.Context, now time.Time, settings identity.Delivery) (routineWindow, bool, error) {
	if settings.RoutineTime != nil || settings.HourLocal == nil || routineCombines(settings) {
		return routineWindowFor(ctx, w.users, now, settings)
	}
	var local time.Time
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		_, clock, err := briefs.LocalDayAt(ctx, tx, now)
		local = clock
		return err
	})
	if err != nil {
		return routineWindow{}, false, err
	}
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	window := routineWindow{Open: time.Date(local.Year(), local.Month(), local.Day(), *settings.HourLocal, 0, 0, 0, local.Location()), Close: day.AddDate(0, 0, 1), Day: day}
	return window, !now.Before(window.Open) && now.Before(window.Close), nil
}

func routineChoicesUnchanged(ctx context.Context, users *identity.Service, expected identity.Delivery) (bool, error) {
	current, err := users.MyDelivery(ctx)
	if err != nil {
		return false, err
	}
	return current.SameAs(expected), nil
}
