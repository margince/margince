// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The plan behind the set of activities a meeting follow-up read gates. Every
// recent meeting with one busy customer names that customer's whole history.
// Joined per meeting, the set grows with meetings times history, though the
// answer is the same history once.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestTheMeetingFollowUpReadsABusyContactsHistoryOnce(t *testing.T) {
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
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE busy_rep AS SELECT $1::uuid id, $2::timestamptz at`, e.rep, asOf); err != nil {
		t.Fatal(err)
	}
	// Forty held meetings in the follow-up window, each with the rep and one
	// customer who has a thousand mails on record.
	if _, err := tx.Exec(ctx, `
	INSERT INTO contact (full_name, source, captured_by) VALUES ('Busy Customer', 'manual', 'human:seed');
	CREATE TEMP TABLE busy_mail AS
	  SELECT gen_random_uuid() id, g, r.at - random() * 30 * interval '1 day' at_time FROM generate_series(1, 1000) g, busy_rep r;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
	  SELECT id, 'email', 'Mail '||g, at_time, 'outbound', 'manual', 'human:seed', 'busy'||g FROM busy_mail;
	INSERT INTO activity_participant (activity_id, address, contact_id, role)
	  SELECT m.id, 'busy@customers.example', c.id, 'to' FROM busy_mail m, contact c WHERE c.full_name = 'Busy Customer';
	CREATE TEMP TABLE busy_meeting AS
	  SELECT gen_random_uuid() id, g, r.at - interval '4 days' - random() * interval '12 days' at_time FROM generate_series(1, 40) g, busy_rep r;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, meeting_status)
	  SELECT id, 'meeting', 'Meeting '||g, at_time, 'outbound', 'manual', 'human:seed', 'held' FROM busy_meeting;
	INSERT INTO activity_participant (activity_id, address, user_id, role)
	  SELECT m.id, 'rep@mine.example', r.id, 'attendee' FROM busy_meeting m, busy_rep r;
	INSERT INTO activity_participant (activity_id, address, contact_id, role)
	  SELECT m.id, 'busy@customers.example', c.id, 'attendee' FROM busy_meeting m, contact c WHERE c.full_name = 'Busy Customer';
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT m.id, 'contact', c.id FROM busy_meeting m, contact c WHERE c.full_name = 'Busy Customer';
	ANALYZE`); err != nil {
		t.Fatalf("seeding the meetings: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		t.Fatal(err)
	}
	every := func(func(any) int) (string, error) { return scopeUnbounded, nil }
	statement, args, err := meetingFollowUpsStatement(e.as(), asOf, 3, every, AwaitingReplyScanCap)
	if err != nil {
		t.Fatal(err)
	}
	var relations float64
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM activity_link) + (SELECT count(*) FROM activity_participant)`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+statement, args...).Scan(&raw); err != nil {
		t.Fatalf("running the meeting follow-up read: %v", err)
	}
	var peak, gated, gatedByID float64
	decodePlan(t, raw).walk(func(n planNode) {
		if n.SubplanName != "CTE "+followUpReadable {
			return
		}
		gated += n.ActualLoops
		n.walk(func(step planNode) {
			peak = max(peak, step.ActualRows*step.ActualLoops)
			if step.Alias == "readable" && strings.Contains(step.IndexCond, "ANY") {
				gatedByID++
			}
		})
	})
	if gated == 0 {
		t.Fatal("the content gate over the meetings' history never ran, so this test checks nothing")
	}
	// No step of the gated set may yield more rows than there are links and
	// participants: one that does joined the history once per meeting.
	if peak > relations {
		t.Errorf("a step of the gated set yields %.0f rows, more than the %.0f links and participants on record", peak, relations)
	}
	if gatedByID == 0 {
		t.Errorf("the content gate walks activity instead of looking up the ids the read needs")
	}
}
