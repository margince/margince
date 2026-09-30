// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The risk-day migration plants its retention window into an installation that
// is already running — including one whose ladder an admin emptied.
//
// Bootstrap seeds the ladder once and never again, so for a running
// installation this migration is the only thing that ever writes the row. An
// admin may delete every policy through the API; that installation still
// exists, and a migration that asked the ladder rather than the workspace
// would leave its verdicts ageing on nobody's clock.
//
// The migration text itself is replayed, so this cannot pass against SQL that
// no longer ships.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

// rewindRiskDayMigration puts the schema back the way the migration found it.
func rewindRiskDayMigration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := conn.Exec(context.Background(), `
		DROP TABLE deal_risk_verdict;
		DROP TABLE deal_risk_day;
		DELETE FROM retention_policy`); err != nil {
		t.Fatalf("undoing the risk-day migration: %v", err)
	}
}

func replayRiskDayMigration(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	up, err := os.ReadFile(filepath.Join("core", "1790496494_a_material_deal_at_risk_is_judged_on_record.up.sql"))
	if err != nil {
		t.Fatalf("reading the migration: %v", err)
	}
	if _, err := conn.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("replaying the risk-day migration: %v", err)
	}
}

// riskDayWindow answers the retain_days of the risk-day policy, or -1 for none.
func riskDayWindow(t *testing.T, conn *pgx.Conn) int {
	t.Helper()
	days := -1
	err := conn.QueryRow(context.Background(),
		`SELECT retain_days FROM retention_policy WHERE object_type = 'deal_risk_day' AND category IS NULL`).Scan(&days)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return days
}

func TestARunningInstallationWithAnEmptyLadderGetsTheRiskDayWindow(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	rewindRiskDayMigration(t, conn)
	if _, err := conn.Exec(context.Background(), `INSERT INTO workspace DEFAULT VALUES`); err != nil {
		t.Fatal(err)
	}

	replayRiskDayMigration(t, conn)

	if got := riskDayWindow(t, conn); got != 90 {
		t.Fatalf("an installation whose ladder was emptied has risk-day window %d after the migration, want 90", got)
	}
}

// A fresh database is left for bootstrap to seed, and a window an admin
// already chose is not overwritten.
func TestTheRiskDayWindowIsPlantedOnlyWhereNothingDecidedIt(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	rewindRiskDayMigration(t, conn)

	replayRiskDayMigration(t, conn)
	if got := riskDayWindow(t, conn); got != -1 {
		t.Fatalf("a database with no installation got a risk-day window of %d — bootstrap seeds that, and would then collide", got)
	}

	rewindRiskDayMigration(t, conn)
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO workspace DEFAULT VALUES;
		INSERT INTO retention_policy (object_type, category, retain_days, action)
		VALUES ('deal_risk_day', NULL, 30, 'erase')`); err != nil {
		t.Fatal(err)
	}
	replayRiskDayMigration(t, conn)
	if got := riskDayWindow(t, conn); got != 30 {
		t.Fatalf("the admin's 30-day window reads %d after the migration, want it kept", got)
	}
}
