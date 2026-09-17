// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The repair that retires the reminders written before the draw learned to hold.
//
// The migration is the only part of this change that touches rows a rep can
// already see, and its three steps are each a way to get it wrong: fold the two
// starters together, archive a child whose account nobody is watching, or stamp
// a row twice and bump its version for nothing. Each test below is one of those.
//
// The SQL is read from the migration file rather than restated here. A test that
// keeps its own copy of a repair proves the copy works.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const checkInRepairMigration = "1789623285_one_open_check_in_per_record.up.sql"

// runCheckInRepair applies the migration's own text inside one transaction, the
// way the runner does. ON COMMIT DROP needs the transaction to exist, so a bare
// Exec on the pool would fail on the temporary table alone.
func runCheckInRepair(t *testing.T, owner *pgx.Conn) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "migrations", "core", checkInRepairMigration)
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the repair migration: %v", err)
	}
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the repair transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("applying the repair: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the repair: %v", err)
	}
}

// seedLegacyReminder writes a reminder the way the starters used to: the
// handler's own subject wording, the engine's provenance, and NO source_system,
// which is exactly what makes it invisible to the new hold.
func seedLegacyReminder(t *testing.T, owner *pgx.Conn, subject, capturedBy string, entityType string, entity ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, due_at, source, captured_by, is_done)
		 VALUES ($1, 'task', $2, now() - interval '9 days', now() - interval '9 days', 'system', $3, false)`,
		id, subject, capturedBy); err != nil {
		t.Fatalf("seeding the legacy reminder: %v", err)
	}
	column := map[string]string{"contact": "contact_id", "company": "company_id", "deal": "deal_id"}[entityType]
	if column == "" {
		t.Fatalf("no activity_link column for %q", entityType)
	}
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, `+column+`) VALUES ($1, $2, $3)`,
		id, entityType, entity); err != nil {
		t.Fatalf("linking the legacy reminder: %v", err)
	}
	return id
}

func isArchived(t *testing.T, owner *pgx.Conn, id ids.UUID) bool {
	t.Helper()
	var archived bool
	if err := owner.QueryRow(context.Background(),
		`SELECT archived_at IS NOT NULL FROM activity WHERE id = $1`, id).Scan(&archived); err != nil {
		t.Fatalf("reading the reminder: %v", err)
	}
	return archived
}

func provenanceOf(t *testing.T, owner *pgx.Conn, id ids.UUID) (string, string) {
	t.Helper()
	var system, sourceID *string
	if err := owner.QueryRow(context.Background(),
		`SELECT source_system, source_id FROM activity WHERE id = $1`, id).Scan(&system, &sourceID); err != nil {
		t.Fatalf("reading the provenance: %v", err)
	}
	if system == nil || sourceID == nil {
		return "", ""
	}
	return *system, *sourceID
}

const (
	noActivitySubject = "Check in — no activity since 2026-09-05"
	cadenceSubject    = "Time for a check-in — last touched 2026-09-05"
)

// Three reminders on one record, written as the anchor moved, collapse to the
// newest — the one naming the date a rep would recognise.
func TestTheRepairFoldsOneRecordsRemindersToTheNewest(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	company := e.SeedCompany(t, "Over-reminded Account", nil)

	var ids3 []ids.UUID
	for range 3 {
		ids3 = append(ids3, seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "company", company))
	}

	runCheckInRepair(t, owner)

	if isArchived(t, owner, ids3[2]) {
		t.Errorf("the newest reminder was archived; it is the one naming the current anchor")
	}
	for _, older := range ids3[:2] {
		if !isArchived(t, owner, older) {
			t.Errorf("an older duplicate survived the fold")
		}
	}
}

// The two starters ask different questions on the same record. Folding them
// together would retire one nobody has answered.
func TestTheRepairKeepsTheTwoStartersApart(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	company := e.SeedCompany(t, "Account With Both", nil)

	noActivity := seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "company", company)
	cadence := seedLegacyReminder(t, owner, cadenceSubject, "system:time-scan", "company", company)

	runCheckInRepair(t, owner)

	if isArchived(t, owner, noActivity) || isArchived(t, owner, cadence) {
		t.Errorf("one starter's reminder was folded into the other's (no_activity archived=%v, cadence archived=%v)",
			isArchived(t, owner, noActivity), isArchived(t, owner, cadence))
	}
	if system, _ := provenanceOf(t, owner, noActivity); system != "no_activity_reminder" {
		t.Errorf("source_system = %q, want no_activity_reminder — the subject is what tells them apart", system)
	}
	if system, _ := provenanceOf(t, owner, cadence); system != "check_in_cadence" {
		t.Errorf("source_system = %q, want check_in_cadence", system)
	}
}

// A deal's reminder folds into its account's — but only when the account holds
// one. Archiving a child whose account is unwatched would leave the silence
// reported by nobody, which is the starvation the collapse itself had to avoid.
func TestTheRepairArchivesAChildOnlyWhenItsAccountHoldsOne(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)

	watched := e.SeedCompany(t, "Watched Account", nil)
	watchedDeal := e.SeedDeal(t, "Watched Deal", pipeline, open, nil)
	attachDealToCompany(t, owner, watchedDeal, watched)
	seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "company", watched)
	childOfWatched := seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "deal", watchedDeal)

	unwatched := e.SeedCompany(t, "Unwatched Account", nil)
	unwatchedDeal := e.SeedDeal(t, "Unwatched Deal", pipeline, open, nil)
	attachDealToCompany(t, owner, unwatchedDeal, unwatched)
	childOfUnwatched := seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "deal", unwatchedDeal)

	runCheckInRepair(t, owner)

	if !isArchived(t, owner, childOfWatched) {
		t.Errorf("a deal's reminder survived beside its account's — two questions about one silence")
	}
	if isArchived(t, owner, childOfUnwatched) {
		t.Errorf("a deal's reminder was archived while its account held none; nobody is now asked about that silence")
	}
}

// The repair runs twice in exactly one situation that matters: somebody reruns
// it. The second pass must change nothing at all — not the archival, and not
// updated_at or version on a row it already stamped.
func TestTheRepairIsANoOpTheSecondTime(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	company := e.SeedCompany(t, "Rerun Account", nil)
	survivor := seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "company", company)
	seedLegacyReminder(t, owner, noActivitySubject, "system:time-scan", "company", company)

	runCheckInRepair(t, owner)
	system, sourceID := provenanceOf(t, owner, survivor)
	var versionAfterFirst int
	if err := owner.QueryRow(context.Background(),
		`SELECT version FROM activity WHERE id = $1`, survivor).Scan(&versionAfterFirst); err != nil {
		t.Fatalf("reading the version: %v", err)
	}

	runCheckInRepair(t, owner)

	systemAgain, sourceIDAgain := provenanceOf(t, owner, survivor)
	if systemAgain != system || sourceIDAgain != sourceID {
		t.Errorf("the identity changed on the second pass: (%q,%q) then (%q,%q)", system, sourceID, systemAgain, sourceIDAgain)
	}
	var versionAfterSecond int
	if err := owner.QueryRow(context.Background(),
		`SELECT version FROM activity WHERE id = $1`, survivor).Scan(&versionAfterSecond); err != nil {
		t.Fatalf("reading the version: %v", err)
	}
	if versionAfterSecond != versionAfterFirst {
		t.Errorf("version moved %d -> %d on a pass that should have written nothing",
			versionAfterFirst, versionAfterSecond)
	}
}

// A task a HUMAN wrote is not this repair's to touch, however much its subject
// looks like a reminder's. source is client-writable, so the pair with
// captured_by is the only thing that marks the engine's own output.
func TestTheRepairLeavesAHumansTaskAlone(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	company := e.SeedCompany(t, "Hand-written Account", nil)

	planted := seedLegacyReminder(t, owner, noActivitySubject, "human:rep-1", "company", company)

	runCheckInRepair(t, owner)

	if isArchived(t, owner, planted) {
		t.Errorf("a task captured by a human was archived by the engine's own repair")
	}
	if system, _ := provenanceOf(t, owner, planted); system != "" {
		t.Errorf("source_system = %q, want empty — a human's task takes no engine identity", system)
	}
}
