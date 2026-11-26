// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The activity_link clock triggers fire once per statement. Each step below is
// one statement a writer can send, with one row or several. After each step,
// every stored clock must hold the value the rows give. It must also equal what
// a full refold (last_activity_of_<table>) reads from scratch.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// linkClockFixture holds the records and messages the steps link. private is
// visible to its participants only, so it moves the deal clock and no other.
type linkClockFixture struct {
	contactA, contactB, contactE, company, deal string
	projectP, projectQ                          string
	early, late, private                        string
}

func seedLinkClockFixture(t *testing.T, conn *pgx.Conn, early, late, private time.Time) linkClockFixture {
	t.Helper()
	var f linkClockFixture
	err := conn.QueryRow(context.Background(), `
WITH co AS (
  INSERT INTO company (display_name, source, captured_by) VALUES ('Clock Co', 'manual', 'test') RETURNING id
), ct AS (
  INSERT INTO contact (full_name, source, captured_by)
  VALUES ('Clock A', 'manual', 'test'), ('Clock B', 'manual', 'test'), ('Clock E', 'manual', 'test')
  RETURNING id, full_name
), p AS (
  INSERT INTO pipeline (name) VALUES ('Clock') RETURNING id
), s AS (
  INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
  SELECT id, 'Open', 1, 'open', 10 FROM p RETURNING id, pipeline_id
), d AS (
  INSERT INTO deal (pipeline_id, stage_id, company_id, name, source, captured_by)
  SELECT s.pipeline_id, s.id, co.id, 'Clock deal', 'manual', 'test' FROM s, co RETURNING id
), pr AS (
  INSERT INTO project (name, source, captured_by) VALUES ('Clock P', 'manual', 'test'), ('Clock Q', 'manual', 'test')
  RETURNING id, name
), m AS (
  INSERT INTO activity (kind, subject, occurred_at, audience, source, captured_by)
  VALUES ('note', 'early', $1, 'workspace', 'manual', 'test'),
         ('note', 'late', $2, 'workspace', 'manual', 'test'),
         ('note', 'private', $3, 'participants', 'manual', 'test')
  RETURNING id, subject
)
SELECT (SELECT id::text FROM ct WHERE full_name = 'Clock A'), (SELECT id::text FROM ct WHERE full_name = 'Clock B'),
       (SELECT id::text FROM ct WHERE full_name = 'Clock E'), (SELECT id::text FROM co), (SELECT id::text FROM d),
       (SELECT id::text FROM pr WHERE name = 'Clock P'), (SELECT id::text FROM pr WHERE name = 'Clock Q'),
       (SELECT id::text FROM m WHERE subject = 'early'), (SELECT id::text FROM m WHERE subject = 'late'),
       (SELECT id::text FROM m WHERE subject = 'private')`, early, late, private).
		Scan(&f.contactA, &f.contactB, &f.contactE, &f.company, &f.deal, &f.projectP, &f.projectQ, &f.early, &f.late, &f.private)
	if err != nil {
		t.Fatalf("seeding the clock fixture: %v", err)
	}
	return f
}

// clockWant names one record and the clock it must hold; nil is no clock.
type clockWant struct {
	table, name, id string
	want            *time.Time
}

func assertLinkClocks(t *testing.T, conn *pgx.Conn, step string, wants []clockWant) {
	t.Helper()
	for _, w := range wants {
		var stored, refolded *time.Time
		if err := conn.QueryRow(context.Background(),
			`SELECT last_activity_at, last_activity_of_`+w.table+`(id) FROM `+pgx.Identifier{w.table}.Sanitize()+` WHERE id = $1`, w.id).
			Scan(&stored, &refolded); err != nil {
			t.Fatalf("%s: reading the %s clock: %v", step, w.name, err)
		}
		if !sameClock(stored, w.want) {
			t.Errorf("%s: %s.last_activity_at = %v, want %v", step, w.name, stored, w.want)
		}
		if !sameClock(stored, refolded) {
			t.Errorf("%s: %s stores %v but a refold reads %v", step, w.name, stored, refolded)
		}
	}
}

func sameClock(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func TestLinkStatementsMoveEveryClockTheirRowsReach(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	late := early.AddDate(0, 0, 7)
	private := late.AddDate(0, 0, 7)
	f := seedLinkClockFixture(t, conn, early, late, private)
	run := func(step, sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
	}
	a := func(want *time.Time) clockWant { return clockWant{"contact", "contact A", f.contactA, want} }
	b := func(want *time.Time) clockWant { return clockWant{"contact", "contact B", f.contactB, want} }
	e := func(want *time.Time) clockWant { return clockWant{"contact", "contact E", f.contactE, want} }
	deal := func(want *time.Time) clockWant { return clockWant{"deal", "deal", f.deal, want} }
	company := func(want *time.Time) clockWant { return clockWant{"company", "company", f.company, want} }
	projectP := func(want *time.Time) clockWant { return clockWant{"project", "project P", f.projectP, want} }
	projectQ := func(want *time.Time) clockWant { return clockWant{"project", "project Q", f.projectQ, want} }

	run("one insert", `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
		f.early, f.contactA)
	assertLinkClocks(t, conn, "one insert", []clockWant{a(&early), b(nil), deal(nil), company(nil)})

	// Contact A already holds the early message and B gets two rows; the latest
	// wins. The private message reaches B, the deal and project P, and moves
	// only the deal.
	run("several inserts", `INSERT INTO activity_link (activity_id, entity_type, contact_id, deal_id, project_id)
		VALUES ($1, 'contact', $4, NULL, NULL), ($2, 'contact', $5, NULL, NULL), ($3, 'contact', $5, NULL, NULL),
		       ($1, 'deal', NULL, $6, NULL), ($3, 'deal', NULL, $6, NULL),
		       ($2, 'project', NULL, NULL, $7), ($1, 'project', NULL, NULL, $7), ($3, 'project', NULL, NULL, $7)`,
		f.late, f.early, f.private, f.contactA, f.contactB, f.deal, f.projectP)
	assertLinkClocks(t, conn, "several inserts",
		[]clockWant{a(&late), b(&early), deal(&private), company(&late), projectP(&late), projectQ(nil)})

	// The update refolds the record the row left and the one it reached.
	run("one update", `UPDATE activity_link SET contact_id = $3 WHERE activity_id = $1 AND contact_id = $2`,
		f.late, f.contactA, f.contactB)
	assertLinkClocks(t, conn, "one update", []clockWant{a(&early), b(&late)})

	run("several updates", `UPDATE activity_link SET contact_id = $2 WHERE contact_id = $1`, f.contactB, f.contactE)
	assertLinkClocks(t, conn, "several updates", []clockWant{a(&early), b(nil), e(&late)})

	run("one project update", `UPDATE activity_link SET project_id = $3 WHERE activity_id = $1 AND project_id = $2`,
		f.late, f.projectP, f.projectQ)
	assertLinkClocks(t, conn, "one project update", []clockWant{projectP(&early), projectQ(&late)})

	run("an update of no row", `UPDATE activity_link SET contact_id = $1 WHERE contact_id = $1`, f.contactB)
	assertLinkClocks(t, conn, "an update of no row", []clockWant{b(nil), e(&late)})

	run("one delete", `DELETE FROM activity_link WHERE activity_id = $1 AND deal_id = $2`, f.late, f.deal)
	assertLinkClocks(t, conn, "one delete", []clockWant{deal(&private), company(nil)})

	run("several deletes", `DELETE FROM activity_link WHERE contact_id = ANY ($1::uuid[]) OR project_id = ANY ($2::uuid[])`,
		[]string{f.contactA, f.contactE}, []string{f.projectP, f.projectQ})
	assertLinkClocks(t, conn, "several deletes", []clockWant{a(nil), e(nil), deal(&private), projectP(nil), projectQ(nil)})
}

// A clock move is not an edit of the record, whichever statement made it.
func TestALinkStatementMovesAClockWithoutEditingTheRecord(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	early := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	f := seedLinkClockFixture(t, conn, early, early.AddDate(0, 0, 7), early.AddDate(0, 0, 14))
	before := readClockRow(t, conn, "contact", f.contactA)
	if _, err := conn.Exec(context.Background(), `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $3), ($2, 'contact', $3)`, f.early, f.late, f.contactA); err != nil {
		t.Fatalf("linking: %v", err)
	}
	after := readClockRow(t, conn, "contact", f.contactA)
	if after.lastActivity == nil || after.version != before.version || !after.updatedAt.Equal(before.updatedAt) {
		t.Errorf("the link moved the clock to %v and the version %d -> %d, updated_at %v -> %v; want a clock and no edit",
			after.lastActivity, before.version, after.version, before.updatedAt, after.updatedAt)
	}
}
