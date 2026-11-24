// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dueday turns a stated due day into the moment it ends.
package dueday

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Odd returns a stated day the task writer would refuse as too far from now,
// judged by the end of that day in zone. It returns "" for an ordinary or empty
// day. A caller asks the rep about an odd one.
func Odd(ctx context.Context, tx pgx.Tx, day, zone string, now time.Time) (string, error) {
	if day == "" {
		return "", nil
	}
	end, err := EndOf(ctx, tx, day, zone)
	if err != nil || values.DueInWindow(end, now) {
		return "", err
	}
	return day, nil
}

// DayOf is the calendar day an instant falls on in the installation's zone,
// the zone EndOf rebuilds a day in. It needs no settings grant, because it only
// names a day for a write the caller is already allowed to make.
func DayOf(ctx context.Context, tx pgx.Tx, at time.Time) (string, error) {
	loc, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return "", err
	}
	return at.In(loc).Format(time.DateOnly), nil
}

// EndOf is the moment a stated day ends in zone, or in the installation's
// zone when zone is empty. A deadline is the last moment still on time. It is
// built from the day's parts, because adding 24 hours misses a DST day.
func EndOf(ctx context.Context, tx pgx.Tx, day, zone string) (time.Time, error) {
	if zone == "" {
		var err error
		if zone, err = identity.TimezoneAppliedTx(ctx, tx); err != nil {
			return time.Time{}, err
		}
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, fmt.Errorf("compose: commitment due-date timezone %q: %w", zone, err)
	}
	parsed, err := time.ParseInLocation(time.DateOnly, day, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf(
			"compose: commitment due date %q is not a date — write it as YYYY-MM-DD: %w", day, err)
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc).UTC(), nil
}

// DayAfter is the calendar day after a stated day, as YYYY-MM-DD. An empty day
// stays empty: a promise nobody dated gives no day to follow it up on.
func DayAfter(day string) (string, error) {
	if day == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return "", fmt.Errorf("compose: commitment due date %q is not a date — write it as YYYY-MM-DD: %w", day, err)
	}
	return parsed.AddDate(0, 0, 1).Format(time.DateOnly), nil
}

// ComingDay turns a calendar day into the due clock of a task due when that day
// ends. An empty zone means the installation's zone. A day already over
// there is a validation error on field. Its task would be overdue at once.
func ComingDay(ctx context.Context, tx pgx.Tx, field, day, zone string, now time.Time) (crmcontracts.IntentTime, error) {
	end, err := EndOf(ctx, tx, day, zone)
	if err != nil {
		return crmcontracts.IntentTime{}, err
	}
	if zone == "" {
		if zone, err = identity.TimezoneAppliedTx(ctx, tx); err != nil {
			return crmcontracts.IntentTime{}, err
		}
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return crmcontracts.IntentTime{}, fmt.Errorf("compose: due-date timezone %q: %w", zone, err)
	}
	if deadline.DayPassed(end, now, loc) {
		return crmcontracts.IntentTime{}, httperr.Validation(field, "in_the_past", "choose today or a later day")
	}
	return crmcontracts.IntentTime{WallClock: day + "T" + values.DayOnlyClock, Timezone: zone}, nil
}

// InstallationZone is the zone the installation's calendar days end in.
func InstallationZone(ctx context.Context, db *database.DB) (*time.Location, error) {
	var loc *time.Location
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		loc, err = identity.ZoneAppliedTx(ctx, tx)
		return err
	})
	return loc, err
}
