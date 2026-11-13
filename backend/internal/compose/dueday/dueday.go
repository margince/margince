// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dueday turns a stated due day into the moment it ends.
package dueday

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// EndOf is the moment a stated day ends in zone, or in the installation's
// zone when zone is empty. A deadline is the last moment still on time. It is
// built from the day's parts, because adding 24 hours misses a DST day.
func EndOf(ctx context.Context, tx pgx.Tx, day, zone string) (time.Time, error) {
	if zone == "" {
		var err error
		if zone, err = identity.TimezoneOf(ctx, tx); err != nil {
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
