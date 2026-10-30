// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The task count widens its due window with the reader's due reminders.
// Postgres plans every copy of the content gate a statement carries, and a
// pooled connection keeps that memory until it closes. These tests pin that
// the widening arm adds no second copy of a gate the read already applies.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func gateCopies(where []string) int {
	return auth.ContentGateCopies(strings.Join(where, " AND "))
}

// discoverCopies counts the discover gates on the listed row by the link fold
// each one reads.
func discoverCopies(where []string) int {
	return strings.Count(strings.Join(where, " AND "), "FROM activity_link l WHERE l.activity_id = a.id")
}

func renderedFilter(t *testing.T, in ListActivitiesInput) []string {
	t.Helper()
	where, _, _, _, err := listActivitiesFilter(teamScopedCtx(), in)
	if err != nil {
		t.Fatalf("rendering the filter: %v", err)
	}
	return where
}

// Under a content gate the reminders arm is already gated: the outer filter
// holds every row to it, reminders included.
func TestTheRemindersArmOfAContentGatedReadAddsNoSecondGate(t *testing.T) {
	until := clocktest.Now(t)
	in := ListActivitiesInput{OpenAndDueBy: &until, IncludeEmailRequests: true, WorklistAsOf: &until, ReadableOnly: true}
	without := renderedFilter(t, in)
	in.OrRemindersDue = true
	with := renderedFilter(t, in)
	if !strings.Contains(strings.Join(with, " AND "), "remind_at") {
		t.Fatal("the widened read names no reminder, so this test reads the wrong clause")
	}
	if got, want := gateCopies(with), gateCopies(without); got != want {
		t.Fatalf("the reminders arm added %d content gate(s) to a read already content-gated", got-want)
	}
}

// Under a discover gate the arm still owes the audience. The content gate is
// discover and audience, so the arm adds the audience half alone.
func TestTheRemindersArmOfADiscoverGatedReadAddsTheAudienceAlone(t *testing.T) {
	until := clocktest.Now(t)
	in := ListActivitiesInput{OpenAndDueBy: &until, WorklistAsOf: &until}
	without := renderedFilter(t, in)
	in.OrRemindersDue = true
	with := renderedFilter(t, in)
	if got, want := gateCopies(with), gateCopies(without)+1; got != want {
		t.Fatalf("the reminders arm carries %d audience gate(s), want 1", got-gateCopies(without))
	}
	if got, want := discoverCopies(with), discoverCopies(without); got != want {
		t.Fatalf("the reminders arm repeated the discover gate %d time(s) the outer read already applies", got-want)
	}
}
