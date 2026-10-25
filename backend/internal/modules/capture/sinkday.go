// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ErrDayWithoutDate refuses a date-only record that names no date. The capture
// time stands in for a missing moment, but it is no date the source named.
var ErrDayWithoutDate = fmt.Errorf("capture: a date-only record needs the date it happened on: %w", apperrors.ErrInvalidArgument)

// dayAnchor is where a date-only capture is stored: the anchor of the date
// named in its own offset.
func dayAnchor(named time.Time) (time.Time, error) {
	if named.IsZero() || !datasource.InstantInRange(named) {
		return time.Time{}, ErrDayWithoutDate
	}
	return occurredday.Anchor(named), nil
}
