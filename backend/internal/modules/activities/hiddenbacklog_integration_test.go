// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// What each hiding rule is keeping off the queue, against a real database.
//
// Every figure here is a DIFFERENCE between two runs of one SQL statement, so
// nothing in the unit lane can check it: a relaxation wired to the wrong hole,
// a predicate that widens the clause it was meant to leave alone, or a count
// taken before the cap all compile and simply report a different number.
//
// The direction that matters is UNDER-reporting. A guardrail that answers zero
// over a queue full of hidden work looks exactly like a healthy one, and there
// is no failing assertion to notice — so each case below hides one message and
// requires the figure for THAT rule to find it.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func hiddenStore(e *loadEnv) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
}

// hiddenNow reads the guardrail, failing the test rather than returning an error
// nobody checks.
func hiddenNow(t *testing.T, e *loadEnv) HiddenBacklog {
	t.Helper()
	got, err := hiddenStore(e).HiddenWaiting(e.as(), time.Now())
	if err != nil {
		t.Fatalf("reading the hidden backlog: %v", err)
	}
	return got
}

// moved is what THIS test's seeding changed, figure by figure.
//
// Absolute counts are the wrong assertion here and the file next door says why:
// the lane template is shared across this package's tests, so a message another
// test seeded is present too and a total would pass or fail on which tests ran.
// A difference is the test's own contribution whatever else is in the database.
func moved(before, after HiddenBacklog) HiddenBacklog {
	return HiddenBacklog{
		Shown:       after.Shown - before.Shown,
		SetAside:    after.SetAside - before.SetAside,
		NotSales:    after.NotSales - before.NotSales,
		PastHorizon: after.PastHorizon - before.PastHorizon,
		Unlinked:    after.Unlinked - before.Unlinked,
	}
}

// The baseline: a queue with nothing hidden reports nothing hidden.
//
// First because it is what the guardrail's target means. Every case below is a
// departure from this, and without it a figure of zero could equally mean "the
// read is broken" — which is the failure this whole file exists to catch.
func TestAQueueHidingNothingReportsAClearBacklog(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	e.seedWait(t, "Question about pricing", "person_id", person)

	got := moved(before, hiddenNow(t, e))

	if got.Shown != 1 {
		t.Fatalf("the queue gained %d waits, wanted the one seeded", got.Shown)
	}
	if !got.Clear() {
		t.Fatalf("a message nothing hides moved a hidden figure: %+v", got)
	}
}

// A reader's own set-aside is counted, and counted as theirs.
//
// `not_mine` carries no moment and does not lift at all, so a rep who marks a
// customer not-theirs removes them from that rep's day until they withdraw it.
// The rep's page cannot show that; this is what does.
func TestAMessageSetAsideIsCountedAsHidden(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	e.seedWait(t, "Still waiting", "person_id", person)
	aside := e.seedWait(t, "Set aside", "person_id", person)
	e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, set_by)
		VALUES ($1, $2, 'not_mine', 'system')`, aside, e.rep)

	got := moved(before, hiddenNow(t, e))

	if got.Shown != 1 {
		t.Fatalf("the queue gained %d waits, wanted only the unhidden one", got.Shown)
	}
	if got.SetAside != 1 {
		t.Fatalf("counted %d set aside, wanted the one marked not_mine (%+v)", got.SetAside, got)
	}
	// Attributed to the rule that actually hid it. A figure that moved under
	// three headings at once would tell a reader nothing about what to look at.
	if got.NotSales != 0 || got.PastHorizon != 0 || got.Unlinked != 0 {
		t.Fatalf("a set-aside was also counted under another rule: %+v", got)
	}
}

// A thread judged not-sales is counted, and it is the judgement worth watching:
// it hides the conversation from the WHOLE workspace and never lifts.
func TestAThreadJudgedNotSalesIsCountedAsHidden(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	e.seedWait(t, "Still waiting", "person_id", person)
	judged := e.seedWait(t, "Newsletter", "person_id", person)
	// Keyed on the THREAD, the way SetThreadNotSales writes it — an activity id
	// here would be judged by a rule that reads thread_key and match nothing.
	// The table has no state column: a ROW is the judgement, which is why the
	// query anti-joins on its existence rather than on a value.
	e.exec(t, `INSERT INTO activity_sales_state (thread_key, kind, channel_provider, set_by)
		SELECT a.thread_key, a.kind, coalesce(a.channel_provider, ''), 'system'
		  FROM activity a WHERE a.id = $1`, judged)

	got := moved(before, hiddenNow(t, e))

	if got.Shown != 1 {
		t.Fatalf("the queue gained %d waits, wanted the one unjudged", got.Shown)
	}
	if got.NotSales != 1 {
		t.Fatalf("counted %d judged not_sales, wanted the one (%+v)", got.NotSales, got)
	}
	if got.SetAside != 0 {
		t.Fatalf("a not_sales judgement was also counted as a set-aside: %+v", got)
	}
}

// NOBODY CHOSE THIS ONE, which is why it is the figure the guardrail exists for.
//
// A customer who wrote four months ago and was never answered falls off the
// queue on a date, with no rep having judged anything. The page reads as clear
// and the customer is still waiting.
func TestAWaitPastTheHorizonIsCountedAsHidden(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	e.seedWait(t, "Recent question", "person_id", person)
	old := e.seedWait(t, "Asked in the spring", "person_id", person)
	// Past waitingHorizonDays and inside hiddenHorizonDays, so it is work the
	// queue drops and the guardrail still reaches.
	e.exec(t, `UPDATE activity SET occurred_at = now() - interval '150 days' WHERE id = $1`, old)

	got := moved(before, hiddenNow(t, e))

	if got.Shown != 1 {
		t.Fatalf("the queue gained %d waits, wanted only the recent one", got.Shown)
	}
	if got.PastHorizon != 1 {
		t.Fatalf("counted %d past the horizon, wanted the 150-day-old wait (%+v)",
			got.PastHorizon, got)
	}
}

// The guardrail's own reach is bounded, and it says so by finding nothing rather
// than by pretending the work is not there.
//
// A message older than hiddenHorizonDays is outside what this reading claims to
// cover. Asserted so the bound is a decision with a test on it rather than a
// number somebody can move without noticing what it stops answering.
func TestAWaitPastTheGuardrailsOwnReachIsNotCounted(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	ancient := e.seedWait(t, "Two years ago", "person_id", person)
	e.exec(t, `UPDATE activity SET occurred_at = now() - interval '800 days' WHERE id = $1`, ancient)

	got := moved(before, hiddenNow(t, e))

	if got.PastHorizon != 0 {
		t.Fatalf("counted a wait older than the guardrail's own reach: %+v", got)
	}
}

// Inbound mail attached to no sales record. Also nobody's choice, and genuinely
// ambiguous: usually a rep's dentist, sometimes a real customer whose thread
// capture failed to link. Its own figure for exactly that reason.
func TestUnlinkedInboundIsCountedSeparately(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	e.seedWait(t, "A real customer", "person_id", person)
	// The same shape minus the sales link — every other rule still satisfied.
	loose := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Dentist appointment', now() - interval '2 days', $2, 'seed', 'system')`,
		loose, "thread-"+loose.String())
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', 'reception@dentist.test')`, ids.NewV7(), loose)

	got := moved(before, hiddenNow(t, e))

	if got.Shown != 1 {
		t.Fatalf("the queue gained %d waits, wanted only the linked one", got.Shown)
	}
	if got.Unlinked != 1 {
		t.Fatalf("counted %d unlinked, wanted the one (%+v)", got.Unlinked, got)
	}
	if got.PastHorizon != 0 {
		t.Fatalf("unlinked mail was also counted against the horizon: %+v", got)
	}
}

// The property every figure rests on: a relaxed read is a SUPERSET of the
// strict one, so no difference can be negative.
//
// HiddenWaiting floors at zero, which would silently turn a broken relaxation
// into a reassuring zero — the under-reporting direction again. This asserts the
// property directly instead of trusting the clamp.
func TestEveryRelaxationAdmitsAtLeastWhatTheQueueShows(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	before := hiddenNow(t, e)
	for _, subject := range []string{"One", "Two", "Three"} {
		e.seedWait(t, subject, "person_id", person)
	}
	// One message hidden by EACH rule, so every relaxation has something of its
	// own to find and a wiring that answers the wrong hole cannot pass by
	// finding somebody else's row.
	aside := e.seedWait(t, "Set aside", "person_id", person)
	e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, set_by)
		VALUES ($1, $2, 'not_mine', 'system')`, aside, e.rep)
	judged := e.seedWait(t, "Judged", "person_id", person)
	// The table has no state column: a ROW is the judgement, which is why the
	// query anti-joins on its existence rather than on a value.
	e.exec(t, `INSERT INTO activity_sales_state (thread_key, kind, channel_provider, set_by)
		SELECT a.thread_key, a.kind, coalesce(a.channel_provider, ''), 'system'
		  FROM activity a WHERE a.id = $1`, judged)
	old := e.seedWait(t, "Old", "person_id", person)
	e.exec(t, `UPDATE activity SET occurred_at = now() - interval '150 days' WHERE id = $1`, old)

	got := moved(before, hiddenNow(t, e))

	if got.Shown != 3 {
		t.Fatalf("the queue gained %d waits, wanted the three nothing hides", got.Shown)
	}
	// Each rule finds its OWN row. A relaxation wired to the wrong hole shows up
	// here as a zero beside a one, which the per-rule cases above would also
	// catch — what this adds is that they hold when all four fire at once.
	for name, count := range map[string]int{
		"set aside":    got.SetAside,
		"not_sales":    got.NotSales,
		"past horizon": got.PastHorizon,
	} {
		if count != 1 {
			t.Fatalf("%s counted %d with every rule hiding one message: %+v", name, count, got)
		}
	}
	if got.Clear() {
		t.Fatalf("no hidden figure moved over three hidden messages: %+v", got)
	}
}
