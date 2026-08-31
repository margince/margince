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
//
// It also files the message under a PERSON, because that is what makes it sales
// mail. The lane requires a link to a record the workspace sells to, so a
// message seeded without one is a rep's private correspondence and correctly
// never appears — every case in this file that is about something else has to
// clear that bar first or it would pass for the wrong reason.
func seedWaitingMessage(t *testing.T, e *Env, thread, direction, subject string, at time.Time) {
	t.Helper()
	seedWaitingMessageLinked(t, e, thread, direction, subject, at, seedWaitingPerson(t, e))
}

// seedWaitingPerson creates one person for a message to be filed under.
func seedWaitingPerson(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO person (id, full_name, source, captured_by, version, created_at, updated_at)
		VALUES ($1, 'Waiting Customer', 'system', $2, 1, now(), now())`,
		id, "human:"+e.AdminUser.String()); err != nil {
		t.Fatalf("seeding the person a thread is filed under: %v", err)
	}
	return id
}

// seedWaitingMessageLinked writes the message and files it under one person.
// Pass a zero person to seed mail linked to NOTHING, which is how the personal
// -mail case is built.
func seedWaitingMessageLinked(
	t *testing.T, e *Env, thread, direction, subject string, at time.Time, person ids.UUID,
) {
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
	if person.IsZero() {
		return
	}
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, id, person); err != nil {
		t.Fatalf("filing the message under a person: %v", err)
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

// Mail linked to nothing is not sales work. This is the reported defect in its
// simplest form: a rep's dentist wrote, nobody replied, and the queue called it
// a customer waiting because unanswered was the only test it applied.
func TestMailLinkedToNothingIsNotWaitingWork(t *testing.T) {
	e := Setup(t)
	seedWaitingMessageLinked(t, e, "thread-personal", "inbound", "Your appointment",
		waitingInstant.Add(-3*24*time.Hour), ids.UUID{})

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if containsSubject(waiting, "Your appointment") {
		t.Fatal("mail filed under no record at all was reported as a customer waiting")
	}
}

// Past the horizon a wait is history. The bands decide what is urgent among
// what survives; this decides what is still a wait at all.
func TestAWaitOlderThanTheHorizonIsNotReported(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-ancient", "inbound", "Six months ago",
		waitingInstant.Add(-200*24*time.Hour))
	seedWaitingMessage(t, e, "thread-recent", "inbound", "Last week",
		waitingInstant.Add(-7*24*time.Hour))

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if containsSubject(waiting, "Six months ago") {
		t.Fatal("a two-hundred-day-old thread was still reported as a wait")
	}
	// The admission half: the horizon must not have swallowed everything.
	if !containsSubject(waiting, "Last week") {
		t.Fatal("the horizon removed a week-old wait, which is ordinary work")
	}
}

// The wait carries whoever answers for the record, so "mine" can mean mine.
// Without this the queue has no owner to compare against and falls back to
// showing unowned work to everybody.
func TestAWaitCarriesTheOwnerOfItsRecord(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE person SET owner_id = $1 WHERE id = $2`, e.AdminUser, person); err != nil {
		t.Fatalf("giving the person an owner: %v", err)
	}
	seedWaitingMessageLinked(t, e, "thread-owned", "inbound", "Owned thread",
		waitingInstant.Add(-2*24*time.Hour), person)

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	for _, row := range waiting {
		if row.Subject != "Owned thread" {
			continue
		}
		if row.OwnerID != ids.UUID(e.AdminUser) {
			t.Fatalf("the wait came back owned by %v, wanted the person's owner %v",
				row.OwnerID, e.AdminUser)
		}
		return
	}
	t.Fatal("the owned thread never reached the reader at all")
}

// An unowned record yields an unowned wait. Zero is the routing answer that
// sends it to the unassigned queue; guessing an owner here is what put every
// colleague's mail on everybody's page.
func TestAWaitOnAnUnownedRecordHasNoOwner(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-unowned", "inbound", "Unowned thread",
		waitingInstant.Add(-2*24*time.Hour))

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	for _, row := range waiting {
		if row.Subject != "Unowned thread" {
			continue
		}
		if !row.OwnerID.IsZero() {
			t.Fatalf("an unowned record produced owner %v", row.OwnerID)
		}
		return
	}
	t.Fatal("the unowned thread never reached the reader at all")
}

func containsSubject(rows []activities.WaitingReply, subject string) bool {
	for _, row := range rows {
		if row.Subject == subject {
			return true
		}
	}
	return false
}
