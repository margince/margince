// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The plan behind each deal's next meeting follow-up. A shared database holds
// far more mail than one workspace's meetings, and its statistics lag behind
// what was just written. Neither may make the batch walk that mail once per
// meeting, or read a meeting once per deal it is filed under.

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The other mail outnumbers what the reader's own meetings may cost, twice
// over meetings squared, so one pass over it stands out. A slice of it names
// a contact, so a walk of the links reads it too.
const (
	planMeetings   = 201
	planDeals      = 20
	planOtherMail  = 100000
	planLinkedMail = 5000
)

func TestTheDealFollowUpBatchReadsEachMeetingOnceWhateverTheStatisticsSay(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded meetings: %v", err)
		}
	}()
	asOf := clocktest.Now(t)
	analyzeAnEarlierTestsMail(ctx, t, tx)
	deals := seedDealMeetingsAmongOtherMail(ctx, t, tx, e.rep, planOtherMail, planLinkedMail)
	if _, err := tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		t.Fatal(err)
	}
	pinNeverAnalyzed(ctx, t, tx)
	forgetAllButLinkStatistics(ctx, t, tx)
	statement, args, err := meetingFollowUpsByDealStatement(e.as(), asOf, 3, deals)
	if err != nil {
		t.Fatal(err)
	}
	// First as a busy shared database leaves it, with nothing but the links
	// analyzed since the rows landed, then with the statistics current.
	for _, stats := range []string{"stale", "current"} {
		if stats == "current" {
			if _, err := tx.Exec(ctx, "ANALYZE"); err != nil {
				t.Fatal(err)
			}
		}
		for _, reuse := range []string{"on", "off"} {
			if _, err := tx.Exec(ctx, "SELECT set_config('enable_material', @reuse, true), set_config('enable_memoize', @reuse, true)", pgx.NamedArgs{"reuse": reuse}); err != nil {
				t.Fatal(err)
			}
			var raw string
			if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement, args...).Scan(&raw); err != nil {
				t.Fatalf("%s statistics, reuse %s: running the deal follow-up batch: %v", stats, reuse, err)
			}
			assertBoundedDealFollowUpPlan(t, stats+" reuse "+reuse, decodePlan(t, raw))
		}
	}
	var answered int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ("+statement+") batch", args...).Scan(&answered); err != nil {
		t.Fatal(err)
	}
	if answered != planDeals {
		t.Fatalf("the batch answered for %d deals, want all %d", answered, planDeals)
	}
}

func TestTheDealFollowUpBatchReadsEachAttendeeByKeyWhenContactsLookEmpty(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded meetings: %v", err)
		}
	}()
	deals := seedDealMeetingsAmongOtherMail(ctx, t, tx, e.rep, 0, 0)
	if _, err := tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		t.Fatal(err)
	}
	pinNeverAnalyzed(ctx, t, tx)
	pinAnalyzedEmpty(ctx, t, tx)
	statement, args, err := meetingFollowUpsByDealStatement(e.as(), clocktest.Now(t), 3, deals)
	if err != nil {
		t.Fatal(err)
	}
	// Bounded run takes about a second; a contact scan per meeting runs for minutes.
	if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '15s'"); err != nil {
		t.Fatal(err)
	}
	var raw string
	err = tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement, args...).Scan(&raw)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.QueryCanceled {
		t.Fatal("the batch ran past 15s with contacts looking empty: a contact scan inside the per-meeting loop")
	}
	if err != nil {
		t.Fatalf("running the deal follow-up batch: %v", err)
	}
	plan := decodePlan(t, raw)
	if plan.ActualRows != planDeals {
		t.Fatalf("the batch answered for %.0f deals, want all %d", plan.ActualRows, planDeals)
	}
	assertAttendeesReadByKey(t, plan)
}

// attendeeContact matches the contact aliases the attendee and recipient lookups
// use; EXPLAIN numbers a repeated alias.
var attendeeContact = regexp.MustCompile(`^(named|kept|rc)(_[0-9]+)?$`)

// assertAttendeesReadByKey holds each attendee's contact to one row read by key,
// and each link and participant read to a few runs per meeting.
func assertAttendeesReadByKey(t *testing.T, plan planNode) {
	t.Helper()
	contacts := 0
	plan.walk(func(n planNode) {
		switch {
		case n.RelationName == "contact" && attendeeContact.MatchString(n.Alias):
			contacts++
			if !readByKey(n) || n.ActualRows+n.RemovedByFilter > 1 {
				t.Errorf("attendee contact %s is read by %s %s on %q, %.0f rows a loop, not by its key",
					n.Alias, n.NodeType, n.IndexName, n.IndexCond, n.ActualRows+n.RemovedByFilter)
			}
		case n.RelationName == "activity_link" || n.RelationName == "activity_participant":
			if n.ActualLoops > perMeetingSteps*planMeetings {
				t.Errorf("%s on %s %s ran %.0f times for %d meetings, more than %d a meeting",
					n.NodeType, n.RelationName, n.Alias, n.ActualLoops, planMeetings, perMeetingSteps)
			}
		}
	})
	if contacts == 0 {
		t.Fatal("the plan reads no attendee contact, so this test checks nothing")
	}
}

// pinAnalyzedEmpty leaves contacts, links and participants as an analyze right after
// a reset leaves them. They hold pages but no rows, so the planner sees none.
func pinAnalyzedEmpty(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, `UPDATE pg_class SET reltuples = 0,
	  relpages = greatest(1, pg_relation_size(oid) / current_setting('block_size')::int)
	 WHERE relnamespace = 'public'::regnamespace AND relname IN ('contact', 'activity_link', 'activity_participant')`); err != nil {
		t.Fatal(err)
	}
}

// assertBoundedDealFollowUpPlan holds the plan to reading each meeting once and
// each later activity by its key, never the unrelated mail.
func assertBoundedDealFollowUpPlan(t *testing.T, stats string, plan planNode) {
	t.Helper()
	var attendeeReads, keyedReads float64
	sets := 0
	plan.walk(func(n planNode) {
		if n.SubplanName == "CTE "+meetingAttendeeSet {
			sets++
			if n.ActualLoops != 1 || n.ActualRows != planMeetings {
				t.Errorf("%s: recipient set ran %.0f times for %.0f meetings", stats, n.ActualLoops, n.ActualRows)
			}
			n.walk(func(recipient planNode) {
				if recipient.NodeType == "Aggregate" && recipient.Strategy == "Plain" {
					attendeeReads += recipient.ActualLoops
				}
				if recipient.RelationName == "activity" {
					t.Errorf("%s: recipient set reads activity by %s %s on %q, not only its links and participants",
						stats, recipient.NodeType, recipient.IndexName, recipient.IndexCond)
				}
			})
		}
		read := (n.ActualRows + n.RemovedByFilter) * n.ActualLoops
		if n.Alias == "keyed" {
			keyedReads += n.ActualLoops
			if !readByKey(n) || n.ActualRows+n.RemovedByFilter > 1 {
				t.Errorf("%s statistics: a later activity is read by %s %s on %q, %.0f rows a loop, not by its key",
					stats, n.NodeType, n.IndexName, n.IndexCond, n.ActualRows+n.RemovedByFilter)
			}
		}
		// A per-meeting step may scan a small table, such as the reader's own
		// meetings. A walk of even the linked slice reads twice this a loop.
		const perMeetingRead = planLinkedMail / 2
		if n.ActualLoops >= planMeetings && read > n.ActualLoops*perMeetingRead {
			t.Errorf("%s statistics: a per-meeting step reads %.0f rows a loop: %s %s %s",
				stats, read/n.ActualLoops, n.NodeType, n.RelationName, n.Alias)
		}
		if read >= planOtherMail {
			t.Errorf("%s statistics: a step reads %.0f rows, the unrelated mail: %s %s %s",
				stats, read, n.NodeType, n.RelationName, n.Alias)
		}
	})
	assertFollowUpsComputedOnce(t, stats, plan)
	if sets != 1 || attendeeReads == 0 || keyedReads == 0 {
		t.Fatalf("%s statistics: the plan read no attendees (%.0f) or no later activity (%.0f), so this test checks nothing",
			stats, attendeeReads, keyedReads)
	}
	if attendeeReads > planMeetings {
		t.Errorf("%s statistics: attendees read %.0f times for %d meetings, once per deal instead of once",
			stats, attendeeReads, planMeetings)
	}
}

// perMeetingSteps bounds how often one step may run for each meeting: a
// follow-up checks a few later activities and arms per meeting. A statement that
// re-runs the follow-ups for every deal link loops planMeetings times as often.
const perMeetingSteps = 8

// assertFollowUpsComputedOnce holds the batch to computing its follow-up rows
// once. Joined as a subquery, a nested loop may run them again for every link.
func assertFollowUpsComputedOnce(t *testing.T, stats string, plan planNode) {
	t.Helper()
	computed := 0
	plan.walk(func(n planNode) {
		if n.ActualLoops > perMeetingSteps*planMeetings {
			t.Errorf("%s: %s %s %s ran %.0f times for %d meetings, so the follow-ups ran again per deal link",
				stats, n.NodeType, n.RelationName, n.Alias, n.ActualLoops, planMeetings)
		}
		if n.SubplanName == "CTE "+followUpsOwed {
			computed++
		}
	})
	if computed != 1 {
		t.Errorf("%s: the plan computes the follow-up rows in %d materialized sets, want exactly one", stats, computed)
	}
}

// readByKey reports whether a scan finds its row through a unique index that
// leads on id. The planner may pick one that carries a version beside it. A
// bitmap heap scan holds that condition on the one index scan under it.
func readByKey(n planNode) bool {
	if n.NodeType == "Bitmap Heap Scan" && len(n.Plans) == 1 {
		n = n.Plans[0]
	}
	return strings.HasPrefix(n.IndexCond, "(id = ")
}

// analyzeAnEarlierTestsMail leaves the column statistics that autovacuum
// takes from an earlier test's few links on three contacts. Planned with
// those, a lookup of one contact's links reads a third of every link.
func analyzeAnEarlierTestsMail(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
	CREATE TEMP TABLE earlier_contact AS SELECT gen_random_uuid() id, n FROM generate_series(1, 3) n;
	INSERT INTO contact (id, full_name, source, captured_by) SELECT id, 'Earlier '||n, 'manual', 'human:seed' FROM earlier_contact;
	CREATE TEMP TABLE earlier_mail AS SELECT gen_random_uuid() id, g FROM generate_series(1, 60) g;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
	  SELECT id, 'email', 'Earlier mail', '2026-01-01T00:00:00Z', 'outbound', 'manual', 'human:seed', 'earlier'||g FROM earlier_mail;
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT m.id, 'contact', c.id FROM earlier_mail m JOIN earlier_contact c ON c.n = 1 + m.g % 3;
	ANALYZE`); err != nil {
		t.Fatalf("analyzing an earlier test's mail: %v", err)
	}
}

// forgetAllButLinkStatistics leaves column statistics only on the links and
// participants that file an activity under a contact, as autovacuum leaves
// them once those tables grow. Without them the planner guesses that each
// contact holds a twentieth of all links. At that guess a scan costs about
// what the index does, so the dead rows of earlier tests pick the plan.
func forgetAllButLinkStatistics(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
	DELETE FROM pg_statistic WHERE starelid IN (SELECT oid FROM pg_class WHERE relnamespace = 'public'::regnamespace);
	DELETE FROM pg_statistic_ext_data WHERE stxoid IN (SELECT oid FROM pg_statistic_ext WHERE stxnamespace = 'public'::regnamespace);
	ANALYZE activity_link, activity_participant`); err != nil {
		t.Fatal(err)
	}
}

// seedDealMeetingsAmongOtherMail files most held meetings under one deal and
// one under each other deal, and logs a larger volume of unrelated mail after
// them. Every row goes through the tables' own triggers, so the linked slice
// stays small and lands before the bulk the link triggers would otherwise read.
// Every mail row runs the activity triggers, so all the other mail in one
// INSERT nears the statement ceiling on a busy runner. A batch stays far below.
const otherMailBatch = 1000

// seedOtherMailInBatches writes plan_other: linked mail first, then the rest.
func seedOtherMailInBatches(ctx context.Context, t *testing.T, tx pgx.Tx, otherMail int) {
	t.Helper()
	for from := 1; from <= otherMail; from += otherMailBatch {
		batch := pgx.NamedArgs{"from": from, "to": from + otherMailBatch - 1}
		if _, err := tx.Exec(ctx, `
		INSERT INTO contact (id, full_name, source, captured_by)
		  SELECT contact, 'Other '||g, 'manual', 'human:seed' FROM plan_other, plan_seed s
		   WHERE g BETWEEN @from AND @to AND g <= s.linked_mail`, batch); err != nil {
			t.Fatalf("seeding the other mail's contacts: %v", err)
		}
		if _, err := tx.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
		  SELECT mail, 'email', 'Other mail '||g, s.at - interval '2 days', 'outbound', 'manual', 'human:seed', 'other'||g
		    FROM plan_other, plan_seed s WHERE g BETWEEN @from AND @to`, batch); err != nil {
			t.Fatalf("seeding the other mail: %v", err)
		}
		if _, err := tx.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		  SELECT mail, 'contact', contact FROM plan_other, plan_seed s
		   WHERE g BETWEEN @from AND @to AND g <= s.linked_mail`, batch); err != nil {
			t.Fatalf("linking the other mail: %v", err)
		}
	}
}

func seedDealMeetingsAmongOtherMail(ctx context.Context, t *testing.T, tx pgx.Tx, rep ids.UUID, otherMail, linkedMail int) []ids.UUID {
	t.Helper()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE plan_seed AS
	  SELECT $1::uuid rep, $2::timestamptz at, gen_random_uuid() pipeline, $3::int meetings, $4::int deals,
	         $5::int other_mail, $6::int linked_mail`,
		rep, clocktest.Now(t), planMeetings, planDeals, otherMail, linkedMail); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
	INSERT INTO pipeline (id, name) SELECT pipeline, 'Plan pipeline' FROM plan_seed;
	INSERT INTO stage (id, pipeline_id, name, "position") SELECT pipeline, pipeline, 'Qualified', 1 FROM plan_seed;
	CREATE TEMP TABLE plan_deal AS SELECT n, gen_random_uuid() id FROM plan_seed s, generate_series(1, s.deals) n;
	INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
	  SELECT d.id, 'Deal '||d.n, 'open', s.rep, s.pipeline, s.pipeline, 'manual', 'system' FROM plan_deal d, plan_seed s;
	CREATE TEMP TABLE plan_meeting AS
	  SELECT gen_random_uuid() id, gen_random_uuid() contact, g, d.id deal,
	         s.at - interval '4 days' - random() * interval '12 days' at_time
	    FROM plan_seed s CROSS JOIN LATERAL generate_series(1, s.meetings) g
	    JOIN plan_deal d ON d.n = greatest(1, g - (s.meetings - s.deals));
	INSERT INTO contact (id, full_name, source, captured_by, owner_id)
	  SELECT m.contact, 'Attendee '||m.g, 'manual', 'human:seed', s.rep FROM plan_meeting m, plan_seed s;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, meeting_status)
	  SELECT id, 'meeting', 'Meeting '||g, at_time, 'outbound', 'manual', 'human:seed', 'held' FROM plan_meeting;
	INSERT INTO activity_participant (activity_id, address, user_id, role)
	  SELECT m.id, 'rep@mine.example', s.rep, 'attendee' FROM plan_meeting m, plan_seed s;
	INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', contact FROM plan_meeting;
	INSERT INTO activity_link (activity_id, entity_type, deal_id) SELECT id, 'deal', deal FROM plan_meeting;
	CREATE TEMP TABLE plan_other AS
	  SELECT gen_random_uuid() mail, gen_random_uuid() contact, g FROM plan_seed s, generate_series(1, s.other_mail) g`); err != nil {
		t.Fatalf("seeding the meetings: %v", err)
	}
	seedOtherMailInBatches(ctx, t, tx, otherMail)
	rows, err := tx.Query(ctx, `SELECT id FROM plan_deal ORDER BY n`)
	if err != nil {
		t.Fatal(err)
	}
	deals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		t.Fatal(err)
	}
	return deals
}
