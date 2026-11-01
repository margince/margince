// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"errors"
	"time"
)

// CalendarOccupancy validates provider bounds before booking buffers are applied.
// A zero-duration placeholder blocks no time; reversed bounds are untrusted.
func CalendarOccupancy(start, end time.Time) (bool, error) {
	if end.Before(start) {
		return false, errors.New("calendar: invalid busy interval")
	}
	return end.After(start), nil
}
