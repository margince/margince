// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// isStorableCalendarDay reports whether s is an ISO calendar date a request may
// bind to a date column. Go reads years Postgres has no date for, so the range
// is the one every other request-stored date obeys.
func isStorableCalendarDay(s string) bool {
	day, err := time.Parse(time.DateOnly, s)
	return err == nil && datasource.InstantInRange(day)
}
