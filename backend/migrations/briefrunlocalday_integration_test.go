// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The local-day backfill, replayed over the state it was written for.
//
// The collapse to one run per rep-day is the dangerous half of that migration,
// because a rep's snooze and dismiss marks live on brief_item rows and the
// candidate filter reads them across ALL of her previous runs — not just the
// newest. Deleting a superseded run without moving its marks silently
// un-suppresses every deal she had answered: a deal snoozed until Friday
// reappears tomorrow, and a dismissed one comes back having changed nothing.
//
// The migration text itself is replayed rather than a copy of its statements,
// so this cannot pass against SQL that no longer ships.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rewindLocalDayMigration puts the schema back the way the migration found it —
// no local_day, no uniqueness — so a test can seed the rows it was written for.
func rewindLocalDayMigration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		ALTER TABLE brief_run DROP CONSTRAINT uq_brief_run_user_day;
		ALTER TABLE brief_run DROP COLUMN local_day`); err != nil {
		t.Fatalf("undoing the local-day migration: %v", err)
	}
}

// replayLocalDayMigration runs the shipped migration text over whatever the
// test seeded. The FILE, not a copy of its statements: a test written against a
// copy keeps passing against SQL that no longer ships.
func replayLocalDayMigration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	up, err := os.ReadFile(filepath.Join("core", "1787670819_a_brief_run_belongs_to_one_local_day.up.sql"))
	if err != nil {
		t.Fatalf("reading the migration: %v", err)
	}
	if _, err := conn.Exec(ctx, string(up)); err != nil {
		t.Fatalf("replaying the local-day migration: %v", err)
	}
}

func TestTheLocalDayCollapseKeepsTheMarksThatSuppressDeals(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	rewindLocalDayMigration(t, conn)
	seedBriefRunFixture(t, conn)
	morning := time.Date(2026, 6, 4, 6, 0, 0, 0, time.UTC)

	// Two runs on one morning, as the old refresh-on-demand path allowed. The
	// rep snoozed a deal in the earlier one; the later one never queued it,
	// which is exactly why the snooze must not die with the run that holds it.
	early := insertBriefRun(t, conn, morning)
	late := insertBriefRun(t, conn, morning.Add(6*time.Hour))
	snoozedUntil := morning.AddDate(0, 0, 3)
	insertBriefItem(t, conn, early, briefFixtureDealA, 1, "snoozed", &morning, &snoozedUntil)
	insertBriefItem(t, conn, late, briefFixtureDealB, 1, "new", nil, nil)

	replayLocalDayMigration(t, conn)

	var runs int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM brief_run`).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("brief_run rows after the collapse = %d, want the 1 survivor", runs)
	}

	// The snooze is on the survivor, with its window intact. Read by deal
	// rather than by item id: what has to survive is the rep's answer about
	// that deal, and the row carrying it is an implementation detail.
	var (
		state string
		until *time.Time
	)
	if err := conn.QueryRow(ctx, `
		SELECT bi.state, bi.snoozed_until
		FROM brief_item bi JOIN brief_run br ON br.id = bi.brief_run_id
		WHERE bi.deal_id = $1`, briefFixtureDealA).Scan(&state, &until); err != nil {
		t.Fatalf("the snoozed deal's mark did not survive the collapse: %v — every deal this rep answered is back in her queue", err)
	}
	if state != "snoozed" {
		t.Fatalf("the surviving mark reads %q, want snoozed", state)
	}
	if until == nil || !until.Equal(snoozedUntil) {
		t.Fatalf("snoozed_until = %v, want the window the rep chose (%v)", until, snoozedUntil)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM brief_item WHERE deal_id = $1`, briefFixtureDealB).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 1 {
		t.Fatalf("the survivor's own item count for its queued deal = %d, want 1", runs)
	}
}

func TestTheLocalDayBackfillUsesTheInstallationZoneNotUTC(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	rewindLocalDayMigration(t, conn)
	seedBriefRunFixture(t, conn)
	if _, err := conn.Exec(ctx,
		`INSERT INTO setting (key, value) VALUES ('installation.timezone', '"Asia/Ho_Chi_Minh"')
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatal(err)
	}

	// 22:30 UTC on the 24th is 05:30 on the 25th in Ho Chi Minh — a morning
	// run, and the rep lived it on the 25th. Backfilling it in UTC would date
	// it to the 24th, and the job would then assemble a SECOND run for the 25th
	// it believes is still missing.
	run := insertBriefRun(t, conn, time.Date(2026, 6, 24, 22, 30, 0, 0, time.UTC))

	replayLocalDayMigration(t, conn)

	var day time.Time
	if err := conn.QueryRow(ctx, `SELECT local_day FROM brief_run WHERE id = $1`, run).Scan(&day); err != nil {
		t.Fatal(err)
	}
	if got := day.Format(time.DateOnly); got != "2026-06-25" {
		t.Fatalf("local_day = %s, want 2026-06-25 — the morning the rep actually lived in the installation's zone", got)
	}
}

// The fixture the two tests above share: one rep and two deals, which is the
// smallest shape brief_run and brief_item's foreign keys will accept.
var (
	briefFixtureUser     = ids.MustParse("01920000-0000-7000-8000-000000000001")
	briefFixtureDealA    = ids.MustParse("01920000-0000-7000-8000-00000000000a")
	briefFixtureDealB    = ids.MustParse("01920000-0000-7000-8000-00000000000b")
	briefFixturePipeline = ids.MustParse("01920000-0000-7000-8000-0000000000c1")
	briefFixtureStage    = ids.MustParse("01920000-0000-7000-8000-0000000000d1")
)

// seedBriefRunFixture writes the rows brief_run and brief_item foreign-key to:
// one rep, and two deals on a pipeline stage. It inserts them directly rather
// than through the module stores, because this package tests SQL against a
// schema and has no composition layer to reach for.
func seedBriefRunFixture(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, 'rep@brief.test', 'Rep')`,
		briefFixtureUser); err != nil {
		t.Fatalf("seeding the fixture rep: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO pipeline (id, name) VALUES ($1, 'Brief')`, briefFixturePipeline); err != nil {
		t.Fatalf("seeding the fixture pipeline: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		 VALUES ($1, $2, 'Open', 1, 'open', 50)`,
		briefFixtureStage, briefFixturePipeline); err != nil {
		t.Fatalf("seeding the fixture stage: %v", err)
	}
	for _, deal := range []ids.UUID{briefFixtureDealA, briefFixtureDealB} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO deal (id, pipeline_id, stage_id, name, owner_id, status, source, captured_by)
			 VALUES ($1, $2, $3, 'Deal', $4, 'open', 'manual', 'test')`,
			deal, briefFixturePipeline, briefFixtureStage, briefFixtureUser); err != nil {
			t.Fatalf("seeding a fixture deal: %v", err)
		}
	}
}

// insertBriefRun writes one pre-migration run — no local_day, which is the
// whole point: the column does not exist yet when the migration replays.
func insertBriefRun(t *testing.T, conn *pgx.Conn, generatedAt time.Time) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := conn.QueryRow(context.Background(),
		`INSERT INTO brief_run (user_id, generated_at, as_of, candidate_count, revenue_norm_minor)
		 VALUES ($1, $2, $2, 3, 5000000) RETURNING id`,
		briefFixtureUser, generatedAt).Scan(&id); err != nil {
		t.Fatalf("seeding a brief run: %v", err)
	}
	return id
}

// insertBriefItem writes one queue entry with the per-rep state the collapse
// must carry forward.
func insertBriefItem(t *testing.T, conn *pgx.Conn, run, deal ids.UUID, rank int,
	state string, stateAt, snoozedUntil *time.Time,
) {
	t.Helper()
	// A snooze carries what it is waiting for, and the column's CHECK pairs the
	// two. Derived from the state rather than taken as a parameter, because
	// every caller here seeds the clock kind and asking each to say so would
	// spread one row's shape across the file.
	var reopenOn *string
	if state == "snoozed" {
		clock := "time"
		reopenOn = &clock
	}
	if _, err := conn.Exec(context.Background(),
		`INSERT INTO brief_item (brief_run_id, deal_id, rank, composite, feature_vector, evidence_ids, state, state_at, snoozed_until, reopen_on)
		 VALUES ($1, $2, $3, 0.5, '{}'::jsonb, ARRAY[]::uuid[], $4, $5, $6, $7)`,
		run, deal, rank, state, stateAt, snoozedUntil, reopenOn); err != nil {
		t.Fatalf("seeding a brief item: %v", err)
	}
}
