// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who is waiting, against a real database.
//
// Every claim this lane makes is SQL — the content gate, the link visibility,
// the per-thread anti-joins, the tie-break — and none of them can fail in a
// unit test with hand-built rows. The admission is asserted as hard as the
// refusal: a gate that refused everyone would pass a refusal-only suite while
// answering nobody, which is the failure this file exists to catch.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var waitingInstant = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

// seedMessage writes one captured message through the owner connection, the
// way capture would leave it: a thread key, an audience, a direction.
func seedWaitingMessage(t *testing.T, e *Env, thread, direction, subject string, at time.Time) {
	t.Helper()
	id := ids.NewV7()
	owner := OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, direction, subject, occurred_at, is_done, source,
		                      captured_by, version, created_at, updated_at,
		                      counterparty_outbound_attested, thread_key, audience)
		VALUES ($1, 'email', $2, $3, $4, false, 'system', $5, 1, now(), now(), false, $6, 'workspace')`,
		id, direction, subject, at, "human:"+e.AdminUser.String(), thread); err != nil {
		t.Fatalf("seeding a %s message: %v", direction, err)
	}
}

// An unanswered message reaches the reader. The ADMISSION case, first: a lane
// that answered nobody would pass every refusal test in this file.
func TestAnUnansweredMessageReachesTheReader(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-waiting-admit", "inbound", "Re: pricing", waitingInstant.Add(-83*24*time.Hour))

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if !containsSubject(waiting, "Re: pricing") {
		t.Fatalf("an unanswered message did not reach the reader: got %d row(s)", len(waiting))
	}
}

// A message somebody answered is not waiting. The reply ends the wait even
// though it is a separate row in the same thread.
func TestAnAnsweredMessageIsNotWaiting(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-waiting-answered", "inbound", "Re: timing", waitingInstant.Add(-10*24*time.Hour))
	seedWaitingMessage(t, e, "thread-waiting-answered", "outbound", "Re: timing", waitingInstant.Add(-9*24*time.Hour))

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if containsSubject(waiting, "Re: timing") {
		t.Fatal("a message that was answered is still reported as waiting")
	}
}

// Only the NEWEST inbound of a thread is the wait. A customer who wrote three
// times is waiting once, and three rows would read as three obligations.
func TestOnlyTheNewestInboundOfAThreadIsTheWait(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-waiting-many", "inbound", "First ask", waitingInstant.Add(-20*24*time.Hour))
	seedWaitingMessage(t, e, "thread-waiting-many", "inbound", "Second ask", waitingInstant.Add(-2*24*time.Hour))

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if containsSubject(waiting, "First ask") {
		t.Fatal("an older message in the same thread is reported alongside the newest")
	}
	if !containsSubject(waiting, "Second ask") {
		t.Fatal("the newest message in the thread is not reported as the wait")
	}
}

// A message dated after the read instant cannot suppress a thread waiting now.
// Mail carries the sender's own Date header, so a future date is a thing that
// happens rather than a thing that cannot.
func TestAFutureDatedMessageDoesNotSuppressTheWaitingOne(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-waiting-future", "inbound", "Waiting now", waitingInstant.Add(-5*24*time.Hour))
	seedWaitingMessage(t, e, "thread-waiting-future", "outbound", "Dated ahead", waitingInstant.Add(365*24*time.Hour))

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if !containsSubject(waiting, "Waiting now") {
		t.Fatal("a message dated a year ahead silenced a thread that is waiting today")
	}
}

func containsSubject(rows []activities.WaitingReply, subject string) bool {
	for _, row := range rows {
		if row.Subject == subject {
			return true
		}
	}
	return false
}
