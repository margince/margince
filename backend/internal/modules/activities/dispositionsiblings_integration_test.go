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

// A judgement the reader already made on an earlier request survives setting
// the card aside and undoing it: the card only fills in where there was none.
func TestACardNeverOverwritesAnEarlierJudgement(t *testing.T) {
	e := setupLoad(t)
	asks := requestsInOneConversation(t, e)
	rep, store := e.asSeat(e.rep), storeKnowing(e)
	if err := store.SetMessageNotMine(rep, ids.From[ids.ActivityKind](asks[0])); err != nil {
		t.Fatalf("judging the first request on its own: %v", err)
	}
	card := ids.From[ids.ActivityKind](asks[2])
	if err := store.SetMessageNotMine(rep, card); err != nil {
		t.Fatalf("setting the card aside: %v", err)
	}
	if err := store.ClearMessageDisposition(rep, card); err != nil {
		t.Fatalf("undoing: %v", err)
	}
	waiting := e.waitingAt(rep, t, requestInstant.Add(4*time.Hour))
	if waiting[asks[0]] {
		t.Error("undoing the card erased the reader's own earlier judgement")
	}
	if !waiting[asks[1]] {
		t.Error("the request the card set aside did not come back")
	}
}

// A time snooze that has already lapsed hides nothing, so the card's
// set-aside reaches that request as if it had no state.
func TestACardReachesARequestWhoseSnoozeHasLapsed(t *testing.T) {
	e := setupLoad(t)
	asks := requestsInOneConversation(t, e)
	rep, store := e.asSeat(e.rep), storeKnowing(e)
	e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, snoozed_until, reopen_on, set_by, set_at)
		VALUES ($1, $2, 'snoozed', now() - interval '1 day', 'time', 'human:test', now() - interval '3 days')`, asks[0], e.rep)

	if err := store.SetMessageNotMine(rep, ids.From[ids.ActivityKind](asks[2])); err != nil {
		t.Fatalf("setting the card aside: %v", err)
	}
	if e.waitingAt(rep, t, time.Now())[asks[0]] {
		t.Error("a request whose snooze had lapsed came back while its card was set aside")
	}
}
