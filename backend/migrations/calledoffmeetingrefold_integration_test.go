// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// 1791342035 refolds a stored clock that a called-off meeting moved before
// 1791342034 taught the functions to skip it. The stale value is seeded under
// the head schema and the shipped file is replayed over it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const calledOffRefold = "1791342035_stored_clocks_drop_called_off_meetings.up.sql"

// clockRow is what the refold may change, and what it must not.
type clockRow struct {
	lastActivity *time.Time
	version      int64
	updatedAt    time.Time
}

func readClockRow(t *testing.T, conn *pgx.Conn, table, id string) clockRow {
	t.Helper()
	var r clockRow
	if err := conn.QueryRow(context.Background(),
		`SELECT last_activity_at, version, updated_at FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id).
		Scan(&r.lastActivity, &r.version, &r.updatedAt); err != nil {
		t.Fatalf("reading the %s clock: %v", table, err)
	}
	return r
}

// seedCalledOffMeeting files a held meeting and a later canceled one on a
// contact and on a deal at a company, and answers the three ids.
func seedCalledOffMeeting(t *testing.T, conn *pgx.Conn, held, canceled time.Time) (company, contact, deal string) {
	t.Helper()
	err := conn.QueryRow(context.Background(), `
WITH co AS (
  INSERT INTO company (display_name, source, captured_by) VALUES ('Refold Co', 'manual', 'test') RETURNING id
), ct AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ('Refold Contact', 'manual', 'test') RETURNING id
), p AS (
  INSERT INTO pipeline (name) VALUES ('Refold') RETURNING id
), s AS (
  INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
  SELECT id, 'Open', 1, 'open', 10 FROM p RETURNING id, pipeline_id
), d AS (
  INSERT INTO deal (pipeline_id, stage_id, company_id, name, source, captured_by)
  SELECT s.pipeline_id, s.id, co.id, 'Refold deal', 'manual', 'test' FROM s, co RETURNING id
), m AS (
  INSERT INTO activity (kind, subject, occurred_at, meeting_status, source, captured_by)
  VALUES ('meeting', 'Held', $1, 'held', 'manual', 'test'),
         ('meeting', 'Called off', $2, 'canceled', 'manual', 'test')
  RETURNING id
), ld AS (
  INSERT INTO activity_link (activity_id, entity_type, deal_id) SELECT m.id, 'deal', d.id FROM m, d
), lc AS (
  INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT m.id, 'contact', ct.id FROM m, ct
)
SELECT co.id::text, ct.id::text, d.id::text FROM co, ct, d`, held, canceled).Scan(&company, &contact, &deal)
	if err != nil {
		t.Fatalf("seeding the meetings: %v", err)
	}
	return company, contact, deal
}

// staleClocks puts every clock on the canceled meeting the way the old rule
// left it, through the same suppression move_last_activity uses.
func staleClocks(t *testing.T, conn *pgx.Conn, at time.Time, company, contact, deal string) {
	t.Helper()
	err := inTx(t, conn, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SELECT set_config('margince.last_activity_move', 'on', true)`); err != nil {
			return err
		}
		for table, id := range map[string]string{"company": company, "contact": contact, "deal": deal} {
			if _, err := tx.Exec(ctx, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET last_activity_at = $1 WHERE id = $2`, at, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("staling the clocks: %v", err)
	}
}

func TestTheRefoldDropsACalledOffMeetingFromEveryStoredClock(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	held := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	canceled := held.AddDate(0, 0, 14)
	company, contact, deal := seedCalledOffMeeting(t, conn, held, canceled)
	staleClocks(t, conn, canceled, company, contact, deal)

	records := map[string]string{"company": company, "contact": contact, "deal": deal}
	before := map[string]clockRow{}
	for table, id := range records {
		before[table] = readClockRow(t, conn, table, id)
		if got := before[table].lastActivity; got == nil || !got.Equal(canceled) {
			t.Fatalf("the seeded %s clock is %v, want the canceled meeting %v — the case below proves nothing", table, got, canceled)
		}
	}

	replayMigration(t, conn, calledOffRefold)

	for table, id := range records {
		after := readClockRow(t, conn, table, id)
		if after.lastActivity == nil || !after.lastActivity.Equal(held) {
			t.Errorf("%s.last_activity_at = %v after the refold, want the held meeting %v", table, after.lastActivity, held)
		}
		if after.version != before[table].version || !after.updatedAt.Equal(before[table].updatedAt) {
			t.Errorf("the refold edited the %s: version %d -> %d, updated_at %v -> %v; a clock move is not an edit",
				table, before[table].version, after.version, before[table].updatedAt, after.updatedAt)
		}
	}
}
