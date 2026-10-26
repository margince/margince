// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Done on a waiting message: the reader dealt with it another way.
//
// It is one reader's own state, like a snooze. These tests write it through
// MarkMessageHandled and read it back through the waiting lane, the hidden
// backlog and the response figures. Each embeds the shared set-aside predicate,
// and each would show a missed state differently.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func handledStore(e *loadEnv) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
}

// onListNow reports whether a message is on one reader's waiting list now.
// Wall time, because seedWait dates its rows with the database's now().
func onListNow(reader context.Context, t *testing.T, e *loadEnv, message ids.UUID) bool {
	t.Helper()
	return e.waitingAt(reader, t, time.Now())[message]
}

// handledFixture seeds a contact the rep owns and one message waiting on it.
func handledFixture(t *testing.T, e *loadEnv, subject string) (contact, message ids.UUID) {
	t.Helper()
	contact = ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, contact, e.rep)
	return contact, e.seedPersonalWait(t, subject, contact)
}

func TestMarkingAMessageDoneTakesItOffItsReadersListOnly(t *testing.T) {
	e := setupLoad(t)
	_, message := handledFixture(t, e, "Answered on the phone")
	rep, colleague := e.asSeat(e.rep), e.asSeat(e.other)
	if !onListNow(rep, t, e, message) || !onListNow(colleague, t, e, message) {
		t.Fatal("the message is not on both lists before anybody marks it, so this proves nothing")
	}
	before := hiddenNow(t, e)

	if err := handledStore(e).MarkMessageHandled(rep, ids.From[ids.ActivityKind](message)); err != nil {
		t.Fatalf("marking the message done: %v", err)
	}

	if onListNow(rep, t, e, message) {
		t.Error("the message is still on the list of the rep who marked it done")
	}
	if !onListNow(colleague, t, e, message) {
		t.Error("one rep's Done took the message off a colleague's list")
	}
	if got := moved(before, hiddenNow(t, e)); got.SetAside != 1 || got.Shown != -1 {
		t.Errorf("the hidden backlog moved %+v; Done should move one row from shown to set aside", got)
	}
	if audited, announced := handledTrail(t, e, message); audited != 1 || announced != 1 {
		t.Errorf("Done left %d audit rows and %d activity.marked_handled events, want one of each", audited, announced)
	}

	if err := handledStore(e).ClearMessageDisposition(rep, ids.From[ids.ActivityKind](message)); err != nil {
		t.Fatalf("undoing Done: %v", err)
	}
	if !onListNow(rep, t, e, message) {
		t.Error("undoing Done did not put the message back on the rep's list")
	}
}

// The customer's next message is new work. Nothing has to wake the old state:
// the new message is its own row and nobody has marked it.
func TestADoneMessageComesBackWhenTheCustomerWritesAgain(t *testing.T) {
	e := setupLoad(t)
	contact, message := handledFixture(t, e, "Handled in the meeting")
	rep := e.asSeat(e.rep)
	if err := handledStore(e).MarkMessageHandled(rep, ids.From[ids.ActivityKind](message)); err != nil {
		t.Fatalf("marking the message done: %v", err)
	}

	next := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'One more question', now() - interval '1 hour', $2, 'seed', 'system')`,
		next, "thread-"+message.String())
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', 'buyer@customer.test')`, ids.NewV7(), next)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), next, contact)
	e.directlyTo(t, next, e.rep)

	if !onListNow(rep, t, e, next) {
		t.Error("the customer wrote again on a thread the rep marked done, and the new message is not on their list")
	}
}

// Done is a disposition, so it counts in Disposed. It is never a reply, so the
// answered count and the median do not move.
func TestMarkingDoneCountsAsPutDownAndNotAsAReply(t *testing.T) {
	e := setupLoad(t)
	_, message := handledFixture(t, e, "Settled another way")
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if err := handledStore(e).MarkMessageHandled(e.asSeat(e.rep), ids.From[ids.ActivityKind](message)); err != nil {
		t.Fatalf("marking the message done: %v", err)
	}
	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if after.Disposed != before.Disposed+1 {
		t.Errorf("Disposed moved %d → %d over one Done", before.Disposed, after.Disposed)
	}
	if after.Answered != before.Answered || after.MedianMinutes != before.MedianMinutes {
		t.Errorf("Done moved the reply figures: answered %d → %d, median %d → %d",
			before.Answered, after.Answered, before.MedianMinutes, after.MedianMinutes)
	}
}

// The table's shape check: Done waits on nothing, so a reopen condition on it
// is refused.
func TestADoneStateCarriesNoReopenCondition(t *testing.T) {
	e := setupLoad(t)
	_, message := handledFixture(t, e, "Shape check")
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_reader_state (activity_id, reader_id, state, reopen_on, set_by)
		 VALUES ($1, $2, 'handled', 'reply', 'system')`, message, e.rep); err == nil {
		t.Error("a handled state waiting on a reply was stored")
	}
	e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, set_by)
		VALUES ($1, $2, 'handled', 'system')`, message, e.rep)
}

// handledTrail counts the audit rows naming Done on one message, and the
// activity.marked_handled events announcing it.
func handledTrail(t *testing.T, e *loadEnv, message ids.UUID) (audited, announced int) {
	t.Helper()
	ctx := context.Background()
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM audit_log
		 WHERE entity_type = 'activity' AND entity_id = $1 AND after->>'disposition' = $2`,
		message, stateHandled).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'activity.marked_handled'
		   AND envelope->'entity'->>'id' = $1::text`,
		message.String()).Scan(&announced); err != nil {
		t.Fatal(err)
	}
	return audited, announced
}

// Done on the card handles its earlier requests too, so a snooze one of them
// carried is replaced, whatever it waited on. A lifted reply or meeting snooze
// would otherwise bring that request back the moment the card is done.
func TestDoneOnTheCardReplacesAnEarlierRequestsSnooze(t *testing.T) {
	for _, reopen := range []string{"reply", "meeting"} {
		t.Run(reopen, func(t *testing.T) {
			e := setupLoad(t)
			asks := requestsInOneConversation(t, e)
			rep, store := e.asSeat(e.rep), storeKnowing(e)
			at := requestInstant.Add(4 * time.Hour)
			var ref *ids.UUID
			if reopen == "meeting" {
				meeting := ids.NewV7()
				e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
					VALUES ($1, 'meeting', 'Call about slots', $2, 'seed', 'system')`,
					meeting, requestInstant.Add(time.Hour))
				ref = &meeting
			}
			e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, reopen_on, reopen_ref, set_by, set_at)
				VALUES ($1, $2, 'snoozed', $3, $4, 'human:test', $5)`,
				asks[0], e.rep, reopen, ref, requestInstant)
			if !e.waitingAt(rep, t, at)[asks[0]] {
				t.Fatal("the earlier request's snooze has not lifted, so this proves nothing")
			}

			if err := store.MarkMessageHandled(rep, ids.From[ids.ActivityKind](asks[2])); err != nil {
				t.Fatalf("marking the card done: %v", err)
			}

			if e.waitingAt(rep, t, at)[asks[0]] {
				t.Errorf("an earlier request whose %s snooze had lifted stayed on the list after Done on its card", reopen)
			}
		})
	}
}

// Done is about a customer's message. A meeting or our own outbound mail is
// refused with the same faults threadOf gives, and nothing is stored.
func TestDoneIsRefusedOnAnythingButAnInboundMessage(t *testing.T) {
	e := setupLoad(t)
	meeting, outbound := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kick-off', now() - interval '1 day', 'seed', 'system')`, meeting)
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Our quote', now() - interval '1 day', $2, 'seed', 'system')`,
		outbound, "thread-"+outbound.String())
	for activity, want := range map[ids.UUID]string{meeting: "not_a_message", outbound: "not_inbound"} {
		err := handledStore(e).MarkMessageHandled(e.asSeat(e.rep), ids.From[ids.ActivityKind](activity))
		var refused *values.ParseError
		if !errors.As(err, &refused) || refused.Code != want {
			t.Errorf("Done on %v answered %v, want the %s refusal", activity, err, want)
		}
		var stored int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM activity_reader_state WHERE activity_id = $1`, activity).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != 0 {
			t.Errorf("a refused Done still stored %d state rows", stored)
		}
	}
}

// Disposed counts only a customer's inbound message, whatever audit rows other
// activities carry.
func TestDisposedCountsOnlyInboundMessages(t *testing.T) {
	e := setupLoad(t)
	meeting := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kick-off', now() - interval '1 day', 'seed', 'system')`, meeting)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ('human', 'human:seed', 'update', 'activity', $1, '{"disposition":"not_mine"}'::jsonb)`, meeting)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if after.Disposed != before.Disposed {
		t.Errorf("a judgement on a meeting moved Disposed %d → %d", before.Disposed, after.Disposed)
	}
}
