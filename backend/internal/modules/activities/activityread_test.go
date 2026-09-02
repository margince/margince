// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The cursor a resumed read carries is keyed to whichever order actually
// ran, so the two may never disagree. The ORDER BY choice itself
// (newest-first vs. due-soonest) is proven behaviourally by
// TestThePlannedLaneCapKeepsTheMostOverdueNotTheNewestLogged, which fails
// against the pre-fix ordering — a unit test asserting the literal SQL
// string here would only restate orderClause's own body.

import (
	"errors"
	"testing"
	"time"
)

// A cursor from the recency read cannot resume an open-and-due read: it is
// keyed to (occurred_at, id), the query would run ordered by (due_at, id),
// and applying one axis's cursor to the other axis's order returns silently
// wrong rows rather than the next page.
func TestACursorFromTheRecencyOrderCannotResumeAnOpenAndDueRead(t *testing.T) {
	until := time.Now()
	cursor := "anything"
	_, _, _, _, err := listActivitiesFilter(unscopedCtx(), ListActivitiesInput{
		OpenAndDueBy: &until,
		Cursor:       &cursor,
	})
	if !errors.Is(err, errOpenAndDueByWithCursor) {
		t.Fatalf("combining OpenAndDueBy with a Cursor → %v, want errOpenAndDueByWithCursor", err)
	}
}
