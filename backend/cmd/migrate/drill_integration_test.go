// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package main

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var startedDrillID = regexp.MustCompile(`^drill ([0-9a-f-]{36}) started`)

// drillInstallation is a migrated database holding one workspace, which is
// what the ledger binds to.
func drillInstallation(t *testing.T, suffix string) string {
	t.Helper()
	maint, base, withDB := testDSNs(t)
	name := base + suffix
	t.Cleanup(func() { mustMigrate(t, "drop-db", "--dsn", maint, "--name", name) })
	mustMigrate(t, "recreate-db", "--dsn", maint, "--name", name)
	dsn := withDB(name)
	mustMigrate(t, "up", "--dsn", dsn)
	stamp(t, dsn, `INSERT INTO workspace DEFAULT VALUES`)
	return dsn
}

type recordedDrill struct {
	outcome, operator, notes string
	restoredTo               time.Time
	recovery                 *time.Duration
}

func readDrill(t *testing.T, dsn, id string) recordedDrill {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to read the ledger: %v", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing the ledger connection: %v", err)
		}
	}()
	var d recordedDrill
	var started time.Time
	var finished *time.Time
	if err := conn.QueryRow(ctx, `
		SELECT outcome, operator, coalesce(notes, ''), restored_to, started_at, finished_at
		  FROM restore_drill WHERE id = $1`, id).
		Scan(&d.outcome, &d.operator, &d.notes, &d.restoredTo, &started, &finished); err != nil {
		t.Fatalf("reading drill %s: %v", id, err)
	}
	if finished != nil {
		window := finished.Sub(started)
		d.recovery = &window
	}
	return d
}

// The operator's whole path: a drill opened and then closed leaves a row under
// the operator's name. The database clock measured its recovery window.
func TestADrillRecordedFromTheCommandLineLeavesMeasuredEvidence(t *testing.T) {
	dsn := drillInstallation(t, "_drill_ledger")
	restoredTo := time.Now().Add(-45 * time.Minute).UTC().Truncate(time.Second)

	out := mustMigrate(t, "drill-start", "--dsn", dsn, "--by", "Ops On Call",
		"--restored-to", restoredTo.Format(time.RFC3339), "--note", "quarterly rehearsal")
	match := startedDrillID.FindStringSubmatch(out)
	if match == nil {
		t.Fatalf("drill-start printed %q, want the drill id first", out)
	}
	id := match[1]
	if running := readDrill(t, dsn, id); running.outcome != "running" || running.recovery != nil {
		t.Fatalf("a started drill reads %q with a recovery window; it has not finished", running.outcome)
	}

	mustMigrate(t, "drill-finish", "--dsn", dsn, "--drill", id,
		"--outcome", "passed", "--note", "restored copy opened; counts matched")

	d := readDrill(t, dsn, id)
	if d.outcome != "passed" {
		t.Errorf("outcome = %q, want passed", d.outcome)
	}
	if d.operator != "Ops On Call" {
		t.Errorf("operator = %q, want the name the operator typed", d.operator)
	}
	if !d.restoredTo.Equal(restoredTo) {
		t.Errorf("restored_to = %s, want %s", d.restoredTo, restoredTo)
	}
	if d.recovery == nil || *d.recovery < 0 {
		t.Errorf("a finished drill has no measured recovery window: %v", d.recovery)
	}
	if d.notes != "restored copy opened; counts matched" {
		t.Errorf("notes = %q, want the closing note", d.notes)
	}
}

// The first answer is the one that happened: a drill already closed refuses a
// second close instead of letting it rewrite the outcome.
func TestAClosedDrillCannotBeClosedAgainFromTheCommandLine(t *testing.T) {
	dsn := drillInstallation(t, "_drill_closed")
	out := mustMigrate(t, "drill-start", "--dsn", dsn, "--by", "ops",
		"--restored-to", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	id := startedDrillID.FindStringSubmatch(out)[1]
	mustMigrate(t, "drill-finish", "--dsn", dsn, "--drill", id, "--outcome", "failed")

	if _, err := migrateCmd(t, "drill-finish", "--dsn", dsn, "--drill", id, "--outcome", "passed"); err == nil {
		t.Fatal("a second drill-finish succeeded on a closed drill")
	}
	if d := readDrill(t, dsn, id); d.outcome != "failed" {
		t.Fatalf("the second close rewrote the outcome to %q", d.outcome)
	}
}

// What the operator typed is checked before anything is written. A missing
// name, a bad time and a future restore point each leave the ledger empty.
func TestDrillStartRefusesInputThatWouldRecordFalseEvidence(t *testing.T) {
	dsn := drillInstallation(t, "_drill_refused")
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	cases := map[string][]string{
		"no operator":         {"--restored-to", past},
		"not a time":          {"--by", "ops", "--restored-to", "yesterday"},
		"restore point ahead": {"--by", "ops", "--restored-to", time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := migrateCmd(t, append([]string{"drill-start", "--dsn", dsn}, args...)...)
			if err == nil {
				t.Fatal("drill-start accepted it")
			}
			if !strings.Contains(err.Error(), "drill-start") {
				t.Errorf("error %q does not name the verb", err)
			}
		})
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing: %v", err)
		}
	}()
	var rows int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM restore_drill`).Scan(&rows); err != nil {
		t.Fatalf("counting drills: %v", err)
	}
	if rows != 0 {
		t.Fatalf("refused input still wrote %d drill row(s)", rows)
	}
}

// Each verb leaves its audit row in the transaction that wrote the drill. The
// actor is the command line's system pass; the typed name is the evidence.
func TestDrillVerbsLeaveAnAuditRowUnderTheSystemPass(t *testing.T) {
	dsn := drillInstallation(t, "_drill_audit")
	out := mustMigrate(t, "drill-start", "--dsn", dsn, "--by", "Dana Ops",
		"--restored-to", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	id := startedDrillID.FindStringSubmatch(out)[1]
	mustMigrate(t, "drill-finish", "--dsn", dsn, "--drill", id, "--outcome", "passed")

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("closing: %v", err)
		}
	}()
	rows, err := conn.Query(ctx, `
		SELECT action, actor_id, coalesce(evidence->>'operator', '')
		  FROM audit_log WHERE entity_type = 'restore_drill' AND entity_id = $1
		 ORDER BY occurred_at, action DESC`, id)
	if err != nil {
		t.Fatalf("reading the audit rows: %v", err)
	}
	var got []string
	for rows.Next() {
		var action, actor, operator string
		if err := rows.Scan(&action, &actor, &operator); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		got = append(got, action+"|"+actor+"|"+operator)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}
	want := []string{"create|" + drillActor + "|Dana Ops", "close|" + drillActor + "|"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("audit rows = %v, want %v", got, want)
	}
}
