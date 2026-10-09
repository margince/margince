// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"
)

// The model's due date is kept when it names a storable day. It is dropped when
// it is unreadable or outside the storable range.
func TestSettleDueDateKeepsOnlyAStorableDay(t *testing.T) {
	if got := settleDueDate("2026-11-02"); got == nil || !got.Equal(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("settleDueDate(2026-11-02) = %v, want that day", got)
	}
	for _, value := range []string{"next tuesday", "9999-12-31", "0000-01-01"} {
		if got := settleDueDate(value); got != nil {
			t.Errorf("settleDueDate(%q) = %v, want no date", value, got)
		}
	}
}
