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
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var waitingInstant = time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)

// seedMessage writes one captured message through the owner connection, the
// way capture would leave it: a thread key, an audience, a direction. Returns
// the activity id so a caller can link it to a record.
func seedWaitingMessage(t *testing.T, e *Env, thread, direction, subject string, at time.Time) ids.UUID {
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
	return id
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

// The record page's own read: GET /activities?waiting_reply=true, scoped to
// one entity. It shares WaitingReplies' thread walk (waitingReplyExistsClause
// embeds waitingRepliesSQL as a subquery) rather than a second copy of it, so
// these are proofs of the SAME engine narrowed to a record, not of a
// parallel one.

// The entity filter and the wait narrow to an intersection: only the message
// that is BOTH linked to this person AND still unanswered comes back — not
// every unanswered thread in the workspace, and not every message on this
// person.
func TestWaitingReplyListFilterFindsTheEntitysUnansweredThread(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	admin := e.Admin()

	dana := e.SeedPerson(t, "Dana Buyer", &e.Rep1)
	someoneElse := e.SeedPerson(t, "Someone Else", &e.Rep1)
	theirs := seedWaitingMessage(t, e, "thread-entity-mine", "inbound", "Re: contract", waitingInstant.Add(-3*24*time.Hour))
	LinkActivity(t, owner, theirs, "person", dana)
	notTheirs := seedWaitingMessage(t, e, "thread-entity-other", "inbound", "Re: unrelated", waitingInstant.Add(-3*24*time.Hour))
	LinkActivity(t, owner, notTheirs, "person", someoneElse)

	asOf := waitingInstant
	et := "person"
	got, _, err := e.Activities.ListActivities(admin, activities.ListActivitiesInput{
		EntityType: &et, EntityID: &dana, WaitingReplyAsOf: &asOf,
	})
	if err != nil {
		t.Fatalf("listing waiting replies for the entity: %v", err)
	}
	if len(got) != 1 || ids.UUID(got[0].Id) != theirs {
		t.Fatalf("waiting_reply on person %v = %v, want the one unanswered message linked to them", dana, got)
	}
}

// An answered thread contributes nothing to the entity's waiting list either
// — the same rule WaitingReplies enforces workspace-wide.
func TestWaitingReplyListFilterOmitsAnAnsweredThread(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	admin := e.Admin()

	dana := e.SeedPerson(t, "Dana Buyer", &e.Rep1)
	inbound := seedWaitingMessage(t, e, "thread-entity-answered", "inbound", "Re: timing", waitingInstant.Add(-10*24*time.Hour))
	LinkActivity(t, owner, inbound, "person", dana)
	outbound := seedWaitingMessage(t, e, "thread-entity-answered", "outbound", "Re: timing", waitingInstant.Add(-9*24*time.Hour))
	LinkActivity(t, owner, outbound, "person", dana)

	asOf := waitingInstant
	et := "person"
	got, _, err := e.Activities.ListActivities(admin, activities.ListActivitiesInput{
		EntityType: &et, EntityID: &dana, WaitingReplyAsOf: &asOf,
	})
	if err != nil {
		t.Fatalf("listing waiting replies for the entity: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("waiting_reply on an answered thread = %v, want none", got)
	}
}

// The row-scope gate applies to the filtered read exactly as it does to the
// unfiltered one: a message reachable only through a colleague's
// capture-private contact does not surface just because the caller asked
// waiting_reply=true instead of the plain list.
func TestWaitingReplyListFilterDropsRowsOutOfCallersScope(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	theirPrivate := e.SeedPerson(t, "Their Private Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "person", theirPrivate, e.Rep3)
	hidden := seedWaitingMessage(t, e, "thread-entity-private", "inbound", "Re: private deal", waitingInstant.Add(-3*24*time.Hour))
	LinkActivity(t, owner, hidden, "person", theirPrivate)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLinkRepPerms)
	asOf := waitingInstant
	got, _, err := e.Activities.ListActivities(rep, activities.ListActivitiesInput{WaitingReplyAsOf: &asOf})
	if err != nil {
		t.Fatalf("listing waiting replies workspace-wide: %v", err)
	}
	for _, a := range got {
		if ids.UUID(a.Id) == hidden {
			t.Fatalf("waiting_reply surfaced a message reachable only through a contact %v cannot read", theirPrivate)
		}
	}
}

// Filtering by an out-of-scope entity refuses the same way the plain list
// already does — waiting_reply changes what the list narrows to, not the
// existence-hiding gate that runs before any SQL is built.
func TestWaitingReplyListFilterRefusesAnOutOfScopeEntity(t *testing.T) {
	e := Setup(t)

	theirPrivate := e.SeedPerson(t, "Their Private Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "person", theirPrivate, e.Rep3)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLinkRepPerms)
	asOf := waitingInstant
	et := "person"
	_, _, err := e.Activities.ListActivities(rep, activities.ListActivitiesInput{
		EntityType: &et, EntityID: &theirPrivate, WaitingReplyAsOf: &asOf,
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("waiting_reply on an out-of-scope entity = %v, want ErrNotFound", err)
	}
}
