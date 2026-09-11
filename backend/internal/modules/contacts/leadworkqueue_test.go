// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLeadQueueCursorRoundTrip(t *testing.T) {
	want := leadQueueCursor{
		AsOf: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC), Rank: 1, Score: 73,
		CreatedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC), ID: ids.NewV7(),
	}
	token, err := encodeLeadQueueCursor(want)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	got, err := decodeLeadQueueCursor(token)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestLeadQueueCursorRejectsMalformedAndImpossibleValues(t *testing.T) {
	outOfRange, err := encodeLeadQueueCursor(leadQueueCursor{
		AsOf: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC), Rank: leadQueueRankInactive + 1, Score: 50,
		CreatedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC), ID: ids.NewV7(),
	})
	if err != nil {
		t.Fatalf("encode out-of-range cursor: %v", err)
	}
	tests := []string{
		"not-base64",
		"e30", // {}
		outOfRange,
	}
	for _, token := range tests {
		if _, err := decodeLeadQueueCursor(token); !errors.As(err, new(*storekit.MalformedCursorError)) {
			t.Errorf("decode %q error = %v, want MalformedCursorError", token, err)
		}
	}
}

// With no first-response target, an unanswered lead still outranks an answered
// one.
//
// The rank used to collapse to a single constant when the policy was off, so
// every lead shared a band and the ordering fell through to score. An answered
// lead with a high score then sorted above an unanswered one — and because the
// queue's readers take a bounded page, a page of answered leads could fill the
// limit and hide the leads that actually owe a reply. The queue's own question
// answered backwards.
func TestTheLeadQueueRanksUnansweredFirstWithNoTarget(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	rank := leadQueueRank(leadSLAPolicy{}, arg, time.Now())

	// It must still DISCRIMINATE: a bare constant is what let score decide.
	if !strings.Contains(rank, "first_response_at IS NOT NULL") {
		t.Errorf("rank with no target = %q, which cannot tell an answered lead from an owed one", rank)
	}
	if !strings.Contains(rank, "archived_at IS NOT NULL") {
		t.Error("an archived lead is not owed a reply and must not be ranked as if it were")
	}
	// And it reads nothing from the policy it does not have.
	if len(args) != 0 {
		t.Errorf("the no-target rank bound %d arguments, want none", len(args))
	}
}
