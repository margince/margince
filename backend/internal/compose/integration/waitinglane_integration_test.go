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
// way capture would leave it: a thread key, an audience, a direction.
//
// It also files the message under a PERSON, because that is what makes it sales
// mail. The lane requires a link to a record the workspace sells to, so a
// message seeded without one is a rep's private correspondence and correctly
// never appears — every case in this file that is about something else has to
// clear that bar first or it would pass for the wrong reason.
//
// Returns the activity id so a caller can link it to a further record — the
// entity-filter tests below file the same message under a second person or
// deal on top of the one seeded here.
func seedWaitingMessage(t *testing.T, e *Env, thread, direction, subject string, at time.Time) ids.UUID {
	t.Helper()
	return seedWaitingMessageLinked(t, e, thread, direction, subject, at, seedWaitingPerson(t, e))
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
) ids.UUID {
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
		return id
	}
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, id, person); err != nil {
		t.Fatalf("filing the message under a person: %v", err)
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

// Money outlives the horizon. A wait past ninety days with an open deal on it
// still reaches the reader, because the caller's staleness rule promises to keep
// exactly that case and a horizon that removed it first would leave that promise
// with nothing to act on.
func TestAnOldWaitWithAnOpenDealSurvivesTheHorizon(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	deal := seedWaitingDeal(t, "open")
	seedWaitingMessageLinked(t, e, "thread-old-funded", "inbound", "Still open",
		waitingInstant.Add(-200*24*time.Hour), person)
	linkActivityToDeal(t, "Still open", deal)

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	if !containsSubject(waiting, "Still open") {
		t.Fatal("a 200-day wait on an OPEN deal was dropped by the horizon")
	}
}

// A thread with an open deal on it says so, and one without says so too.
//
// Both halves in one test because the flag only means something as a pair: a
// query that answered true for everything would pass the first half alone, and
// that is the answer that keeps every ancient thread in the day forever.
func TestAWaitReportsWhetherAnOpenDealIsOnIt(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	deal := seedWaitingDeal(t, "open")
	seedWaitingMessageLinked(t, e, "thread-funded", "inbound", "Funded thread",
		waitingInstant.Add(-2*24*time.Hour), person)
	linkActivityToDeal(t, "Funded thread", deal)
	seedWaitingMessageLinked(t, e, "thread-unfunded", "inbound", "Unfunded thread",
		waitingInstant.Add(-2*24*time.Hour), person)

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	assertOpenDeal(t, waiting, "Funded thread", true)
	assertOpenDeal(t, waiting, "Unfunded thread", false)
}

// A deal that is WON is not money still on the thread. Without the status
// predicate every closed deal would keep its thread in the day forever.
func TestAClosedDealIsNotAnOpenOne(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	deal := seedWaitingDeal(t, "won")
	seedWaitingMessageLinked(t, e, "thread-won", "inbound", "Won thread",
		waitingInstant.Add(-2*24*time.Hour), person)
	linkActivityToDeal(t, "Won thread", deal)

	waiting, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}

	assertOpenDeal(t, waiting, "Won thread", false)
}

// seedWaitingDeal creates one deal in the given status, with the pipeline and
// stage every deal needs. A closed deal carries a closed_at, which its own CHECK
// constraint requires.
func seedWaitingDeal(t *testing.T, status string) ids.UUID {
	t.Helper()
	owner := OwnerConn(t)
	pipeline := SeedIDRow(t, owner, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := SeedIDRow(t, owner, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	var closedAt any
	if status != "open" {
		closedAt = waitingInstant.Add(-24 * time.Hour)
	}
	return SeedIDRow(t, owner, `INSERT INTO deal (id, name, pipeline_id, stage_id, status, closed_at, source, captured_by)
		VALUES ($1, 'A deal', $2, $3, $4, $5, 'manual', 'human:x')`,
		pipeline, stage, status, closedAt)
}

// linkActivityToDeal files an already-seeded message under a deal as well.
func linkActivityToDeal(t *testing.T, subject string, deal ids.UUID) {
	t.Helper()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, deal_id)
		SELECT id, 'deal', $1 FROM activity WHERE subject = $2`,
		deal, subject); err != nil {
		t.Fatalf("filing %q under a deal: %v", subject, err)
	}
}

func assertOpenDeal(t *testing.T, rows []activities.WaitingReply, subject string, want bool) {
	t.Helper()
	for _, row := range rows {
		if row.Subject != subject {
			continue
		}
		if row.HasOpenDeal != want {
			t.Fatalf("%q reported HasOpenDeal=%v, wanted %v", subject, row.HasOpenDeal, want)
		}
		return
	}
	t.Fatalf("%q never reached the reader at all", subject)
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
	// Zero person: no auto-link, so the ONLY sales link this message carries
	// is the one below — otherwise the message would still be admitted
	// through a public link and this would test nothing.
	hidden := seedWaitingMessageLinked(t, e, "thread-entity-private", "inbound", "Re: private deal",
		waitingInstant.Add(-3*24*time.Hour), ids.UUID{})
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
