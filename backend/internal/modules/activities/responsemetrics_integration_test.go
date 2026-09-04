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

// A forged thread key cannot manufacture an answer from another medium.
//
// thread_key is one flat namespace holding both a mail thread root and a
// channel's provider:bot:chat key, and the mail half is attacker-supplied — it
// is the message's own References root, chosen verbatim by the sender. Matching
// a reply on the key alone lets a stranger name a Telegram conversation whose
// parts are both discoverable, and have somebody else's channel reply counted as
// the answer to their mail.
//
// Two things break if it does. The published median absorbs a data point the
// attacker chose, in whichever direction they chose it. And this reader would
// call the thread answered while waitingSQL — which does apply the medium match
// — still shows it waiting, so the two readers of one question disagree.
//
// Every other thread reader in the tree matches the triple and says in its own
// comment that this is a security control. This case is what keeps the
// restatement here honest.
func TestAReplyOnAnotherMediumDoesNotAnswerAForgedMailThread(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	// The key a channel conversation already lives under.
	forged := "telegram:bot-7:chat-42"
	// The colleague's channel reply on it, which the attacker was never part of.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, channel_provider, source, captured_by)
		VALUES ($1, 'message', 'outbound', 'Sure, sending now', now() - interval '1 hour', $2, 'telegram', 'seed', 'system')`,
		ids.NewV7(), forged)
	// The stranger's mail, carrying that key as its References root.
	inbound := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Forged root', now() - interval '2 hours', $2, 'seed', 'system')`,
		inbound, forged)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), inbound, person)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if after.Answered != before.Answered {
		t.Fatalf("the answered count moved %d → %d over a mail nobody replied to — "+
			"a channel reply on a forged thread key was counted as its answer",
			before.Answered, after.Answered)
	}
}

// Putting a row down and picking it back up counts as NOTHING.
//
// Four verbs write this column and two of them are UNDO. Counting every
// non-null value scored the rep who set a message aside and then thought better
// of it at TWO dispositions rather than none — so the figure ran backwards for
// exactly the behaviour it should reward, and the careful self-correcting rep
// scored worst on a number a manager reads.
//
// Written through the STORE's own verbs rather than by inserting audit rows.
// The defect is in which states the reader counts, so a test that hand-writes
// the audit row is a test agreeing with itself about what the writer produces.
func TestSettingARowAsideAndTakingItBackCountsAsNoDisposition(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	activity := e.seedWait(t, "Second thoughts", "person_id", person)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	store := metricsStore(e)
	id := ids.From[ids.ActivityKind](activity)
	if err := store.SetMessageNotMine(e.as(), id); err != nil {
		t.Fatalf("setting the message aside: %v", err)
	}
	// The undo, through the verb a rep actually presses.
	if err := store.ClearMessageDisposition(e.as(), id); err != nil {
		t.Fatalf("taking it back: %v", err)
	}

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	// ONE, not two and not zero: the not_mine really happened and is counted,
	// and the picked_up that undid it is not a second act of judgement.
	if after.Disposed != before.Disposed+1 {
		t.Fatalf("a set-aside and its undo counted %d dispositions, want the one "+
			"judgement — the undo is not a second one",
			after.Disposed-before.Disposed)
	}
}

// The same for the workspace-wide judgement, which has its own undo verb.
func TestTakingBackANotSalesCountsAsNoFurtherJudgement(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	activity := e.seedWait(t, "Not sales after all", "person_id", person)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	store := metricsStore(e)
	id := ids.From[ids.ActivityKind](activity)
	if err := store.SetThreadNotSales(e.as(), id); err != nil {
		t.Fatalf("judging the thread: %v", err)
	}
	if err := store.ClearThreadNotSales(e.as(), id); err != nil {
		t.Fatalf("taking the judgement back: %v", err)
	}

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if after.Disposed != before.Disposed+1 {
		t.Fatalf("a not_sales and its undo counted %d dispositions, want the one",
			after.Disposed-before.Disposed)
	}
	// And the sales_again does not inflate the figure that costs everybody.
	if after.DisposedNotSales != before.DisposedNotSales+1 {
		t.Fatalf("the workspace-wide figure counted %d, want the one judgement",
			after.DisposedNotSales-before.DisposedNotSales)
	}
}

// A judgement on a conversation this reader cannot open counts for nobody.
//
// The whole point of reading audit_log here is that it records judgements the
// state table has already forgotten — but audit_log holds every workspace's
// bookkeeping without regard to who may read the record judged, so the count
// had no visibility clause at all. The median beside it did, which made one
// response body answer two different questions: "how fast do the conversations
// I can see get answered" and "how much did EVERYBODY put down".
//
// A limited activity captured by nobody this reader is, with no participant row
// and no audience membership for them, is the case that tells the two apart. It
// is invisible to `e.rep` and its judgement must be too.
func TestAJudgementOnAnUnreadableConversationCountsForNobody(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Someone Else''s Buyer', $2, 'seed', 'system')`, person, e.other)
	activity := e.seedWait(t, "A held conversation", "person_id", person)
	// Held to a named audience that does not include this reader, and captured
	// by the other seat — so no arm of the audience test admits `e.rep`.
	e.exec(t, `UPDATE activity SET audience = 'selected', captured_by = $2 WHERE id = $1`,
		activity, "human:"+e.other.String())
	e.exec(t, `INSERT INTO activity_audience_member (activity_id, subject_type, subject_id, created_by)
		VALUES ($1, 'user', $2, 'seed')`, activity, e.other)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	// The other seat judges it, audited the way recordDisposition writes it.
	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ('human', $2, 'update', 'activity', $1, '{"disposition":"not_sales"}'::jsonb)`,
		activity, "human:"+e.other.String())

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if after.Disposed != before.Disposed {
		t.Fatalf("the disposal figure moved %d → %d over a judgement on a conversation "+
			"this reader cannot open — the count is answering for the whole workspace "+
			"while the median beside it answers for the caller",
			before.Disposed, after.Disposed)
	}
	if after.DisposedNotSales != before.DisposedNotSales {
		t.Fatalf("the workspace-wide figure moved %d → %d over an unreadable conversation",
			before.DisposedNotSales, after.DisposedNotSales)
	}
}

// And the admit case, without which the test above passes against a count that
// refuses EVERYTHING. The same fixture, readable: the judgement lands.
func TestAJudgementOnAReadableConversationIsStillCounted(t *testing.T) {
	e := setupLoad(t)
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	activity := e.seedWait(t, "An open conversation", "person_id", person)
	from, to := window()
	before, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}

	e.exec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
		VALUES ('human', 'human:seed', 'update', 'activity', $1, '{"disposition":"not_sales"}'::jsonb)`,
		activity)

	after, err := metricsStore(e).ResponseWindow(e.as(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if after.Disposed != before.Disposed+1 {
		t.Fatalf("a judgement on a workspace-audience conversation counted %d, want the one — "+
			"a clause that refuses every row passes the refusal test above and reports a "+
			"quiet fortnight for a workspace full of judgement",
			after.Disposed-before.Disposed)
	}
}
