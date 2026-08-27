// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PO-F-4's five states, in the order that keeps them mutually exclusive. The
// order is the point: a silent account must read as dormant rather than as
// whichever side happened to write last a year ago.
func TestEngagementStateAnswersWhoseMoveItIs(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	day := 24 * time.Hour

	cases := []struct {
		name string
		in   suggestionInputs
		want string
	}{
		{"nothing captured", suggestionInputs{}, "never_contacted"},
		{"they wrote this morning", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "inbound", At: ago(4 * time.Hour)},
		}, "active"},
		{"we wrote yesterday", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "outbound", At: ago(day)},
		}, "active"},
		// The threshold is shared with the no_reply suggestion, so the strip and
		// the nudge below it cannot disagree about whether an account is waiting.
		{"we wrote a fortnight ago", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "outbound", At: ago(14 * day)},
		}, "waiting_on_them"},
		{"they wrote a fortnight ago", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "inbound", At: ago(14 * day)},
		}, "waiting_on_us"},
		{"exactly at the waiting threshold", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "outbound", At: ago(7 * day)},
		}, "waiting_on_them"},
		// Dormancy outranks direction: after a quarter, whose move it is stops
		// being a question anyone can act on.
		{"silent for four months, we wrote last", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "outbound", At: ago(120 * day)},
		}, "dormant"},
		{"silent for four months, they wrote last", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "inbound", At: ago(120 * day)},
		}, "dormant"},
		{"just inside dormancy", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "inbound", At: ago(89 * day)},
		}, "waiting_on_us"},
		// A message with no recorded direction cannot say whose move it is. It
		// still proves contact happened, so the account is not "never contacted".
		{"newest message has no direction", suggestionInputs{
			hasNewest: true, newest: lastMessage{Direction: "", At: ago(day)},
		}, "active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(engagementState(tc.in, now)); got != tc.want {
				t.Errorf("engagementState = %q, want %q", got, tc.want)
			}
		})
	}
}

// Every rule that can name what performing it means does so. The client must
// not infer the action from the evidence order — nobody promised that order,
// and a control wired to a guess is worse than none (AC-company-14).
func TestEachSuggestionNamesWhatPerformingItMeans(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	org := ids.From[ids.OrganizationKind](ids.NewV7())
	unanswered := ids.NewV7()

	stale := staleThread(org, now, lastMessage{
		ID: unanswered, Direction: "outbound", At: now.AddDate(0, 0, -14),
	})
	if stale == nil || stale.Action == nil {
		t.Fatalf("the no-reply rule named no action: %+v", stale)
	}
	if stale.Action.Kind != "draft_reply" {
		t.Errorf("no_reply action = %q, want draft_reply", stale.Action.Kind)
	}
	// The composer has to open on the message that went unanswered, not on
	// whichever activity happened to be first in the evidence.
	if stale.Action.ActivityId == nil || ids.UUID(*stale.Action.ActivityId) != unanswered {
		t.Errorf("draft_reply anchors on %v, want the unanswered message %v", stale.Action.ActivityId, unanswered)
	}

	dealID := ids.NewV7()
	stalled := stalledDealSuggestions([]stalledDeal{{ID: dealID, Name: "Retrofit"}})
	if len(stalled) != 1 || stalled[0].Action == nil {
		t.Fatalf("the stalled-deal rule named no action: %+v", stalled)
	}
	if stalled[0].Action.Kind != "open_deal" || stalled[0].Action.DealId == nil ||
		ids.UUID(*stalled[0].Action.DealId) != dealID {
		t.Errorf("stalled_deal action = %+v, want open_deal on %v", stalled[0].Action, dealID)
	}

	next := noNextStepSuggestion(org, suggestionInputs{open: pipeline{OpenCount: 2, OpenDigest: "d"}})
	if next == nil || next.Action == nil || next.Action.Kind != "add_task" {
		t.Fatalf("the no-next-step rule named no add_task action: %+v", next)
	}
	// Several deals are open, so naming one would be a guess dressed as advice.
	if next.Action.DealId != nil {
		t.Errorf("add_task picked deal %v for an account with several open", next.Action.DealId)
	}
}

// The advice has to be checkable: it says there is no next step, and the only
// way a reader can weigh that is against the deals it was read from.
func TestNoNextStepNamesTheDealsItWasReadFrom(t *testing.T) {
	t.Parallel()
	org := ids.OrganizationID{UUID: ids.NewV7()}
	first, second := ids.NewV7(), ids.NewV7()

	one := noNextStepSuggestion(org, suggestionInputs{open: pipeline{
		OpenCount: 1, OpenDigest: "d",
		Open: []openDeal{{ID: first, Name: "PIM rollout Phase 2"}},
	}})
	if one == nil {
		t.Fatal("the no-next-step rule stayed silent on an account with an open deal")
	}
	if !strings.Contains(one.Reason, "PIM rollout Phase 2") {
		t.Errorf("reason = %q, want the deal's own name in it", one.Reason)
	}
	if len(one.Evidence) != 1 || one.Evidence[0].EntityType != "deal" ||
		ids.UUID(one.Evidence[0].EntityId) != first {
		t.Errorf("evidence = %+v, want the open deal itself", one.Evidence)
	}

	// Two deals, and both are cited: a receipt that named one of them would
	// send the reader to check half the claim.
	two := noNextStepSuggestion(org, suggestionInputs{open: pipeline{
		OpenCount: 2, OpenDigest: "d",
		Open: []openDeal{
			{ID: first, Name: "PIM rollout Phase 2"},
			{ID: second, Name: "Data quality add-on"},
		},
	}})
	if two == nil || len(two.Evidence) != 2 {
		t.Fatalf("evidence = %+v, want both open deals", two)
	}
	if !strings.Contains(two.Reason, "Data quality add-on") {
		t.Errorf("reason = %q, want both deals named", two.Reason)
	}

	// A count with no names still states the finding rather than dropping it,
	// and its receipt falls back to the account so the dismissal stays keyed to
	// this record rather than to an empty list.
	bare := noNextStepSuggestion(org, suggestionInputs{open: pipeline{OpenCount: 4, OpenDigest: "d"}})
	if bare == nil || len(bare.Evidence) != 1 ||
		bare.Evidence[0].EntityType != "organization" {
		t.Fatalf("evidence = %+v, want the account itself when no deal survived the read", bare)
	}
	if !strings.Contains(bare.Reason, "4 deals are open") {
		t.Errorf("reason = %q, want the count when there are no names", bare.Reason)
	}
}
