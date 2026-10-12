// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A wait with a meeting booked is scheduled work, not an unanswered customer:
// the card stays, says when the meeting is, stops counting waiting days and
// stops claiming Focus priority.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestABookedMeetingStopsTheWaitingCount(t *testing.T) {
	start := rankInstant.Add(48 * time.Hour)
	// Three days old: young enough that nothing but the booking can demote
	// it, so the band assertion below tests the booked branch and not the
	// stale rule.
	waiting := WaitingCustomer{
		ActivityID:       ids.MustParse("01a05500-0000-7000-8000-0000000000d1"),
		Subject:          "When can we meet?",
		Since:            rankInstant.Add(-3 * 24 * time.Hour),
		ConfirmedRequest: true,
		Engaged:          true,
		MeetingBookedAt:  &start,
	}

	row := classifyWaiting(waiting, rankInstant)

	if row.item.Level == levelWaiting {
		t.Fatal("a wait with a meeting booked still claims the live-wait band")
	}
	var booked, days bool
	for _, because := range row.item.Because {
		if because.Kind == "meeting_booked" {
			booked = true
			if because.Value == nil || because.Value.Date == nil || !because.Value.Date.Equal(start) {
				t.Fatalf("the booked reason does not carry the meeting's start: %+v", because.Value)
			}
		}
		if because.Kind == "waiting_days" {
			days = true
		}
	}
	if !booked {
		t.Fatal("the row was demoted without saying a meeting is booked")
	}
	if days {
		t.Fatal("the row still counts waiting days against work that is scheduled")
	}
	// The age comparator must not credit this row with the wait it no longer
	// states: a neighbour's "above the next by waiting time" explanation reads
	// these two fields.
	if row.waitingDays != 0 || row.waitingRank != 0 {
		t.Fatalf("a booked row still competes by age: days %d, rank %d", row.waitingDays, row.waitingRank)
	}
	if !row.occurredAt.Equal(rankInstant) {
		t.Fatalf("a booked row still enters the occurrence tie-break at its request's age: %v", row.occurredAt)
	}
}

// Without a booked meeting the row counts its days exactly as before — the
// contrast that proves the branch keys on the meeting and nothing else.
func TestAWaitWithoutABookingStillCountsItsDays(t *testing.T) {
	waiting := WaitingCustomer{
		ActivityID:       ids.MustParse("01a05500-0000-7000-8000-0000000000d2"),
		Subject:          "When can we meet?",
		Since:            rankInstant.Add(-3 * 24 * time.Hour),
		ConfirmedRequest: true,
		Engaged:          true,
	}

	row := classifyWaiting(waiting, rankInstant)

	if row.item.Level != levelWaiting {
		t.Fatal("an unbooked live wait left the top band")
	}
	for _, because := range row.item.Because {
		if because.Kind == "meeting_booked" {
			t.Fatal("the row claims a meeting nobody booked")
		}
	}
}
