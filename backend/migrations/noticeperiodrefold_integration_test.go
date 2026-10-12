// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// 1791782102 refolds every stored company clock to what last_activity_of_company
// answers. The stale values are seeded under the head schema and the shipped
// file is replayed over them.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const noticePeriodRefold = "1791782102_stored_company_clocks_count_a_contact_serving_notice.up.sql"

// seedEmployerWithMail files one note against a contact employed at a new
// company, ending endsInDays after current_date, and answers the company's id.
func seedEmployerWithMail(t *testing.T, conn *pgx.Conn, status string, endsInDays *int, at time.Time) string {
	t.Helper()
	var company string
	err := conn.QueryRow(context.Background(), `
WITH co AS (
  INSERT INTO company (display_name, source, captured_by) VALUES ('Notice Co', 'manual', 'test') RETURNING id
), ct AS (
  INSERT INTO contact (full_name, source, captured_by) VALUES ('Notice Contact', 'manual', 'test') RETURNING id
), r AS (
  INSERT INTO relationship (kind, contact_id, company_id, employment_status, ended_at, source, captured_by)
  SELECT 'employment', ct.id, co.id, $1, current_date + $3::int, 'manual', 'test' FROM ct, co
), a AS (
  INSERT INTO activity (kind, subject, occurred_at, source, captured_by)
  VALUES ('note', 'Call notes', $2, 'manual', 'test') RETURNING id
), l AS (
  INSERT INTO activity_link (activity_id, entity_type, contact_id) SELECT a.id, 'contact', ct.id FROM a, ct
)
SELECT co.id::text FROM co`, status, at, endsInDays).Scan(&company)
	if err != nil {
		t.Fatalf("seeding the employer: %v", err)
	}
	return company
}

// staleCompanyClock sets a company's clock through the suppression
// move_last_activity uses, so the value is one no current rule would fold.
func staleCompanyClock(t *testing.T, conn *pgx.Conn, company string, at *time.Time) {
	t.Helper()
	err := inTx(t, conn, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SELECT set_config('margince.last_activity_move', 'on', true)`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE company SET last_activity_at = $1 WHERE id = $2`, at, company)
		return err
	})
	if err != nil {
		t.Fatalf("staling the company clock: %v", err)
	}
}

func companyRowVersion(t *testing.T, conn *pgx.Conn, company string) string {
	t.Helper()
	var xmin string
	if err := conn.QueryRow(context.Background(), `SELECT xmin::text FROM company WHERE id = $1`, company).Scan(&xmin); err != nil {
		t.Fatalf("reading the company row version: %v", err)
	}
	return xmin
}

func TestTheRefoldMovesEveryStoredCompanyClockToTheCurrentRule(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	mail := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	notice := seedEmployerWithMail(t, conn, "current", new(30), mail)
	former := seedEmployerWithMail(t, conn, "former", nil, mail)
	settled := seedEmployerWithMail(t, conn, "current", nil, mail)
	staleCompanyClock(t, conn, notice, nil)
	staleCompanyClock(t, conn, former, &mail)

	stale := map[string]*time.Time{notice: &mail, former: nil}
	before := map[string]clockRow{}
	for id := range stale {
		before[id] = readClockRow(t, conn, "company", id)
	}
	settledBefore := companyRowVersion(t, conn, settled)
	if got := readClockRow(t, conn, "company", settled).lastActivity; got == nil || !got.Equal(mail) {
		t.Fatalf("the settled company's clock is %v, want %v; the untouched case below proves nothing", got, mail)
	}

	replayMigration(t, conn, noticePeriodRefold)

	for id, want := range stale {
		after := readClockRow(t, conn, "company", id)
		if (after.lastActivity == nil) != (want == nil) || (want != nil && !after.lastActivity.Equal(*want)) {
			t.Errorf("company %s clock = %v after the refold, want %v", id, after.lastActivity, want)
		}
		if after.version != before[id].version || !after.updatedAt.Equal(before[id].updatedAt) {
			t.Errorf("the refold edited company %s: version %d -> %d, updated_at %v -> %v; a clock move is not an edit",
				id, before[id].version, after.version, before[id].updatedAt, after.updatedAt)
		}
	}
	if got := companyRowVersion(t, conn, settled); got != settledBefore {
		t.Errorf("the refold rewrote a company whose clock was already right (xmin %s -> %s)", settledBefore, got)
	}
}
