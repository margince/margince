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
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

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
	deals := seedDealMeetingsAmongOtherMail(ctx, t, tx, e.rep)
	if _, err := tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		t.Fatal(err)
	}
	statement, args, err := meetingFollowUpsByDealStatement(e.as(), asOf, 3, deals)
	if err != nil {
		t.Fatal(err)
	}
	// First as a busy shared database leaves it, with nothing analyzed since
	// the rows landed, then with the statistics current.
	for _, stats := range []string{"stale", "current"} {
		if stats == "current" {
			if _, err := tx.Exec(ctx, "ANALYZE"); err != nil {
				t.Fatal(err)
			}
		}
		var raw string
		if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement, args...).Scan(&raw); err != nil {
			t.Fatalf("running the deal follow-up batch: %v", err)
		}
		assertBoundedDealFollowUpPlan(t, stats, decodePlan(t, raw))
	}
	var answered int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ("+statement+") batch", args...).Scan(&answered); err != nil {
		t.Fatal(err)
	}
	if answered != planDeals {
		t.Fatalf("the batch answered for %d deals, want all %d", answered, planDeals)
	}
}

// assertBoundedDealFollowUpPlan holds the plan to reading each meeting once and
// each later activity by its key, never the unrelated mail.
func assertBoundedDealFollowUpPlan(t *testing.T, stats string, plan planNode) {
	t.Helper()
	var attendeeReads, keyedReads float64
	plan.walk(func(n planNode) {
		read := (n.ActualRows + n.RemovedByFilter) * n.ActualLoops
		switch n.Alias {
		case "attendees":
			attendeeReads += n.ActualLoops
		case "keyed":
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
	if attendeeReads == 0 || keyedReads == 0 {
		t.Fatalf("%s statistics: the plan read no attendees (%.0f) or no later activity (%.0f), so this test checks nothing",
			stats, attendeeReads, keyedReads)
	}
	if attendeeReads > planMeetings {
		t.Errorf("%s statistics: attendees read %.0f times for %d meetings, once per deal instead of once",
			stats, attendeeReads, planMeetings)
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

// seedDealMeetingsAmongOtherMail files most held meetings under one deal and
// one under each other deal, and logs a larger volume of unrelated mail after
// them. Every row goes through the tables' own triggers, so the linked slice
// stays small and lands before the bulk the link triggers would otherwise read.
func seedDealMeetingsAmongOtherMail(ctx context.Context, t *testing.T, tx pgx.Tx, rep ids.UUID) []ids.UUID {
	t.Helper()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE plan_seed AS
	  SELECT $1::uuid rep, $2::timestamptz at, gen_random_uuid() pipeline, $3::int meetings, $4::int deals,
	         $5::int other_mail, $6::int linked_mail`,
		rep, clocktest.Now(t), planMeetings, planDeals, planOtherMail, planLinkedMail); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
	INSERT INTO pipeline (id, name) SELECT pipeline, 'Plan pipeline' FROM plan_seed;
	INSERT INTO stage (id, pipeline_id, name, "position") SELECT pipeline, pipeline, 'Qualified', 1 FROM plan_seed;
	CREATE TEMP TABLE plan_deal AS SELECT n, gen_random_uuid() id FROM plan_seed s, generate_series(1, s.deals) n;
	INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
	  SELECT d.id, 'Deal '||d.n, 'open', s.rep, s.pipeline, s.pipeline, 'seed', 'system' FROM plan_deal d, plan_seed s;
	CREATE TEMP TABLE plan_meeting AS
	  SELECT gen_random_uuid() id, gen_random_uuid() contact, g, d.id deal,
	         s.at - interval '4 days' - random() * interval '12 days' at_time
	    FROM plan_seed s CROSS JOIN LATERAL generate_series(1, s.meetings) g
	    JOIN plan_deal d ON d.n = greatest(1, g - (s.meetings - s.deals));
	INSERT INTO contact (id, full_name, source, captured_by) SELECT contact, 'Attendee '||g, 'manual', 'human:seed' FROM plan_meeting;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, meeting_status)
	  SELECT id, 'meeting', 'Meeting '||g, at_time, 'outbound', 'manual', 'human:seed', 'held' FROM plan_meeting;
	INSERT INTO activity_participant (activity_id, address, user_id, role)
	  SELECT m.id, 'rep@mine.example', s.rep, 'attendee' FROM plan_meeting m, plan_seed s;
	INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT id, 'contact', contact FROM plan_meeting;
	INSERT INTO activity_link (activity_id, entity_type, deal_id) SELECT id, 'deal', deal FROM plan_meeting;
	CREATE TEMP TABLE plan_other AS
	  SELECT gen_random_uuid() mail, gen_random_uuid() contact, g FROM plan_seed s, generate_series(1, s.other_mail) g;
	INSERT INTO contact (id, full_name, source, captured_by)
	  SELECT contact, 'Other '||g, 'manual', 'human:seed' FROM plan_other, plan_seed s WHERE g <= s.linked_mail;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
	  SELECT mail, 'email', 'Other mail '||g, s.at - interval '2 days', 'outbound', 'manual', 'human:seed', 'other'||g
	    FROM plan_other, plan_seed s WHERE g <= s.linked_mail;
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT mail, 'contact', contact FROM plan_other, plan_seed s WHERE g <= s.linked_mail;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
	  SELECT mail, 'email', 'Other mail '||g, s.at - interval '2 days', 'outbound', 'manual', 'human:seed', 'other'||g
	    FROM plan_other, plan_seed s WHERE g > s.linked_mail`); err != nil {
		t.Fatalf("seeding the meetings: %v", err)
	}
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
