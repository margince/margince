// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// classifyBriefItem's close date reaches the ordering as well as the card,
// and never overstates what it means.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestClassifyBriefItemStampsItsDeadline(t *testing.T) {
	closes := rankInstant.Add(-24 * time.Hour)
	item := crmcontracts.AttentionItem{
		Id:     "brief-1",
		Source: "brief_item",
		DueAt:  &closes,
	}

	got := classifyBriefItem(item, rankInstant)

	if got.deadlineAt.IsZero() {
		t.Fatal("deadlineAt is zero; the ordering can never read the close date the row was given")
	}
	if !got.deadlineAt.Equal(closes) {
		t.Fatalf("deadlineAt = %v, want the row's own close date %v", got.deadlineAt, closes)
	}
	if !got.overdue {
		t.Fatal("a close date in the past was not marked overdue")
	}
	if !hasReasonKind(got.item.Because, "overdue") {
		t.Fatalf("because = %+v, wanted an overdue reason", got.item.Because)
	}
}

// A close date months out is a forecast, not work owed: unlike a task's due
// date, it never claims due_today — that word means the reader owes it
// TODAY, and a deal closing three months out is not.
func TestClassifyBriefItemNeverClaimsAFutureCloseDateIsDueToday(t *testing.T) {
	closes := rankInstant.Add(90 * 24 * time.Hour)
	item := crmcontracts.AttentionItem{
		Id:     "brief-2",
		Source: "brief_item",
		DueAt:  &closes,
	}

	got := classifyBriefItem(item, rankInstant)

	if got.overdue {
		t.Fatal("a close date three months out was marked overdue")
	}
	if hasReasonKind(got.item.Because, "due_today") {
		t.Fatalf("because = %+v, a close date three months out is not due today", got.item.Because)
	}
	if got.deadlineAt.IsZero() {
		t.Fatal("deadlineAt is zero even with a close date set; a far-future date must still reach the ordering")
	}
}

func hasReasonKind(reasons []crmcontracts.WorklistReason, kind crmcontracts.WorklistReasonKind) bool {
	for _, r := range reasons {
		if r.Kind == kind {
			return true
		}
	}
	return false
}
