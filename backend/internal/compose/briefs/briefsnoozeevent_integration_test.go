// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// A snooze that waits on the world rather than on the clock.
//
// "Remind me in three days" was the only thing a rep could say, so every
// set-aside was a guess: too early and the item is still dead, too late and the
// customer waited. These tests hold the two conditions that replace the guess —
// the customer writing back, and a meeting being over — through all three
// readers that must agree about whether a snooze is finished: the home read
// that flips the state, the fresh run that suppresses the deal, and the mark
// guard that decides whether a rep may still act.
//
// All instants are injected; nothing here reads the wall clock.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestASnoozeWaitingForAReplyLiftsWhenOneArrives(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	setDown := briefClock.Add(time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnReply, nil, nil, setDown); err != nil {
		t.Fatal(err)
	}

	// No reply yet: hidden from the read, suppressed from a fresh run, and no
	// passage of time changes either — which is the whole point of the
	// condition.
	muchLater := setDown.Add(6 * time.Hour)
	if visible := readableDeals(t, b, muchLater); visible[item.DealID] {
		t.Fatal("an item waiting for a reply re-surfaced with no reply; a condition that lifts on time alone is the guess this replaces")
	}
	ranked, err := b.engine.Rank(b.repCtx, muchLater)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range ranked.Queue {
		if candidate.DealID == item.DealID {
			t.Fatal("a fresh run ranked a deal whose item is still waiting for a reply")
		}
	}

	// The reply arrives, linked to the same deal and after the set-down.
	replyAt := setDown.Add(time.Hour)
	reply := seedInbound(t, owner, "they wrote back", replyAt)
	integration.LinkActivity(t, owner, reply, "deal", b.dealA)

	if visible := readableDeals(t, b, replyAt.Add(time.Hour)); !visible[item.DealID] {
		t.Fatal("the reply arrived and the item stayed set aside; the rep is now the one keeping the customer waiting")
	}
}

// TestAnInboundBeforeTheSnoozeIsNotTheReplyBeingWaitedFor is the case the
// bound on occurred_at exists for. Without it the snooze lifts on mail that was
// already sitting there when the rep set the item down — which is to say it
// never takes at all.
func TestAnInboundBeforeTheSnoozeIsNotTheReplyBeingWaitedFor(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	// A real inbound on this deal, stamped BEFORE the rep sets the item down.
	// Seeded here rather than relying on setupBrief's own linked activity,
	// which carries no direction at all and would be excluded by the inbound
	// clause no matter what the time bound said — a fixture that cannot fail
	// the assertion it is named for.
	setDown := briefClock.Add(3 * time.Hour)
	old := seedInbound(t, owner, "last week's mail", setDown.Add(-2*time.Hour))
	integration.LinkActivity(t, owner, old, "deal", b.dealA)

	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnReply, nil, nil, setDown); err != nil {
		t.Fatal(err)
	}
	if visible := readableDeals(t, b, setDown.Add(2*time.Hour)); visible[item.DealID] {
		t.Fatal("mail that predates the snooze lifted it, so the snooze never took")
	}
}

// TestAFutureDatedReplyDoesNotLiftASnoozeYet bounds the other end. A reply
// stamped for next week has not arrived, and treating it as one puts the item
// back for something still to come.
func TestAFutureDatedReplyDoesNotLiftASnoozeYet(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	setDown := briefClock.Add(time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnReply, nil, nil, setDown); err != nil {
		t.Fatal(err)
	}
	ahead := seedInbound(t, owner, "dated next week", setDown.AddDate(0, 0, 7))
	integration.LinkActivity(t, owner, ahead, "deal", b.dealA)

	if visible := readableDeals(t, b, setDown.Add(4*time.Hour)); visible[item.DealID] {
		t.Fatal("a reply dated next week lifted the snooze today")
	}
}

// TestOurOwnOutboundIsNotTheCustomerReplying is the direction guard. A rep who
// sets an item down and then sends a chaser has not been replied to, and
// lifting on their own mail would put the work back the moment they did it.
func TestOurOwnOutboundIsNotTheCustomerReplying(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	setDown := briefClock.Add(time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnReply, nil, nil, setDown); err != nil {
		t.Fatal(err)
	}
	chasedAt := setDown.Add(time.Hour)
	ours := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, direction)
		VALUES ($1, 'email', 'just chasing', $2, 'manual', 'human:x', 'outbound')`, chasedAt)
	integration.LinkActivity(t, owner, ours, "deal", b.dealA)

	if visible := readableDeals(t, b, chasedAt.Add(time.Hour)); visible[item.DealID] {
		t.Fatal("our own chaser lifted the snooze; the rep is waiting on the customer, not on themselves")
	}
}

func TestASnoozeWaitingForAMeetingLiftsOnceItIsOver(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	setDown := briefClock.Add(time.Hour)
	meetsAt := setDown.Add(4 * time.Hour)
	meeting := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'the demo', $2, 'manual', 'human:x')`, meetsAt)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnMeeting, nil, &meeting, setDown); err != nil {
		t.Fatal(err)
	}

	if visible := readableDeals(t, b, meetsAt.Add(-time.Hour)); visible[item.DealID] {
		t.Fatal("the item came back before the meeting it was waiting for")
	}
	if visible := readableDeals(t, b, meetsAt.Add(time.Hour)); !visible[item.DealID] {
		t.Fatal("the meeting is over and the item stayed set aside")
	}
}

// TestACancelledMeetingReturnsTheWorkRatherThanHoldingItForever is the case
// that decides between two defensible rules. A rep waiting on a meeting that is
// then cancelled is waiting for something that will never happen, and the only
// outcome that actually loses work is holding the item forever.
func TestACancelledMeetingReturnsTheWorkRatherThanHoldingItForever(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	meeting := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'the demo', $2, 'manual', 'human:x')`, briefClock.AddDate(0, 0, 16))
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnMeeting, nil, &meeting, briefClock.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Still far ahead, so nothing but the cancellation can bring this back.
	stillWaiting := briefClock.Add(5 * time.Hour)
	if visible := readableDeals(t, b, stillWaiting); visible[item.DealID] {
		t.Fatal("an item waiting for a future meeting came back early")
	}
	if _, err := owner.Exec(context.Background(),
		`UPDATE activity SET archived_at = now() WHERE id = $1`, meeting); err != nil {
		t.Fatal(err)
	}
	if visible := readableDeals(t, b, stillWaiting.Add(time.Hour)); !visible[item.DealID] {
		t.Fatal("the meeting was cancelled and the work stayed buried; nothing will ever bring it back")
	}
}

// TestTheMarkGuardAgreesWithTheReadAboutAnEventSnooze is the third reader.
// A rep looking at a screen the read has not refreshed must not be told their
// item is a conflict when the read would already have re-surfaced it — and must
// not be allowed to mark one the read still hides.
func TestTheMarkGuardAgreesWithTheReadAboutAnEventSnooze(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	setDown := briefClock.Add(time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnReply, nil, nil, setDown); err != nil {
		t.Fatal(err)
	}
	// No reply: the guard refuses, exactly as the read hides it.
	if _, err := b.engine.MarkActed(b.repCtx, item.ID, setDown.Add(time.Hour)); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("marking an item still waiting for a reply → %v, want ErrConflict", err)
	}
	replyAt := setDown.Add(time.Hour)
	reply := seedInbound(t, owner, "they wrote back", replyAt)
	integration.LinkActivity(t, owner, reply, "deal", b.dealA)

	// The reply arrived: the guard admits the mark without the read having run
	// first, which is the stale-screen case it exists for.
	if _, err := b.engine.MarkActed(b.repCtx, item.ID, replyAt.Add(time.Hour)); err != nil {
		t.Fatalf("marking an item whose reply arrived: %v", err)
	}
}

// TestASnoozeShapeIsRefusedBeforeItReachesTheDatabase holds the three ways a
// caller can name a wait that means nothing. Each must answer as a validation
// error the client can read, not as a constraint violation it cannot.
func TestASnoozeShapeIsRefusedBeforeItReachesTheDatabase(t *testing.T) {
	b := setupBrief(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	at := briefClock.Add(time.Hour)
	moment := briefClock.Add(8 * time.Hour)

	for _, bad := range []struct {
		name string
		on   values.ReopenCondition
		till *time.Time
		ref  *ids.UUID
	}{
		{"a clock snooze with no moment", values.ReopenOnTime, nil, nil},
		{"a reply snooze carrying a moment", values.ReopenOnReply, &moment, nil},
		{"a meeting snooze naming no meeting", values.ReopenOnMeeting, nil, nil},
		{"a reply snooze naming a meeting", values.ReopenOnReply, nil, &item.DealID},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, bad.on, bad.till, bad.ref, at); err == nil {
				t.Fatal("accepted, and the row it would write describes a wait nothing can lift")
			}
		})
	}
}

// seedInbound writes one inbound message at a given instant. Every instant in
// this file is derived from the moment the item was set down rather than
// written out, so a change to briefClock cannot silently move a reply to the
// wrong side of the snooze and turn a real assertion into a vacuous one.
func seedInbound(t *testing.T, owner *pgx.Conn, subject string, at time.Time) ids.UUID {
	t.Helper()
	return integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, direction)
		VALUES ($1, 'email', $2, $3, 'manual', 'human:x', 'inbound')`, subject, at)
}

// TestASnoozeCannotWaitForSomethingThatIsNotAMeeting holds the guard on
// reopen_ref. Three ways to name the wrong thing, and each loses work or leaks:
// an id naming nothing never lifts, an id naming an email lifts instantly
// because its occurred_at is already past, and an id naming a row the rep
// cannot read would tell them when somebody else's meeting happened.
func TestASnoozeCannotWaitForSomethingThatIsNotAMeeting(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	at := briefClock.Add(time.Hour)

	email := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, direction)
		VALUES ($1, 'email', 'not a meeting', $2, 'manual', 'human:x', 'inbound')`, briefClock.Add(-24*time.Hour))
	missing := ids.NewV7()

	for _, bad := range []struct {
		name string
		ref  ids.UUID
	}{
		{"an id that names no row at all", missing},
		{"an id that names an email rather than a meeting", email},
	} {
		t.Run(bad.name, func(t *testing.T) {
			ref := bad.ref
			if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnMeeting, nil, &ref, at); err == nil {
				t.Fatal("accepted; the snooze it writes either never lifts or lifts on the wrong row")
			}
		})
	}
}

// TestAMeetingSnoozeHoldsUntilTheMeetingHasENDED is the difference between the
// meeting's start and its end. Lifting at occurred_at puts the work back while
// the rep is still in the room, which is the one moment they cannot act on it.
func TestAMeetingSnoozeHoldsUntilTheMeetingHasENDED(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	setDown := briefClock.Add(time.Hour)
	startsAt := setDown.Add(2 * time.Hour)
	meeting := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, source, captured_by)
		VALUES ($1, 'meeting', 'the demo', $2, 3600, 'manual', 'human:x')`, startsAt)
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnMeeting, nil, &meeting, setDown); err != nil {
		t.Fatal(err)
	}
	// Thirty minutes in: the meeting has started and is not over.
	if visible := readableDeals(t, b, startsAt.Add(30*time.Minute)); visible[item.DealID] {
		t.Fatal("the work came back while the rep was still in the meeting")
	}
	if visible := readableDeals(t, b, startsAt.Add(90*time.Minute)); !visible[item.DealID] {
		t.Fatal("the meeting ended and the work stayed set aside")
	}
}

// TestACancelledMeetingStatusReturnsTheWork covers the other way a meeting
// stops being something worth waiting for. Archiving is one; the record's own
// status is the one a calendar sync actually writes.
func TestACancelledMeetingStatusReturnsTheWork(t *testing.T) {
	b := setupBrief(t)
	owner := integration.OwnerConn(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	item := run.Items[0]
	meeting := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'the demo', $2, 'booked', 'manual', 'human:x')`, briefClock.AddDate(0, 0, 16))
	if _, err := b.engine.MarkSnoozed(b.repCtx, item.ID, values.ReopenOnMeeting, nil, &meeting, briefClock.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	stillWaiting := briefClock.Add(5 * time.Hour)
	if visible := readableDeals(t, b, stillWaiting); visible[item.DealID] {
		t.Fatal("an item waiting for a booked future meeting came back early")
	}
	if _, err := owner.Exec(context.Background(),
		`UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`, meeting); err != nil {
		t.Fatal(err)
	}
	if visible := readableDeals(t, b, stillWaiting.Add(time.Hour)); !visible[item.DealID] {
		t.Fatal("the meeting was cancelled and the work stayed buried")
	}
}

// readableDeals is what the rep's own home read currently shows them, as a set
// of deal ids. The read is the surface the whole feature is judged by, so the
// tests above ask it rather than querying brief_item directly.
func readableDeals(t *testing.T, b *briefEnv, at time.Time) map[ids.UUID]bool {
	t.Helper()
	latest, err := b.engine.LatestRun(b.repCtx, at)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ids.UUID]bool{}
	for _, item := range latest.Items {
		seen[item.DealID] = true
	}
	return seen
}
