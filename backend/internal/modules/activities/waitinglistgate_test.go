// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A waiting list admits only ids its waiting set content-gated. So the list
// adds no gate of its own, and the set's copies are the statement's.
func TestAWaitingListCarriesOnlyTheGatesOfItsWaitingSet(t *testing.T) {
	asOf := clocktest.Now(t)
	in := ListActivitiesInput{WaitingReplyAsOf: &asOf}
	set, err := appendWaitingReplyClause(teamScopedCtx(), in, func(any) int { return 1 }, nil)
	if err != nil {
		t.Fatalf("rendering the waiting set: %v", err)
	}
	if gateCopies(set) == 0 {
		t.Fatal("the waiting set carries no content gate, so the list would admit unreadable mail")
	}
	page, err := timelinePageSQL(teamScopedCtx(), in, 50)
	if err != nil {
		t.Fatalf("rendering the waiting list: %v", err)
	}
	if got, want := gateCopies([]string{page.sql}), gateCopies(set); got != want {
		t.Fatalf("the waiting list carries %d content gates, want the %d of its waiting set", got, want)
	}
	if !strings.Contains(page.sql, "waiting_thread") {
		t.Fatal("the list holds no waiting set, so this test reads the wrong statement")
	}
}
