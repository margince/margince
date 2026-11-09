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
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The other mail is far more than the meetings. A slice of it names a contact:
// walking those links once per meeting reads more than all of the mail.
const (
	planMeetings   = 201
	planOtherMail  = 50000
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
	busy, quiet := ids.NewV7(), ids.NewV7()
	seedDealMeetingsAmongOtherMail(ctx, t, tx, e.rep, busy, quiet)
	if _, err := tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		t.Fatal(err)
	}
	statement, args, err := meetingFollowUpsByDealStatement(e.as(), asOf, 3, []ids.UUID{busy, quiet})
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
		// Walking even the linked slice of the other mail once per meeting
		// reads this much; the reader's own meetings read far less.
		const walkedPerMeeting = planMeetings * planLinkedMail
		var attendeeReads float64
		decodePlan(t, raw).walk(func(n planNode) {
			if n.Alias == "attendees" {
				attendeeReads += n.ActualLoops
			}
			if read := (n.ActualRows + n.RemovedByFilter) * n.ActualLoops; read >= walkedPerMeeting {
				t.Errorf("%s statistics: a step reads %.0f rows, the other mail once per meeting: %s %s %s",
					stats, read, n.NodeType, n.RelationName, n.Alias)
			}
		})
		if attendeeReads == 0 {
			t.Fatalf("%s statistics: the plan never read a meeting's attendees, so this test checks nothing", stats)
		}
		if attendeeReads > planMeetings {
			t.Errorf("%s statistics: attendees read %.0f times for %d meetings, once per deal instead of once",
				stats, attendeeReads, planMeetings)
		}
	}
	var deals int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM ("+statement+") batch", args...).Scan(&deals); err != nil {
		t.Fatal(err)
	}
	if deals != 2 {
		t.Fatalf("the batch answered for %d deals, want both", deals)
	}
}

// seedDealMeetingsAmongOtherMail files every held meeting but one under busy,
// the last under quiet, and logs a larger volume of unrelated mail after them.
// Every row goes through the tables' own triggers, so the linked slice stays
// small and lands before the bulk the link triggers would otherwise read.
func seedDealMeetingsAmongOtherMail(ctx context.Context, t *testing.T, tx pgx.Tx, rep, busy, quiet ids.UUID) {
	t.Helper()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE plan_seed AS
	  SELECT $1::uuid rep, $2::timestamptz at, $3::uuid busy, $4::uuid quiet, $5::int meetings, $6::int other_mail, $7::int linked_mail`,
		rep, clocktest.Now(t), busy, quiet, planMeetings, planOtherMail, planLinkedMail); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
	INSERT INTO pipeline (id, name) SELECT busy, 'Plan pipeline' FROM plan_seed;
	INSERT INTO stage (id, pipeline_id, name, "position") SELECT busy, busy, 'Qualified', 1 FROM plan_seed;
	INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
	  SELECT d, 'Deal', 'open', rep, busy, busy, 'seed', 'system' FROM plan_seed, unnest(ARRAY[busy, quiet]) d;
	CREATE TEMP TABLE plan_meeting AS
	  SELECT gen_random_uuid() id, gen_random_uuid() contact, g, CASE WHEN g = s.meetings THEN s.quiet ELSE s.busy END deal,
	         s.at - interval '4 days' - random() * interval '12 days' at_time
	    FROM plan_seed s, generate_series(1, s.meetings) g;
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
}
