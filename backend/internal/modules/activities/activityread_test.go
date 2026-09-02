// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which ORDER BY a timeline read gets: newest-first for the general read, and
// due-soonest for the one caller asking "what is open and due" — the dial
// margince#3287 needed so a capped page keeps the most urgent rows rather
// than the most recently logged ones.

import (
	"testing"
	"time"
)

func TestTheOpenAndDueQueueOrdersByDueDateNotRecency(t *testing.T) {
	until := time.Now()
	got := orderClause(ListActivitiesInput{OpenAndDueBy: &until})
	want := " ORDER BY a.due_at ASC, a.id ASC"
	if got != want {
		t.Fatalf("order clause = %q, want %q — a cap over the wrong order keeps the newest rows, not the most overdue ones",
			got, want)
	}
}

func TestEveryOtherTimelineReadStaysNewestFirst(t *testing.T) {
	got := orderClause(ListActivitiesInput{})
	want := " ORDER BY a.occurred_at DESC, a.id DESC"
	if got != want {
		t.Fatalf("order clause = %q, want %q", got, want)
	}
}
