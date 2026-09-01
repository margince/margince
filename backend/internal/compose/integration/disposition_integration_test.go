// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a rep set aside stays set aside — and stays set aside for THEM.
//
// The whole feature is two anti-joins in one SQL statement, so none of it can
// be proved with hand-built rows. The claim that costs the most if it is wrong
// is the one this file leads with: a snooze belongs to the person who set it,
// and a rep clearing their own morning must not clear a colleague's.
//
// Every refusal here is paired with an admission over the same fixture. A read
// that returned nothing would pass a suite of hide-tests while showing nobody
// their day.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitsFor reads the waiting lane as one person and reports whether the subject
// is on their day.
func waitsFor(t *testing.T, e *Env, user ids.UUID, subject string) bool {
	t.Helper()
	rows, err := activities.NewStore(e.DB()).WaitingReplies(
		e.As(user, nil, AdminPerms), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	return containsSubject(rows, subject)
}

// waitingMessageID finds the seeded message by its subject, because the seed
// helpers mint ids they do not return.
func waitingMessageID(t *testing.T, e *Env, subject string) ids.ActivityID {
	t.Helper()
	rows, err := activities.NewStore(e.DB()).WaitingReplies(e.Admin(), waitingInstant)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	for _, row := range rows {
		if row.Subject == subject {
			return ids.From[ids.ActivityKind](row.ActivityID)
		}
	}
	t.Fatalf("no waiting row for %q — the fixture never reached the lane", subject)
	return ids.ActivityID{}
}

// atWaitingInstant is a store whose clock reads the fixture's instant.
//
// A snooze is refused into the past, and every fixture here is dated before the
// wall clock — so without this the suite could not set one at all, and the two
// snooze cases would be asserting a refusal they never meant to trigger. The
// store carries WithClock for exactly this.
func atWaitingInstant(e *Env) *activities.Store {
	return activities.NewStore(e.DB()).WithClock(func() time.Time { return waitingInstant })
}

// ONE REP'S SNOOZE IS THEIR OWN.
//
// The failure this guards against is silent: the row simply stops appearing for
// a colleague who never judged it, and no screen anywhere says somebody else
// put it down. That is why the personal states live in their own table keyed on
// the reader, and why this test asserts BOTH halves of the same fixture.
func TestOneRepsSnoozeDoesNotHideTheRowFromAnother(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-snooze-mine", "inbound", "Snoozed by one",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Snoozed by one")

	// Both see it before anybody judges it, or the assertion below is a query
	// that never matched rather than a change.
	if !waitsFor(t, e, e.Rep1, "Snoozed by one") {
		t.Fatal("the first rep never saw the message, so this proves nothing about hiding it")
	}
	if !waitsFor(t, e, e.Rep2, "Snoozed by one") {
		t.Fatal("the second rep never saw the message, so this proves nothing about hiding it")
	}

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, waitingInstant.Add(48*time.Hour)); err != nil {
		t.Fatalf("the first rep snoozing the message: %v", err)
	}

	if waitsFor(t, e, e.Rep1, "Snoozed by one") {
		t.Fatal("a rep's own snooze did not take: the message is still on their day")
	}
	if !waitsFor(t, e, e.Rep2, "Snoozed by one") {
		t.Fatal("one rep's snooze took the message off a COLLEAGUE's day, who never judged it")
	}
}

// `not_mine` is the same shape and is asserted separately: it takes a different
// branch of the anti-join, and a rule that held for one state and not the other
// would pass a suite that only tested snooze.
func TestOneRepsNotMineDoesNotHideTheRowFromAnother(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-notmine", "inbound", "Not mine to answer",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Not mine to answer")

	store := activities.NewStore(e.DB())
	if err := store.SetMessageNotMine(e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("the first rep handing the message on: %v", err)
	}

	if waitsFor(t, e, e.Rep1, "Not mine to answer") {
		t.Fatal("not-mine did not take: the message is still on the rep's day")
	}
	if !waitsFor(t, e, e.Rep2, "Not mine to answer") {
		t.Fatal("one rep's not-mine took the message off the colleague it was handed TO")
	}
}

// A DISPOSITION IS REFUSED ON A MESSAGE THE CALLER MAY NOT READ, and refused
// the same way as one that is not there.
//
// The schema fitness gate waives activity_reader_state.activity_id on exactly
// this claim: the write is gated, so the FK is not the thing standing between a
// rep and a message they have no business knowing exists. A waiver is a claim,
// and this is the test that holds it — without one, a change to judgeMessage
// would leave the waiver standing over nothing.
//
// INDISTINGUISHABLE is the point. If "you may not read this" and "there is no
// such message" answered differently, setting one aside would be a way to ask
// whether it exists — and the answer is itself the disclosure.
func TestSettingAsideAMessageTheReaderCannotSeeIsRefusedAsAbsent(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-unreadable", "inbound", "Held from this reader",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Held from this reader")
	store := activities.NewStore(e.DB())

	// THE ADMISSION FIRST. While the message is workspace-wide the same call
	// succeeds, so the refusal below is about what the reader may see rather
	// than about the write being broken.
	if err := store.SetMessageNotMine(e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("a readable message could not be set aside: %v — the refusal below would prove nothing", err)
	}
	if _, err := OwnerConn(t).Exec(context.Background(),
		`DELETE FROM activity_reader_state WHERE activity_id = $1`, id); err != nil {
		t.Fatalf("clearing the admission: %v", err)
	}

	// Now held to its participants, of whom this rep is not one.
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, id); err != nil {
		t.Fatalf("narrowing the message: %v", err)
	}

	err := store.SetMessageNotMine(e.As(e.Rep1, nil, AdminPerms), id)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("setting aside a message the reader may not open answered %v, want ErrNotFound — the "+
			"reader-state row is what would tell them it exists", err)
	}

	absent := ids.From[ids.ActivityKind](ids.NewV7())
	missing := store.SetMessageNotMine(e.As(e.Rep1, nil, AdminPerms), absent)
	if !errors.Is(missing, apperrors.ErrNotFound) {
		t.Fatalf("setting aside a message that does not exist answered %v, want ErrNotFound", missing)
	}

	var rows int
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM activity_reader_state WHERE activity_id = $1`, id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d reader-state row(s) landed for a message the caller may not read", rows)
	}
}

// NOT SALES BINDS EVERYBODY, which is the opposite reach and the reason there
// are two tables rather than one with a flag.
func TestNotSalesHidesTheThreadFromEveryone(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-notsales", "inbound", "Procurement newsletter",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Procurement newsletter")

	store := activities.NewStore(e.DB())
	if err := store.SetThreadNotSales(e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("judging the thread not sales: %v", err)
	}

	if waitsFor(t, e, e.Rep1, "Procurement newsletter") {
		t.Fatal("the rep who judged the thread still sees it")
	}
	if waitsFor(t, e, e.Rep2, "Procurement newsletter") {
		t.Fatal("a thread judged not-sales still reaches a colleague, so the judgement is not about the thread")
	}
}

// A SNOOZE LIFTS ON ITS OWN MOMENT, driven by the read instant rather than by
// waiting for a wall clock to catch up.
func TestASnoozeLiftsWhenItsMomentPasses(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-snooze-lifts", "inbound", "Back on Thursday",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Back on Thursday")

	until := waitingInstant.Add(24 * time.Hour)
	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, until); err != nil {
		t.Fatalf("snoozing the message: %v", err)
	}

	rows, err := activities.NewStore(e.DB()).WaitingReplies(
		e.As(e.Rep1, nil, AdminPerms), waitingInstant)
	if err != nil {
		t.Fatalf("reading before the moment: %v", err)
	}
	if containsSubject(rows, "Back on Thursday") {
		t.Fatal("the message is still on the day before its snooze lifts")
	}

	// One second past the moment. The same rows, the same reader, a later
	// instant — so what changed is the snooze and nothing else.
	rows, err = activities.NewStore(e.DB()).WaitingReplies(
		e.As(e.Rep1, nil, AdminPerms), until.Add(time.Second))
	if err != nil {
		t.Fatalf("reading after the moment: %v", err)
	}
	if !containsSubject(rows, "Back on Thursday") {
		t.Fatal("a snooze whose moment has passed still hides the message, so it never lifts")
	}
}

// PICKING IT BACK UP is the undo behind every set-aside verb, and it clears the
// reader's own state without touching the thread's.
func TestPickingUpRestoresTheRowToItsOwnReaderOnly(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-undo", "inbound", "Picked back up",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Picked back up")

	store := activities.NewStore(e.DB())
	first := e.As(e.Rep1, nil, AdminPerms)
	if err := store.SetMessageNotMine(first, id); err != nil {
		t.Fatalf("setting the message aside: %v", err)
	}
	if waitsFor(t, e, e.Rep1, "Picked back up") {
		t.Fatal("the set-aside did not take, so the undo below proves nothing")
	}

	if err := store.ClearMessageDisposition(first, id); err != nil {
		t.Fatalf("picking the message back up: %v", err)
	}
	if !waitsFor(t, e, e.Rep1, "Picked back up") {
		t.Fatal("picking the message back up did not restore it to the reader's day")
	}
}

// Clearing what was never set is the same success: the reader's goal state
// already holds, and answering an error would make an undo button fail on a row
// somebody else had already restored.
func TestPickingUpAMessageNobodySetAsideIsTheSameSuccess(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-undo-noop", "inbound", "Never put down",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Never put down")

	if err := activities.NewStore(e.DB()).ClearMessageDisposition(
		e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("picking up a message nobody set aside: %v", err)
	}
	if !waitsFor(t, e, e.Rep1, "Never put down") {
		t.Fatal("a no-op undo removed the message from the day")
	}
}

// A SNOOZE INTO THE PAST is refused rather than stored. Storing it would write
// a row that hides nothing, and read to the rep as a snooze that did not take.
func TestASnoozeIntoThePastIsRefused(t *testing.T) {
	e := Setup(t)
	seedWaitingMessage(t, e, "thread-snooze-past", "inbound", "Already gone",
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, "Already gone")

	// Judged against the SAME clock the accepting cases use, so what is refused
	// here is the moment being behind rather than the fixture being historical.
	err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, waitingInstant.Add(-time.Hour))
	if err == nil {
		t.Fatal("a snooze into the past was accepted, and hides nothing")
	}
	if !waitsFor(t, e, e.Rep1, "Already gone") {
		t.Fatal("a refused snooze still took the message off the day")
	}
}

// A JUDGED THREAD STAYS JUDGED WHEN THE SENDER WRITES AGAIN.
//
// This is the case that made the feature's headline false. The judgement was
// keyed on one activity id, so the next inbound — a different row, and now the
// thread's waiting candidate — brought the newsletter straight back onto every
// queue. Keying on the thread is what makes "this conversation is not sales"
// mean what it says.
func TestAJudgedThreadStaysJudgedWhenTheSenderWritesAgain(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, "thread-recurring", "inbound", "Newsletter issue 41",
		waitingInstant.Add(-5*24*time.Hour), person)
	id := waitingMessageID(t, e, "Newsletter issue 41")

	if err := activities.NewStore(e.DB()).SetThreadNotSales(
		e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("judging the thread not sales: %v", err)
	}
	if waitsFor(t, e, e.Rep1, "Newsletter issue 41") {
		t.Fatal("the judgement did not take, so the next assertion proves nothing")
	}

	// The next issue arrives on the SAME thread. It is a different activity,
	// and it is the one the lane would now pick as the thread's wait.
	seedWaitingMessageLinked(t, e, "thread-recurring", "inbound", "Newsletter issue 42",
		waitingInstant.Add(-1*24*time.Hour), person)

	if waitsFor(t, e, e.Rep1, "Newsletter issue 42") {
		t.Fatal("a judged thread came back as fresh work when the sender wrote again")
	}
	// And for everybody, because the judgement is about the conversation.
	if waitsFor(t, e, e.Rep2, "Newsletter issue 42") {
		t.Fatal("the next issue of a judged thread reached a colleague")
	}
}

// Only an unanswered INBOUND MESSAGE carries a disposition.
//
// Without this bound any discoverable activity — a note, a task, the
// workspace's own reply — could be given durable state plus an audit row and a
// public event, and the rows would sit in two tables no read ever consults.
func TestOnlyAnInboundMessageCarriesADisposition(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, "thread-outbound-probe", "inbound", "A real wait",
		waitingInstant.Add(-2*24*time.Hour), person)
	inbound := waitingMessageID(t, e, "A real wait")

	// The admitting case first: a lane that refused everything would pass the
	// refusals below while making the feature dead.
	store := activities.NewStore(e.DB())
	if err := store.SetThreadNotSales(e.As(e.Rep1, nil, AdminPerms), inbound); err != nil {
		t.Fatalf("an inbound message was refused a disposition: %v", err)
	}

	// The workspace's own reply on the same thread is not something anybody
	// waits on, and judging it would judge the thread from the wrong end.
	seedWaitingMessageLinked(t, e, "thread-outbound-probe", "outbound", "Our answer",
		waitingInstant.Add(-1*24*time.Hour), person)
	var outbound ids.ActivityID
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM activity WHERE subject = 'Our answer'`).Scan(&outbound); err != nil {
		t.Fatalf("finding the outbound message: %v", err)
	}
	if err := store.SetThreadNotSales(e.As(e.Rep1, nil, AdminPerms), outbound); err == nil {
		t.Fatal("the workspace's own outbound reply was accepted as a disposition target")
	}
}
