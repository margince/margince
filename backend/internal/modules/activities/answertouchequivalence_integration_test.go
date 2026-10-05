// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The call and held-meeting answer arms walked from the sender's contact
// answer exactly what the same arms joined flat answer, over one timeline.
//
// The two spellings differ only in join order, which the planner is free to
// choose for the flat one, so the property worth holding is that no row
// qualifies under one and not the other and no first answer moves. The flat
// spelling is derived from the rendered one here rather than kept as a second
// copy of every arm, so the comparison cannot drift from what ships.

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// contactFirstTouchArm matches one touch arm as touchAnswerArm renders it:
// the walk's table and alias, the touch predicate, and the asker predicate.
var contactFirstTouchArm = regexp.MustCompile(`(?s)CROSS JOIN LATERAL \(SELECT (\w+)\.activity_id FROM (\w+) \w+\s+` +
	`WHERE \w+\.contact_id = answer_asker\.contact_id OFFSET 0\) answer_walk\s+` +
	`CROSS JOIN LATERAL \(SELECT answer_touch\.id, answer_touch\.occurred_at FROM activity answer_touch\s+` +
	`WHERE answer_touch\.id = answer_walk\.activity_id\s+AND (.*?) OFFSET 0\) answer_touch\s+` +
	`WHERE (answer_asker\.activity_id = \w+\.id AND answer_asker\.role = 'from')`)

// flatTouchArms is sql with its touch arms joined flat: the walked table joined
// on the sender's contact, the activity joined on its id, and every predicate
// in one WHERE, which is how the planner was free to start from the activity.
func flatTouchArms(t *testing.T, sql string) string {
	t.Helper()
	if n := len(contactFirstTouchArm.FindAllStringIndex(sql, -1)); n == 0 || n%2 != 0 {
		t.Fatalf("the rendered statement carries %d contact-first touch arms, want a link and an attendee arm per answer check", n)
	}
	return contactFirstTouchArm.ReplaceAllString(sql,
		`JOIN $2 $1 ON $1.contact_id = answer_asker.contact_id
	  JOIN activity answer_touch ON answer_touch.id = $1.activity_id
	  WHERE $4
	    AND $3`)
}

// seedTouchTimeline writes one inbound message per edge case, each from its own
// contact, at base. The touches that may or may not answer them follow.
func seedTouchTimeline(ctx context.Context, t *testing.T, tx pgx.Tx, base time.Time) {
	t.Helper()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE touch_base AS SELECT $1::timestamptz AS base`, base); err != nil {
		t.Fatalf("seeding the touch timeline's base: %v", err)
	}
	if _, err := tx.Exec(ctx, `
	CREATE TEMP TABLE touch_case (name text, contact uuid, inbound uuid, at timestamptz);
	INSERT INTO touch_case
	  SELECT name, gen_random_uuid(), gen_random_uuid(), (SELECT base FROM touch_base) + shift
	    FROM (VALUES ('linked', interval '0'), ('attended', interval '0'), ('company', interval '0'),
	                 ('silent', interval '0'), ('late', interval '0'), ('unreadable', interval '0'),
	                 ('both', interval '0'), ('old', interval '-400 days')) c(name, shift);
	INSERT INTO contact (id, full_name, source, captured_by)
	  SELECT contact, 'Touch '||name, 'manual', 'human:seed' FROM touch_case;
	INSERT INTO company (id, display_name, source, captured_by)
	  VALUES ('00000000-0000-7000-8000-00000000c0c0', 'Touch Co', 'manual', 'human:seed');
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
	  SELECT inbound, 'email', 'Ask '||name, at, 'inbound', 'manual', 'human:seed', 'touch-'||name FROM touch_case;
	INSERT INTO activity_participant (activity_id, address, contact_id, role)
	  SELECT inbound, name||'@touch.example', contact, 'from' FROM touch_case;
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT inbound, 'contact', contact FROM touch_case;
	-- A sender capture never matched to a contact: no arm walks from it.
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by, thread_key)
	  VALUES ('00000000-0000-7000-8000-0000000000aa', 'email', 'Ask nobody', (SELECT base FROM touch_base), 'inbound', 'manual', 'human:seed', 'touch-nobody');
	INSERT INTO activity_participant (activity_id, address, role)
	  VALUES ('00000000-0000-7000-8000-0000000000aa', 'nobody@touch.example', 'from');

	CREATE TEMP TABLE touch (id uuid, name text, kind text, status text, shift interval, via text,
	                         archived bool, restricted bool, audience text);
	INSERT INTO touch
	  SELECT gen_random_uuid(), name, kind, status, shift, via, archived, restricted, audience
	    FROM (VALUES
	      ('linked',     'call',    NULL,     interval '2 hours',  'link',     false, false, 'workspace'),
	      ('old',        'call',    NULL,     interval '2 hours',  'link',     false, false, 'workspace'),
	      ('attended',   'meeting', 'held',   interval '5 hours',  'attendee', false, false, 'workspace'),
	      ('company',    'call',    NULL,     interval '1 hour',   'company',  false, false, 'workspace'),
	      ('late',       'call',    NULL,     interval '40 days',  'link',     false, false, 'workspace'),
	      ('unreadable', 'call',    NULL,     interval '1 hour',   'link',     true,  false, 'workspace'),
	      ('unreadable', 'call',    NULL,     interval '2 hours',  'link',     true,  true,  'workspace'),
	      ('unreadable', 'call',    NULL,     interval '3 hours',  'link',     false, false, 'participants'),
	      ('unreadable', 'meeting', 'booked', interval '4 hours',  'attendee', false, false, 'workspace'),
	      ('unreadable', 'meeting', 'held',   interval '0',        'attendee', false, false, 'workspace'),
	      ('unreadable', 'call',    NULL,     interval '-1 hour',  'link',     false, false, 'workspace'),
	      ('both',       'call',    NULL,     interval '3 hours',  'link',     false, false, 'workspace'),
	      ('both',       'meeting', 'held',   interval '1 hour',   'attendee', false, false, 'workspace'),
	      ('both',       'meeting', 'held',   interval '1 hour',   'link',     false, false, 'workspace')
	    ) v(name, kind, status, shift, via, archived, restricted, audience);
	INSERT INTO activity (id, kind, meeting_status, occurred_at, source, captured_by, audience,
	                      archived_at, restricted_at, restricted_reason, restricted_until, retention_class, retention_class_at)
	  SELECT tc.id, tc.kind, tc.status, c.at + tc.shift, 'manual', 'human:seed', tc.audience,
	         CASE WHEN tc.archived THEN now() END,
	         CASE WHEN tc.restricted THEN now() END, CASE WHEN tc.restricted THEN 'legal_hold' END,
	         CASE WHEN tc.restricted THEN now() + interval '1 year' END,
	         CASE WHEN tc.restricted THEN 'commercial_correspondence' END, CASE WHEN tc.restricted THEN now() END
	    FROM touch tc JOIN touch_case c USING (name);
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT tc.id, 'contact', c.contact FROM touch tc JOIN touch_case c USING (name) WHERE tc.via = 'link';
	-- A call is never linked to a company, so the company's answer is a call
	-- with a colleague of the sender: neither arm walks from the company.
	INSERT INTO contact (id, full_name, source, captured_by)
	  VALUES ('00000000-0000-7000-8000-00000000c011', 'Touch colleague', 'manual', 'human:seed');
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT tc.id, 'contact', '00000000-0000-7000-8000-00000000c011' FROM touch tc WHERE tc.via = 'company';
	INSERT INTO activity_link (activity_id, entity_type, company_id)
	  SELECT inbound, 'company', '00000000-0000-7000-8000-00000000c0c0' FROM touch_case WHERE name = 'company';
	INSERT INTO activity_participant (activity_id, address, contact_id, role)
	  SELECT tc.id, tc.name||'@touch.example', c.contact, 'attendee' FROM touch tc JOIN touch_case c USING (name) WHERE tc.via = 'attendee';
	ANALYZE activity; ANALYZE activity_link; ANALYZE activity_participant`); err != nil {
		t.Fatalf("seeding the touch timeline: %v", err)
	}
}

// touchAnswer is one inbound message's reading: when its first answer came,
// and whether it counts as answered.
type touchAnswer struct {
	Subject  string
	First    *time.Time
	Answered bool
}

func readTouchAnswers(ctx context.Context, t *testing.T, tx pgx.Tx, sql string, base time.Time) []touchAnswer {
	t.Helper()
	rows, err := tx.Query(ctx, sql, base)
	if err != nil {
		t.Fatalf("reading the answers: %v", err)
	}
	answers, err := pgx.CollectRows(rows, pgx.RowToStructByPos[touchAnswer])
	if err != nil {
		t.Fatalf("collecting the answers: %v", err)
	}
	return answers
}

func TestTheContactFirstTouchArmsAnswerExactlyWhatTheFlatOnesDo(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded timeline: %v", err)
		}
	}()
	base := time.Now().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	seedTouchTimeline(ctx, t, tx, base)

	// Unbounded, as the response spread reads it, and bounded inside the late
	// answer, as the owed reading does.
	for name, until := range map[string]string{"unbounded": `'infinity'::timestamptz`, "bounded": `($1::timestamptz + interval '30 days')`} {
		t.Run(name, func(t *testing.T) {
			sql := `SELECT inbound.subject, ` + firstAnswerAtSQL("inbound", until) + `, ` + answeredSQL("inbound", until) + `
			  FROM activity inbound
			 WHERE inbound.direction = 'inbound' AND inbound.thread_key LIKE 'touch-%' AND $1::timestamptz IS NOT NULL
			 ORDER BY inbound.subject`
			got := readTouchAnswers(ctx, t, tx, sql, base)
			want := readTouchAnswers(ctx, t, tx, flatTouchArms(t, sql), base)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("the contact-first arms answered\n%+v\nthe flat ones\n%+v", got, want)
			}
			answeredAfter := map[string]time.Duration{}
			for _, a := range got {
				if a.First != nil && a.Answered {
					answeredAfter[a.Subject] = a.First.Sub(base)
				} else if a.First != nil || a.Answered {
					t.Errorf("%s has a first answer %v but answered=%v", a.Subject, a.First, a.Answered)
				}
			}
			wantAfter := map[string]time.Duration{
				"Ask linked": 2 * time.Hour, "Ask attended": 5 * time.Hour, "Ask both": time.Hour,
				"Ask old": 2*time.Hour - 400*24*time.Hour,
			}
			if name == "unbounded" {
				wantAfter["Ask late"] = 40 * 24 * time.Hour
			}
			if !reflect.DeepEqual(answeredAfter, wantAfter) {
				t.Errorf("the timeline answered %v, want %v — an edge case is not exercised", answeredAfter, wantAfter)
			}
		})
	}

	t.Run("response spread", func(t *testing.T) {
		statement := fmt.Sprintf(firstResponseSQL, scopeUnbounded, liveRecord(openDealPredicate, "d"),
			liveRecord(workingLeadPredicate, "ld"), ownDomainSenderSQL("inbound", 3), slowPercentile)
		now := time.Now()
		read := func(sql string) [2]int64 {
			var spread [2]int64
			if err := tx.QueryRow(ctx, sql, now.AddDate(0, 0, -waitingHorizonWindowDays), now, []string{"mine.example"}).
				Scan(&spread[0], &spread[1]); err != nil {
				t.Fatalf("measuring the spread: %v", err)
			}
			return spread
		}
		got, want := read(statement), read(flatTouchArms(t, statement))
		if got != want {
			t.Fatalf("the contact-first spread is %v, the flat one %v", got, want)
		}
		if got[0] < 4 {
			t.Errorf("the spread counted %d answers, want at least the timeline's four in the window", got[0])
		}
	})
}
