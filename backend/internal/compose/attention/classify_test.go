// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What classifyBriefItem does with the close date margince#3438 put on the
// row: it must reach the ordering, not just the card.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The overnight brief's close date prints on the card and, before this fix,
// ordered nothing: classifyBriefItem was the one lane classifier that never
// called stampDeadline, so ranked.deadlineAt stayed zero for every brief row
// and the ranking never read item.DueAt at all.
func TestClassifyBriefItemStampsItsDeadline(t *testing.T) {
	closes := rankInstant.Add(-24 * time.Hour)
	item := crmcontracts.AttentionItem{
		Id:     "brief-1",
		Source: sourceAtRisk,
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

// A brief row whose close date has not yet arrived says so as due_today does
// for every other classifier, rather than staying silent about a date it
// still prints on the card.
func TestClassifyBriefItemNamesAFutureCloseDate(t *testing.T) {
	closes := rankInstant.Add(24 * time.Hour)
	item := crmcontracts.AttentionItem{
		Id:     "brief-2",
		Source: sourceAtRisk,
		DueAt:  &closes,
	}

	got := classifyBriefItem(item, rankInstant)

	if got.overdue {
		t.Fatal("a close date in the future was marked overdue")
	}
	if !hasReasonKind(got.item.Because, "due_today") {
		t.Fatalf("because = %+v, wanted a due_today reason", got.item.Because)
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
