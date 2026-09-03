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
	overdue := true
	item := crmcontracts.AttentionItem{
		Id:      "brief-1",
		Source:  "brief_item",
		DueAt:   &closes,
		Overdue: &overdue,
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
}

// The brief row and the at-risk row for the SAME deal must never disagree
// about whether it is late. The at-risk lane compares calendar dates in the
// workspace zone (deals.CloseIsOverdue) — a deal due TODAY is not overdue —
// and classifyBriefItem trusts the identical pre-computed verdict rather
// than recomputing one from the INSTANT the close date round-trips as (UTC
// midnight of the due day), which would read "overdue" from 00:00 UTC onward
// on the due day itself — a whole local day out of step with the sibling
// lane, for a deal due today.
func TestClassifyBriefItemTrustsTheDealsCalendarOverdueVerdictOverAnInstantComparison(t *testing.T) {
	// The close date's UTC-midnight round-trip is already behind rankInstant
	// (09:00 UTC on the same day), so an instant comparison would call this
	// overdue — but the deal's own calendar-date verdict, from the SAME lane's
	// deal-figures read, says it is due today and not yet late.
	closesToday := time.Date(rankInstant.Year(), rankInstant.Month(), rankInstant.Day(), 0, 0, 0, 0, time.UTC)
	notOverdue := false
	item := crmcontracts.AttentionItem{
		Id:      "brief-due-today",
		Source:  "brief_item",
		DueAt:   &closesToday,
		Overdue: &notOverdue,
	}

	got := classifyBriefItem(item, rankInstant)

	if got.overdue {
		t.Fatal("a deal due TODAY was marked overdue — the brief row disagrees with the at-risk row for the identical deal")
	}
	if got.item.Overdue == nil || *got.item.Overdue {
		t.Fatalf("item.Overdue = %v, wanted the deal's own false to reach the card's badge too", got.item.Overdue)
	}
}

// The date is a deal's expected close date, the same fact classifyRisk's own
// "deals_at_risk" row carries for the identical deal — so it reads closing_soon
// the way that sibling does, never due_today (a word that means the reader
// owes it TODAY, which a deal closing in three months is not) and never a
// second "overdue" sentence classifyRisk does not say either — the badge
// already carries that, from the SAME deadlineAt this stamps.
func TestClassifyBriefItemNamesTheCloseDateLikeItsRiskLaneSibling(t *testing.T) {
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
	if hasReasonKind(got.item.Because, "overdue") {
		t.Fatalf("because = %+v, overdue is the badge's job (item.Overdue), not a because reason here", got.item.Because)
	}
	if !hasReasonKind(got.item.Because, "closing_soon") {
		t.Fatalf("because = %+v, wanted closing_soon — the same reason classifyRisk gives the identical fact", got.item.Because)
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
