// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A date-only capture is stored at noon UTC of the date it names in its own
// offset. One naming no date is refused, as the API refuses it.
func TestADateOnlyCaptureIsAnchoredOrRefused(t *testing.T) {
	t.Parallel()
	lateInHonolulu := time.Date(2026, 3, 9, 23, 0, 0, 0, time.FixedZone("HST", -10*3600))
	if got, err := dayAnchor(lateInHonolulu); err != nil || !got.Equal(time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("anchored at %v (%v), want noon UTC of 9 March", got, err)
	}
	_, err := dayAnchor(time.Time{})
	if !errors.Is(err, ErrDayWithoutDate) || !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("a day with no date = %v, want ErrDayWithoutDate as an invalid argument", err)
	}
}
