// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Three requests in one conversation are one card: the newest, carrying how
// many came before and when the first arrived. Another conversation, and mail
// that asks nothing, keep their own rows.
func TestAConversationsRequestsAreOneCard(t *testing.T) {
	at := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	ask := func(thread string, hours int) activities.WaitingReply {
		return activities.WaitingReply{
			ActivityID: ids.NewV7(), ThreadKey: thread, Sender: "buyer@customer.example",
			OccurredAt: at.Add(time.Duration(hours) * time.Hour), OwedVerdict: activities.OwedVerdictAsksUs,
		}
	}
	middle, newest, first, elsewhere := ask("t1", 1), ask("t1", 2), ask("t1", 0), ask("t2", 1)
	note := activities.WaitingReply{ActivityID: ids.NewV7(), ThreadKey: "t1", Sender: "buyer@customer.example", OccurredAt: at}

	kept := keepWaitingCustomers([]activities.WaitingReply{middle, newest, elsewhere, first, note})

	if len(kept) != 3 {
		t.Fatalf("kept %d rows, want the card, the other conversation and the note", len(kept))
	}
	card := kept[0]
	if card.ActivityID != newest.ActivityID {
		t.Errorf("the card is %v, want the newest request %v", card.ActivityID, newest.ActivityID)
	}
	if card.EarlierRequests != 2 || !card.FirstAskedAt.Equal(first.OccurredAt) {
		t.Errorf("the card holds %d earlier requests from %v, want 2 from %v",
			card.EarlierRequests, card.FirstAskedAt, first.OccurredAt)
	}
	if kept[1].ActivityID != elsewhere.ActivityID || kept[1].EarlierRequests != 0 {
		t.Errorf("another conversation's request was folded: %+v", kept[1])
	}
}

// The lane folds each scan page and then the pages together, so two cards of
// one conversation merge their counts and keep the earliest first request.
func TestTwoCardsOfOneConversationMergeTheirCounts(t *testing.T) {
	at := time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)
	newer := activities.WaitingReply{
		ActivityID: ids.NewV7(), OccurredAt: at.Add(5 * time.Hour),
		EarlierRequests: 1, FirstAskedAt: at.Add(4 * time.Hour),
	}
	older := activities.WaitingReply{
		ActivityID: ids.NewV7(), OccurredAt: at.Add(2 * time.Hour),
		EarlierRequests: 2, FirstAskedAt: at,
	}

	foldIntoCard(&older, newer)

	if older.ActivityID != newer.ActivityID || older.EarlierRequests != 4 || !older.FirstAskedAt.Equal(at) {
		t.Errorf("merged card = %v with %d earlier from %v, want %v with 4 from %v",
			older.ActivityID, older.EarlierRequests, older.FirstAskedAt, newer.ActivityID, at)
	}
}
