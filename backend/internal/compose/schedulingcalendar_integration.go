// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// SchedulingCalendarForTest is the production calendar seam with no provider
// registry: its record checks are real, and every provider call refuses.
//
//nolint:ireturn // the seam is consumed as the activities port
func SchedulingCalendarForTest(pool *pgxpool.Pool) activities.SchedulingCalendar {
	return newSchedulingCalendar(pool, nil)
}

// WithSchedulingCalendarForTest books the HTTP doors through wrap's answer,
// which is handed the production seam to answer the record checks with. It
// lives behind the integration tag, so no shipped binary links it.
func WithSchedulingCalendarForTest(wrap func(activities.SchedulingCalendar) activities.SchedulingCalendar) Option {
	return func(s *Server, pool *pgxpool.Pool) { s.send.Calendar = wrap(newSchedulingCalendar(pool, nil)) }
}
