// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// Each growth step copies the corpus's captured mail one more year back. The
// copy goes to clones of the contacts it touches, as older mail is with others.
// The source set is fixed at the first step, so every year is the same size.
const growthSourceSQL = `CREATE TEMP TABLE IF NOT EXISTS bench_growth_source AS
      SELECT id FROM activity WHERE kind = 'email' AND source_system = 'email'`

// growthShiftArg and growthSuffixArg are the arguments a copy takes, in this
// order. The first is how far back the year goes; the second keeps it unique.
const (
	growthShiftArg  = "$1::interval"
	growthSuffixArg = "$2::text"
)

// growthCopy is one table's copy for a year. from aliases the source rows as
// src; set rewrites the named columns, and every other time column moves back.
type growthCopy struct {
	table, from string
	set         map[string]string
	// deferred are the per-row triggers the corpus seeder also turns off for a
	// bulk insert; moveLastActivity replays what they would have done.
	deferred []string
}

// growthAddress keeps a cloned contact's address unique: "a@b" becomes "a+y3@b".
func growthAddress(column string) string {
	return "regexp_replace(" + column + ", '@', '+' || " + growthSuffixArg + " || '@')"
}

func growthTagged(column string) string {
	return "CASE WHEN " + column + " IS NULL THEN NULL ELSE " + column + " || '-' || " + growthSuffixArg + " END"
}

// growthCopies are in write order: parents before the rows that point at them.
// Participants and links keep their triggers' own stamps, such as the domain
// evidence the waiting scan reads.
var growthCopies = []growthCopy{
	{
		table: "contact", from: `contact src JOIN bench_growth_contact m ON m.orig = src.id`,
		set: map[string]string{"id": "m.fresh", "source_id": growthTagged("src.source_id")},
	},
	{
		table: "contact_email", from: `contact_email src JOIN bench_growth_contact m ON m.orig = src.contact_id
	      WHERE src.archived_at IS NULL`,
		set:      map[string]string{"id": "public.uuidv7()", "contact_id": "m.fresh", "email": growthAddress("src.email")},
		deferred: []string{settlementEvidenceTrigger},
	},
	{
		table: "relationship", from: `relationship src JOIN bench_growth_contact m ON m.orig = src.contact_id
	      WHERE src.kind = 'employment'`,
		set:      map[string]string{"id": "public.uuidv7()", "contact_id": "m.fresh"},
		deferred: []string{"relationship_last_activity"},
	},
	{
		table: "activity", from: `activity src JOIN bench_growth_activity m ON m.orig = src.id`,
		set: map[string]string{
			"id": "m.fresh", "source_id": growthTagged("src.source_id"),
			"thread_key": growthTagged("src.thread_key"),
		},
		deferred: []string{settlementEvidenceTrigger},
	},
	{
		table: "capture_import", from: `capture_import src JOIN bench_growth_activity m ON m.orig = src.activity_id`,
		set: map[string]string{"id": "public.uuidv7()", "activity_id": "m.fresh"},
	},
	{
		table: "activity_participant", from: `activity_participant src
	      JOIN bench_growth_activity m ON m.orig = src.activity_id
	      LEFT JOIN bench_growth_contact c ON c.orig = src.contact_id`,
		set: map[string]string{
			"id": "public.uuidv7()", "activity_id": "m.fresh",
			"contact_id": "coalesce(c.fresh, src.contact_id)",
			"address":    "CASE WHEN c.fresh IS NULL THEN src.address ELSE " + growthAddress("src.address") + " END",
		},
		deferred: []string{settlementEvidenceTrigger},
	},
	// An old year's mail keeps its deal links, as history filed under a
	// long-lived deal does, so the deal and last-touch walks grow with it.
	{
		table: "activity_link", from: `activity_link src
	      JOIN bench_growth_activity m ON m.orig = src.activity_id
	      LEFT JOIN bench_growth_contact c ON c.orig = src.contact_id
	      WHERE src.entity_type IN ('contact', 'company', 'deal')`,
		set:      map[string]string{"id": "public.uuidv7()", "activity_id": "m.fresh", "contact_id": "coalesce(c.fresh, src.contact_id)"},
		deferred: []string{"activity_link_last_activity", settlementEvidenceTrigger},
	},
}

// growHistory adds one year of mail older than everything there, as year
// years back, and returns how many mails it wrote. The maps are temp tables
// on the owner's session, in the throwaway bench database.
func growHistory(t *testing.T, e *apptest.AppEnv, year int) int {
	t.Helper()
	start := time.Now()
	s := dailySeeder{t: t, owner: e.Owner}
	s.exec("growth source", growthSourceSQL)
	s.exec("growth activity map", `CREATE TEMP TABLE bench_growth_activity AS
	      SELECT id AS orig, public.uuidv7() AS fresh FROM bench_growth_source`)
	// UNION removes the duplicates before each contact draws its one new id.
	s.exec("growth contact map", `CREATE TEMP TABLE bench_growth_contact AS
	      SELECT x.contact_id AS orig, public.uuidv7() AS fresh FROM (
	        SELECT l.contact_id FROM activity_link l JOIN bench_growth_source g ON g.id = l.activity_id WHERE l.contact_id IS NOT NULL
	        UNION
	        SELECT p.contact_id FROM activity_participant p JOIN bench_growth_source g ON g.id = p.activity_id WHERE p.contact_id IS NOT NULL) x`)
	shift, suffix := fmt.Sprintf("%d years", year), fmt.Sprintf("y%d", year)
	for _, c := range growthCopies {
		statement := growthInsert(t, e.Owner, c)
		// Postgres refuses an argument the statement never names, and not every copy tags a column.
		args := []any{shift}
		if strings.Contains(statement, growthSuffixArg) {
			args = append(args, suffix)
		}
		s.execDeferringTriggers("growth "+c.table, c.table, c.deferred, statement, args...)
	}
	mails := assertGrownYear(t, e)
	s.exec("growth drop maps", `DROP TABLE bench_growth_activity, bench_growth_contact`)
	s.moveLastActivity()
	settleHomeCorpus(t, e)
	t.Logf("home-growth: year %d added %d mails in %s", year, mails, time.Since(start).Round(time.Millisecond))
	return mails
}

// assertGrownYear fails a year shorter than its source: every mail and every
// participant is copied, and the participant trigger stamped the same evidence.
func assertGrownYear(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var source, copied, sourceParties, copiedParties, sourceStamped, copiedStamped int
	if err := e.Owner.QueryRow(context.Background(), `SELECT
	        (SELECT count(*) FROM bench_growth_source),
	        (SELECT count(*) FROM activity a JOIN bench_growth_activity m ON m.fresh = a.id),
	        (SELECT count(*) FROM activity_participant p JOIN bench_growth_source g ON g.id = p.activity_id),
	        (SELECT count(*) FROM activity_participant p JOIN bench_growth_activity m ON m.fresh = p.activity_id),
	        (SELECT count(domain_evidence_at) FROM activity_participant p JOIN bench_growth_source g ON g.id = p.activity_id),
	        (SELECT count(domain_evidence_at) FROM activity_participant p JOIN bench_growth_activity m ON m.fresh = p.activity_id)`).
		Scan(&source, &copied, &sourceParties, &copiedParties, &sourceStamped, &copiedStamped); err != nil {
		t.Fatalf("counting the grown year: %v", err)
	}
	if source == 0 || copied != source || copiedParties != sourceParties || copiedStamped != sourceStamped {
		t.Fatalf("the grown year is short: %d of %d mails, %d of %d participants, %d of %d stamped with domain evidence",
			copied, source, copiedParties, sourceParties, copiedStamped, sourceStamped)
	}
	return copied
}

// growthInsert builds the copy from the table's own columns. So a column added
// later is copied too, and a new time column moves back with the year.
func growthInsert(t *testing.T, owner *pgx.Conn, c growthCopy) string {
	t.Helper()
	rows, err := owner.Query(context.Background(), `SELECT attname::text,
	        atttypid IN ('timestamptz'::regtype, 'timestamp'::regtype)
	      FROM pg_attribute WHERE attrelid = $1::regclass AND attnum > 0 AND NOT attisdropped AND attgenerated = ''
	      ORDER BY attnum`, c.table)
	if err != nil {
		t.Fatalf("reading %s's columns: %v", c.table, err)
	}
	var columns, values []string
	used := 0
	for rows.Next() {
		var name string
		var timed bool
		if err := rows.Scan(&name, &timed); err != nil {
			t.Fatalf("reading %s's columns: %v", c.table, err)
		}
		column := pgx.Identifier{name}.Sanitize()
		value, rewritten := c.set[name]
		switch {
		case rewritten:
			used++
		case timed:
			value = "src." + column + " - " + growthShiftArg
		default:
			value = "src." + column
		}
		columns, values = append(columns, column), append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading %s's columns: %v", c.table, err)
	}
	if used != len(c.set) {
		t.Fatalf("the growth step rewrites %d columns of %s, and the table has only %d of them", len(c.set), c.table, used)
	}
	return "INSERT INTO " + pgx.Identifier{c.table}.Sanitize() + " (" + strings.Join(columns, ", ") + ")\n" +
		"      SELECT " + strings.Join(values, ", ") + " FROM " + c.from
}
