// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestAnUndatedTaskCitesWithNoDateRatherThanTheZeroTime holds what nil means on
// the wire.
//
// An open task with no due date has no moment, and taking the address of its
// zero value renders `0001-01-01T00:00:00Z` — a date no reader can act on and
// none of them chose. It reaches the card because a cited record may be a task:
// the model cites one the way it cites any timeline row, and the reader opens it
// the same way.
func TestAnUndatedTaskCitesWithNoDateRatherThanTheZeroTime(t *testing.T) {
	undated := taskAsActivity(activities.OpenTask{ID: ids.NewV7(), Subject: "Send the scope"})
	if got := evidenceOf(undated, "Open: Send the scope"); got.OccurredAt != nil {
		t.Errorf("an undated task cites with occurred_at %v, want none — the card would print a date "+
			"nobody chose", got.OccurredAt)
	}

	dated := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	withDate := taskAsActivity(activities.OpenTask{ID: ids.NewV7(), Subject: "Send the scope", DueAt: &dated})
	got := evidenceOf(withDate, "Open: Send the scope")
	if got.OccurredAt == nil || !got.OccurredAt.Equal(dated) {
		t.Errorf("a dated task cites with occurred_at %v, want its due date", got.OccurredAt)
	}
}
