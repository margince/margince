// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A customer mail with no thread key, against a real database.
//
// The unit lane can only read the query as a string: it proves a clause is
// spelled, never that Postgres answers with the row. That gap is where this
// defect lived — every string-level gate over the waiting query passed while
// the queue returned nothing for a client asking for an appointment.
//
// A thread key is absent whenever nothing stamped one: mail imported without
// its headers, a channel row captured before identity ran, a seeded record. It
// says nothing about whether anybody answered.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// threadless seeds one inbound customer mail with NO thread key, NO capture
// label and NO verdict — the shape no classifier pass has read.
//
// It mirrors waitingFrom deliberately: the two differ in the thread key alone,
// which is what makes a disagreement between them attributable to that column.
func (e *loadEnv) threadless(t *testing.T, subject, address string, contact ids.UUID) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now() - interval '2 days', 'seed', 'system')`,
		activity, subject)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', $3)`, ids.NewV7(), activity, address)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activity, contact)
	return activity
}

// The regression, stated as the queue's own answer.
//
// The exclusion this replaces demanded that a threadless row already carry a
// capture label. The label comes from a classifier pass that reads ITS BACKLOG
// FROM THIS QUERY, so the row was never judged, never labelled, and excluded
// again on every later pass. Nothing failed and nothing was logged; the queue
// simply reported that nobody was waiting.
func TestAThreadlessCustomerMailIsWaiting(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	activity := e.threadless(t, "Dienstag 14 Uhr würde bei uns passen", "buyer@customer.test", contact)

	// waitFor fails the test when the row is absent, which is the assertion.
	if got := e.waitFor(t, activity); got.ActivityID != activity {
		t.Fatalf("the queue returned %v for the threadless mail, want %v", got.ActivityID, activity)
	}
}

// One threadless outbound must not settle another threadless question.
//
// This is the failure the old exclusion was guarding against, and it is real:
// NULL-matching thread keys would join every unthreaded row to every other, so
// a single unanswered outbound would empty the queue of every unthreaded
// question in the workspace. Admitting the rows is only correct while the
// reply anti-joins still refuse to match two unknown keys.
func TestAThreadlessReplyDoesNotSettleAnotherThreadlessQuestion(t *testing.T) {
	e := setupLoad(t)
	first := e.buyer(t)
	second := e.buyer(t)
	asking := e.threadless(t, "Can you send the revised plan?", "first@customer.test", first)
	alsoAsking := e.threadless(t, "When does phase 3 start?", "second@customer.test", second)

	// A later outbound with no thread key either. Under IS NOT DISTINCT FROM
	// it would match both questions and silence them at once.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Unrelated note', now() - interval '1 hour', 'seed', 'system')`,
		ids.NewV7())

	if got := e.waitFor(t, asking); got.ActivityID != asking {
		t.Error("the first threadless question was settled by an unrelated threadless mail")
	}
	if got := e.waitFor(t, alsoAsking); got.ActivityID != alsoAsking {
		t.Error("the second threadless question was settled by an unrelated threadless mail")
	}
}

// A threadless machine sender is still not waiting.
//
// Admitting threadless rows must not admit the notification traffic the queue
// has always excluded, and the machine rule sits above the scan cap precisely
// so a flood of it cannot displace a customer. Held here because the widening
// is what makes this reachable: before it, the row never got this far.
func TestAThreadlessMachineSenderIsNotWaiting(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	noise := e.threadless(t, "Your build has finished", "noreply@ci.example", contact)

	rows, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).
		WaitingReplies(e.as(), time.Now())
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == noise {
			t.Fatal("a threadless noreply notification became somebody's obligation")
		}
	}
}

// A threaded conversation still settles on its own reply.
//
// The change widens what counts as waiting, so the case it must not break is
// the ordinary one: a question answered inside its own thread is finished.
func TestAnAnsweredThreadStaysAnswered(t *testing.T) {
	e := setupLoad(t)
	contact := e.buyer(t)
	asked := e.waitingFrom(t, "Can you confirm the price?", "buyer@customer.test", contact)

	// waitingFrom derives the thread key from the activity id, so the reply
	// joins the same conversation without a read-back.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Re: Can you confirm the price?', now() - interval '1 hour', $2, 'seed', 'system')`,
		ids.NewV7(), "thread-"+asked.String())

	rows, err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).
		WaitingReplies(e.as(), time.Now())
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == asked {
			t.Fatal("a question answered in its own thread is still reported as waiting")
		}
	}
}
