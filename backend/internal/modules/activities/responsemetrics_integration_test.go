// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// How fast the workspace answers, and how much of the queue it puts down,
// against a real database.
//
// Both figures are SQL — a LATERAL join for the first reply, a percentile for
// the median, and a filtered count over audit_log — and none of it is visible
// to the unit lane: a wrong join direction, a median over the wrong column, or
// a disposition read off the state table instead of the audit row all compile
// and simply report a different number.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func metricsStore(e *loadEnv) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
}

// window is a fortnight around now, wide enough for the fixtures below and
// bounded so the read is over a window rather than over all history.
func window() (time.Time, time.Time) {
	now := time.Now()
	return now.Add(-14 * 24 * time.Hour), now.Add(time.Hour)
}

// seedAnswered writes an inbound and the reply it got, `after` later.
//
// Through the same columns seedWait uses, so what varies between this file's
// fixtures and the waiting lane's is only whether a reply exists.
func seedAnswered(t *testing.T, e *loadEnv, person ids.UUID, ago, after time.Duration) {
	t.Helper()
	inbound, thread := ids.NewV7(), "thread-"+ids.NewV7().String()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Question', now() - $2::interval, $3, 'seed', 'system')`,
		inbound, ago.String(), thread)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), inbound, person)
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Re: Question', now() - $2::interval, $3, 'seed', 'system')`,
		ids.NewV7(), (ago - after).String(), thread)
}

// The median is the MIDDLE wait, not the average of them.
//
// One message answered after a week drags a mean past every figure a reader
// would recognise, and the question is what a customer typically waits. Three
// answered messages with one outlier is the smallest fixture where the two
// answers differ enough to tell apart.
func TestTheResponseTimeIsTheMedianRatherThanTheMean(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	// Waits of 1h, 2h and 100h. Median 2h = 120 minutes; the mean would be
	// roughly 34 hours, which describes nobody's experience.
	seedAnswered(t, e, person, 48*time.Hour, time.Hour)
	seedAnswered(t, e, person, 47*time.Hour, 2*time.Hour)
	seedAnswered(t, e, person, 46*time.Hour, 100*time.Hour)

	from, to := window()
	got, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if got.Answered < 3 {
		t.Fatalf("counted %d answered, wanted at least the three seeded", got.Answered)
	}
	// Asserted as a RANGE because the lane template is shared: another test's
	// answered thread shifts the median, and a fixed number would pass or fail
	// on which tests ran. The mean of this fixture is ~2000 minutes, so any
	// figure this low proves the percentile is what ran.
	if got.MedianMinutes > 400 {
		t.Fatalf("the median came back %d minutes — a mean over these waits would be "+
			"about 2000, so this is not a median", got.MedianMinutes)
	}
}

// The wait is measured to the FIRST reply, not the newest one.
//
// Taking the latest outbound on the thread would report the wait as the length
// of the whole conversation, so a customer answered in an hour and talked to
// for a week would read as a week-long wait.
func TestTheWaitIsMeasuredToTheFirstReplyNotTheLast(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	inbound, thread := ids.NewV7(), "thread-"+ids.NewV7().String()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Question', now() - interval '10 days', $2, 'seed', 'system')`,
		inbound, thread)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), inbound, person)
	// Answered in an hour, then talked to for another nine days.
	for _, at := range []string{"10 days", "1 day"} {
		e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
			VALUES ($1, 'email', 'outbound', 'Re', now() - $2::interval + interval '1 hour', $3, 'seed', 'system')`,
			ids.NewV7(), at, thread)
	}

	from, to := window()
	got, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if got.Answered == 0 {
		t.Fatal("the answered thread was not counted at all")
	}
	// Nine days is 12960 minutes. Anything near that means the LAST reply was
	// measured, which is the defect this case exists for.
	if got.MedianMinutes > 5000 {
		t.Fatalf("the wait came back %d minutes over a thread answered in one hour — "+
			"the last reply was measured, not the first", got.MedianMinutes)
	}
}

// An unanswered thread is not a fast one.
//
// The waiting lane's own subject, and the failure worth guarding: a query that
// admitted threads with no reply at all would count them at some default and
// report the workspace as answering everything.
func TestAnUnansweredThreadIsNotCountedAsAnswered(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	// A wait with no reply — exactly what the Worklist queue is made of.
	e.seedWait(t, "Nobody answered this", "person_id", person)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if after.Answered != before.Answered {
		t.Fatalf("an unanswered wait moved the answered count from %d to %d",
			before.Answered, after.Answered)
	}
}

// Dispositions are counted from the AUDIT row, not from the state table.
//
// activity_reader_state holds what is set aside NOW, so a snooze that lifted
// and a not_mine somebody withdrew have left no trace in it. The audit row is
// the only record the judgement was ever made, which is what a rate over a
// window needs — and a metric read off the state table would fall as readers
// tidied up, reporting less judgement the more of it happened.
func TestDispositionsAreCountedFromTheAuditRowRatherThanTheStateTable(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	activity := e.seedWait(t, "Set aside then withdrawn", "person_id", person)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	// The judgement, audited the way recordDisposition writes it.
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ('human', 'human:seed', 'update', 'activity', $1, '{"disposition":"not_sales"}'::jsonb)`,
		activity)
	// And the reader withdrawing it, so the state table holds nothing.
	e.exec(t, `DELETE FROM activity_reader_state WHERE activity_id = $1`, activity)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if after.Disposed != before.Disposed+1 {
		t.Fatalf("the disposition count moved %d → %d over one audited judgement — "+
			"a read off the state table loses it the moment the reader withdraws",
			before.Disposed, after.Disposed)
	}
	if after.DisposedNotSales != before.DisposedNotSales+1 {
		t.Fatalf("not_sales moved %d → %d, wanted the one judgement",
			before.DisposedNotSales, after.DisposedNotSales)
	}
}

// A snooze is a disposition but not the workspace-wide one.
//
// not_sales gets its own figure because it is the judgement that costs
// everybody: the other two hide a row from one reader, this one hides the
// conversation from all of them and does not lift. A metric that folded them
// together would make a fortnight of ordinary tidying look like a fortnight of
// customers being written off.
func TestOnlyTheWorkspaceWideJudgementCountsAsNotSales(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	activity := e.seedWait(t, "Snoozed", "person_id", person)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ('human', 'human:seed', 'update', 'activity', $1, '{"disposition":"snoozed"}'::jsonb)`,
		activity)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if after.Disposed != before.Disposed+1 {
		t.Fatalf("a snooze did not count as a disposition: %d → %d",
			before.Disposed, after.Disposed)
	}
	if after.DisposedNotSales != before.DisposedNotSales {
		t.Fatalf("a snooze was counted as the workspace-wide judgement: %d → %d",
			before.DisposedNotSales, after.DisposedNotSales)
	}
}

// An update that is not a disposition is not one.
//
// audit_log's `update` on `activity` is written by anything that changes a
// message, so counting the action alone would report every edit as a rep putting
// work down.
func TestAnOrdinaryActivityUpdateIsNotADisposition(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	activity := e.seedWait(t, "Edited", "person_id", person)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ('human', 'human:seed', 'update', 'activity', $1, '{"subject":"Renamed"}'::jsonb)`,
		activity)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if after.Disposed != before.Disposed {
		t.Fatalf("an ordinary edit counted as a disposition: %d → %d",
			before.Disposed, after.Disposed)
	}
}

// A window that ends before it starts is refused rather than answered.
//
// Zeros would read as a quiet fortnight, which is the under-reporting direction
// every figure in this family has to fail away from.
func TestABackwardsWindowIsRefusedRatherThanAnsweredWithZeros(t *testing.T) {
	e := setupLoad(t)
	now := time.Now()

	_, err := metricsStore(e).ResponseWindow(e.as(), now, now.Add(-time.Hour))

	if err == nil {
		t.Fatal("a window ending before it starts answered instead of refusing")
	}
}
