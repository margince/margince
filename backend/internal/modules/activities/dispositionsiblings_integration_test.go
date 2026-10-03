// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// requestsInOneConversation seeds three requests on one thread, oldest first,
// and returns them in that order.
func requestsInOneConversation(t *testing.T, e *loadEnv) []ids.UUID {
	t.Helper()
	thread := "conversation-" + ids.NewV7().String()
	out := make([]ids.UUID, 0, 3)
	for i, subject := range []string{"Please send slots", "Re: Please send slots", "Re: Re: Please send slots"} {
		id := seedEmailRequest(t, e, subject, "commitment", OwedVerdictAsksUs)
		e.exec(t, `UPDATE activity SET thread_key = $2, occurred_at = $3 WHERE id = $1`,
			id, thread, requestInstant.Add(time.Duration(i)*time.Hour))
		out = append(out, id)
	}
	return out
}

// Setting the card aside sets the conversation's earlier requests aside with
// it, and undoing it brings back exactly those.
func TestSettingACardAsideReachesItsEarlierRequests(t *testing.T) {
	e := setupLoad(t)
	asks := requestsInOneConversation(t, e)
	alone := seedEmailRequest(t, e, "Unrelated question", "commitment", OwedVerdictAsksUs)
	rep, store := e.asSeat(e.rep), storeKnowing(e)
	at := requestInstant.Add(4 * time.Hour)

	if err := store.SetMessageNotMine(rep, ids.From[ids.ActivityKind](asks[2])); err != nil {
		t.Fatalf("setting the card aside: %v", err)
	}
	waiting := e.waitingAt(rep, t, at)
	for _, id := range asks {
		if waiting[id] {
			t.Errorf("request %v of the set-aside conversation is still waiting", id)
		}
	}
	if !waiting[alone] {
		t.Error("a request in another conversation was set aside with the card")
	}

	if err := store.ClearMessageDisposition(rep, ids.From[ids.ActivityKind](asks[2])); err != nil {
		t.Fatalf("undoing: %v", err)
	}
	waiting = e.waitingAt(rep, t, at)
	for _, id := range asks {
		if !waiting[id] {
			t.Errorf("request %v did not come back with the card", id)
		}
	}
}
