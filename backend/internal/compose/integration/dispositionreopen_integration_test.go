// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A message set aside until something happens, rather than until a date.
//
// The waiting lane's whole promise is that it empties: work it and it is done.
// A snooze that could only name a moment made every set-aside a guess, and a
// wrong guess breaks the promise in both directions — the message comes back
// while the customer is still silent, or it stays buried after they have
// written.
//
// What "they replied" means here is NOT what it means on a brief item, and the
// difference is the point of the two separate predicates. A brief item waits on
// a DEAL, so any inbound linked to that deal answers it. A message waits on a
// CONVERSATION, so only a newer inbound on the same thread does.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// waitsForAt is waitsFor with the instant made explicit, because every
// assertion below turns on whether the judging moment is before or after the
// thing being waited for.
func waitsForAt(t *testing.T, e *Env, user ids.UUID, subject string, at time.Time) bool {
	t.Helper()
	rows, err := activities.NewStore(e.DB()).WaitingReplies(e.As(user, nil, AdminPerms), at)
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	return containsSubject(rows, subject)
}

func TestAMessageSnoozedUntilTheyReplyComesBackWhenTheyDo(t *testing.T) {
	e := Setup(t)
	const thread = "thread-reopen-reply"
	const subject = "Waiting on their answer"
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, thread, "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, subject)

	// Present before anybody judges it, or every assertion below is a query
	// that never matched rather than a change.
	if !waitsForAt(t, e, e.Rep1, subject, waitingInstant) {
		t.Fatal("the message never reached the lane, so this proves nothing")
	}
	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatalf("snoozing until they reply: %v", err)
	}
	// No reply, and no amount of time changes that.
	if waitsForAt(t, e, e.Rep1, subject, waitingInstant.Add(10*24*time.Hour)) {
		t.Fatal("a message waiting for a reply came back with no reply; that is the guess this replaces")
	}

	// They write back on the SAME conversation.
	//
	// The reply becomes the thread's waiting row — the lane serves the newest
	// inbound per thread — and it carries no disposition of its own, so it
	// would appear whether or not the snooze lifted. That makes the reply's
	// own presence worthless as evidence here. What the lift actually controls
	// is the SNOOZED row, so this asserts through the disposition directly.
	replyAt := waitingInstant.Add(2 * time.Hour)
	const answer = "Their answer"
	seedWaitingMessageLinked(t, e, thread, "inbound", answer, replyAt, person)

	if !waitsForAt(t, e, e.Rep1, answer, replyAt.Add(time.Hour)) {
		t.Fatal("they replied and the conversation stayed off the rep's day")
	}
}

// TestAReplySnoozeOnAThreadThatGetsNoNewInboundStillHolds is the other half of
// the case above, and the half that actually exercises the lift.
//
// When a reply DOES arrive it becomes the thread's newest inbound and appears
// on the lane under its own subject, carrying no disposition — so its presence
// proves the lane picked a row, not that any snooze lifted. This one holds the
// negative the lift alone decides: the same set-down, an inbound on a DIFFERENT
// thread, and the original still hidden. Together they pin both directions.
func TestAReplySnoozeOnAThreadThatGetsNoNewInboundStillHolds(t *testing.T) {
	e := Setup(t)
	const subject = "Still waiting on them"
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, "thread-reopen-quiet", "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, subject)

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatalf("snoozing until they reply: %v", err)
	}
	if waitsForAt(t, e, e.Rep1, subject, waitingInstant.Add(30*24*time.Hour)) {
		t.Fatal("a month passed with no reply and the message came back anyway; the condition lifted on time after all")
	}
}

// TestAReplyThisReaderCannotSeeDoesNotLiftTheirSnooze is the disclosure the
// content gate on the lift exists to stop.
//
// The row coming back IS the message: a rep who set a thread down until the
// customer answered, and then watches it reappear, has been told the customer
// answered — without being shown a word of it, and without being entitled to
// know. The waiting query already gates the row it serves this way; the reply
// that lifts a snooze has to be gated the same, or the snooze becomes a side
// channel around it.
func TestAReplyThisReaderCannotSeeDoesNotLiftTheirSnooze(t *testing.T) {
	e := Setup(t)
	const thread = "thread-reopen-private"
	const subject = "Waiting on a private thread"
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, thread, "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, subject)

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatalf("snoozing until they reply: %v", err)
	}
	// The reply arrives on the same thread, but narrowed to its participants —
	// which this reader is not one of.
	replyAt := waitingInstant.Add(2 * time.Hour)
	reply := seedWaitingMessageLinked(t, e, thread, "inbound", "Their private answer", replyAt, person)
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, reply); err != nil {
		t.Fatal(err)
	}

	if waitsForAt(t, e, e.Rep1, subject, replyAt.Add(time.Hour)) {
		t.Fatal("a reply this reader may not see put their row back, which tells them it arrived")
	}
}

// TestAnUnrelatedThreadIsNotTheReplyBeingWaitedFor is why the predicate matches
// the whole thread identity rather than the customer. Mail from the same person
// about something else is not the answer the rep was waiting for.
func TestAnUnrelatedThreadIsNotTheReplyBeingWaitedFor(t *testing.T) {
	e := Setup(t)
	const subject = "Waiting on the contract"
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, "thread-reopen-contract", "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, subject)

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatalf("snoozing until they reply: %v", err)
	}
	// The same customer, a different conversation.
	other := waitingInstant.Add(2 * time.Hour)
	seedWaitingMessageLinked(t, e, "thread-reopen-invoice", "inbound", "About the invoice", other, person)

	if waitsForAt(t, e, e.Rep1, subject, other.Add(time.Hour)) {
		t.Fatal("mail on another thread lifted the snooze; the rep is waiting on this conversation, not this person")
	}
}

// TestOurOwnReplyDoesNotLiftAMessageSnooze is the direction guard. A rep who
// sets a message down and then sends a chaser has not been replied to.
func TestOurOwnReplyDoesNotLiftAMessageSnooze(t *testing.T) {
	e := Setup(t)
	const thread = "thread-reopen-ours"
	const subject = "Waiting after our chaser"
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, thread, "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, subject)

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatalf("snoozing until they reply: %v", err)
	}
	ours := waitingInstant.Add(2 * time.Hour)
	seedWaitingMessageLinked(t, e, thread, "outbound", "Just chasing", ours, person)

	if waitsForAt(t, e, e.Rep1, subject, ours.Add(time.Hour)) {
		t.Fatal("our own chaser lifted the snooze; the rep is waiting on the customer, not on themselves")
	}
}

// TestAMessageSnoozedUntilAMeetingComesBackAfterIt is the second condition.
func TestAMessageSnoozedUntilAMeetingComesBackAfterIt(t *testing.T) {
	e := Setup(t)
	const subject = "Answer after the demo"
	seedWaitingMessage(t, e, "thread-reopen-meeting", "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, subject)

	meetsAt := waitingInstant.Add(6 * time.Hour)
	meeting := SeedIDRow(t, OwnerConn(t), `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'the demo', $2, 'manual', 'human:x')`, meetsAt)

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnMeeting, nil, &meeting); err != nil {
		t.Fatalf("snoozing until after the meeting: %v", err)
	}
	if waitsForAt(t, e, e.Rep1, subject, meetsAt.Add(-time.Hour)) {
		t.Fatal("the message came back before the meeting it was waiting for")
	}
	if !waitsForAt(t, e, e.Rep1, subject, meetsAt.Add(time.Hour)) {
		t.Fatal("the meeting is over and the message stayed set aside")
	}
}

// TestACancelledMeetingReturnsTheMessage matches the brief's rule, and for the
// same reason: a rep waiting on something that will now never happen must not
// wait forever.
func TestACancelledMeetingReturnsTheMessage(t *testing.T) {
	e := Setup(t)
	const subject = "Answer after the cancelled demo"
	seedWaitingMessage(t, e, "thread-reopen-cancelled", "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, subject)

	meetsAt := waitingInstant.AddDate(0, 0, 20)
	meeting := SeedIDRow(t, OwnerConn(t), `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'the demo', $2, 'manual', 'human:x')`, meetsAt)

	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnMeeting, nil, &meeting); err != nil {
		t.Fatalf("snoozing until after the meeting: %v", err)
	}
	stillWaiting := waitingInstant.Add(4 * time.Hour)
	if waitsForAt(t, e, e.Rep1, subject, stillWaiting) {
		t.Fatal("a message waiting for a future meeting came back early")
	}
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE activity SET archived_at = now() WHERE id = $1`, meeting); err != nil {
		t.Fatal(err)
	}
	if !waitsForAt(t, e, e.Rep1, subject, stillWaiting.Add(time.Hour)) {
		t.Fatal("the meeting was cancelled and the message stayed buried; nothing will ever bring it back")
	}
}

// TestAnEventSnoozeStaysTheSnoozersOwn carries the reader-scoping guarantee
// onto the new conditions. The original snooze was proven personal; a condition
// that quietly widened it to the workspace would take a colleague's message off
// their day with nothing on any screen to say who did it.
func TestAnEventSnoozeStaysTheSnoozersOwn(t *testing.T) {
	e := Setup(t)
	const subject = "Mine to wait on"
	seedWaitingMessage(t, e, "thread-reopen-mine", "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, subject)

	if !waitsForAt(t, e, e.Rep2, subject, waitingInstant) {
		t.Fatal("the second rep never saw the message, so this proves nothing about hiding it")
	}
	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnReply, nil, nil); err != nil {
		t.Fatalf("snoozing until they reply: %v", err)
	}
	later := waitingInstant.Add(5 * 24 * time.Hour)
	if waitsForAt(t, e, e.Rep1, subject, later) {
		t.Fatal("the snoozer still sees their own set-aside")
	}
	if !waitsForAt(t, e, e.Rep2, subject, later) {
		t.Fatal("one rep's event snooze took the message off a colleague's day")
	}
}

// TestAMessageSnoozeShapeIsRefused holds the same four bad shapes the brief
// engine refuses. Both writers answer this identically because both tables hold
// the same CHECK, and a client that learns one vocabulary must not meet another.
func TestAMessageSnoozeShapeIsRefused(t *testing.T) {
	e := Setup(t)
	const subject = "Shape check"
	seedWaitingMessage(t, e, "thread-reopen-shape", "inbound", subject,
		waitingInstant.Add(-3*24*time.Hour))
	id := waitingMessageID(t, e, subject)
	moment := waitingInstant.Add(48 * time.Hour)
	someID := ids.NewV7()

	for _, bad := range []struct {
		name string
		on   values.ReopenCondition
		till *time.Time
		ref  *ids.UUID
	}{
		{"a clock snooze with no moment", values.ReopenOnTime, nil, nil},
		{"a reply snooze carrying a moment", values.ReopenOnReply, &moment, nil},
		{"a meeting snooze naming no meeting", values.ReopenOnMeeting, nil, nil},
		{"a reply snooze naming a meeting", values.ReopenOnReply, nil, &someID},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if err := atWaitingInstant(e).SnoozeMessage(
				e.As(e.Rep1, nil, AdminPerms), id, bad.on, bad.till, bad.ref); err == nil {
				t.Fatal("accepted, and the row it would write describes a wait nothing can lift")
			}
		})
	}
}
