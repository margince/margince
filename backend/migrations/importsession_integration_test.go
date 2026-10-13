// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The import-session migrations turn a run into a saved session. Each backfill
// test rewinds them, seeds rows as older code wrote them, and replays the real
// files. So the shipped SQL is what is checked, not a copy of it.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

const importSessionChecksVersion = "20261013120001"

func customNamespace(t *testing.T) dbmigrate.Namespace {
	t.Helper()
	custom, err := migrations.Custom()
	if err != nil {
		t.Fatalf("loading custom migrations: %v", err)
	}
	return custom
}

// rewindImportSession reverts both import-session migrations on a head database.
func rewindImportSession(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	headSchema(t, conn)
	through := namespaceThroughVersion(t, customNamespace(t), importSessionChecksVersion)
	if _, err := dbmigrate.Down(context.Background(), conn, through, 2); err != nil {
		t.Fatalf("reverting the import-session migrations: %v", err)
	}
}

func replayImportSession(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	if _, err := dbmigrate.Up(context.Background(), conn, customNamespace(t)); err != nil {
		t.Fatalf("applying the import-session migrations over the seeded rows: %v", err)
	}
}

// legacyRun is one import_run row in the shape the code before the session wrote.
type legacyRun struct {
	connector, status, sourceRef, sourceName, runError string
	age                                                time.Duration
}

func seedLegacyRun(t *testing.T, conn *pgx.Conn, run legacyRun) string {
	t.Helper()
	if run.connector == "" {
		run.connector = migration.ConnectorCSV
	}
	var runError *string
	if run.runError != "" {
		runError = &run.runError
	}
	var id string
	if err := conn.QueryRow(context.Background(), `
		INSERT INTO import_run (connector, status, source_ref, source, captured_by, mapping, error, created_at, updated_at)
		VALUES ($1, $2, $3, 'import_api', 'human:fixture',
		        CASE WHEN $4 = '' THEN '{}'::jsonb ELSE jsonb_build_object('source_name', $4::text) END,
		        $5, now() - $6::interval, now() - $6::interval + interval '1 minute')
		RETURNING id::text`,
		run.connector, run.status, run.sourceRef, run.sourceName, runError, run.age.String()).Scan(&id); err != nil {
		t.Fatalf("seeding a legacy %s run: %v", run.status, err)
	}
	return id
}

type sessionRow struct {
	status, runError, sourceName string
	supersededBy                 *string
	approvedAtIsUpdatedAt        bool
	approved, released           bool
}

func readSessionRow(t *testing.T, conn *pgx.Conn, id string) sessionRow {
	t.Helper()
	var row sessionRow
	var runError *string
	if err := conn.QueryRow(context.Background(), `
		SELECT status, error, source_name, superseded_by::text,
		       approved_at IS NOT DISTINCT FROM updated_at, approved_at IS NOT NULL, source_released_at IS NOT NULL
		  FROM import_run WHERE id::text = $1`, id).Scan(
		&row.status, &runError, &row.sourceName, &row.supersededBy,
		&row.approvedAtIsUpdatedAt, &row.approved, &row.released); err != nil {
		t.Fatalf("reading import run %s: %v", id, err)
	}
	if runError != nil {
		row.runError = *runError
	}
	return row
}

func TestTheBackfillCancelsEveryLegacyValidationFailure(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	stopped := seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/a",
		runError: migration.CauseValidationStopped,
	})
	commitStopped := seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/b",
		runError: migration.CauseStopped,
	})
	replayImportSession(t, conn)

	if got := readSessionRow(t, conn, stopped); got.status != migration.StatusCancelled || !got.released {
		t.Errorf("a test run that stopped reads %+v, want cancelled with its file released: it wrote nothing", got)
	}
	if got := readSessionRow(t, conn, commitStopped); got.status != migration.StatusFailed || got.released {
		t.Errorf("a stopped commit reads %+v, want still failed and holding its file: a resume reads it", got)
	}
}

func TestTheBackfillStampsApprovalOnEveryRunThatWrote(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	wrote := map[string]string{}
	for _, status := range []string{"running", "complete", "undoing", "undone"} {
		wrote[status] = seedLegacyRun(t, conn, legacyRun{status: status, sourceRef: "ws/import/" + status})
	}
	wrote["failed"] = seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/failed",
		runError: migration.CauseStopped,
	})
	unwritten := map[string]string{}
	for _, status := range []string{"validating", "awaiting_approval"} {
		unwritten[status] = seedLegacyRun(t, conn, legacyRun{status: status, sourceRef: "ws/import/" + status})
	}
	replayImportSession(t, conn)

	for status, id := range wrote {
		if got := readSessionRow(t, conn, id); !got.approvedAtIsUpdatedAt || !got.approved {
			t.Errorf("a legacy %s run reads %+v, want approved_at = updated_at: it reached a commit", status, got)
		}
	}
	for status, id := range unwritten {
		if got := readSessionRow(t, conn, id); got.approved {
			t.Errorf("a legacy %s run reads approved; nobody has approved it yet", status)
		}
	}
}

func TestTheOlderOfTwoNamedReportsIsCancelled(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	older := seedLegacyRun(t, conn, legacyRun{
		status: "awaiting_approval", sourceRef: "ws/import/old",
		sourceName: "Fair 2026", age: 2 * time.Hour,
	})
	newer := seedLegacyRun(t, conn, legacyRun{
		status: "validating", sourceRef: "ws/import/new",
		sourceName: "Fair 2026", age: time.Hour,
	})
	writing := seedLegacyRun(t, conn, legacyRun{
		status: "running", sourceRef: "ws/import/w",
		sourceName: "Expo", age: 2 * time.Hour,
	})
	laterReport := seedLegacyRun(t, conn, legacyRun{
		status: "awaiting_approval", sourceRef: "ws/import/r",
		sourceName: "Expo", age: time.Hour,
	})
	replayImportSession(t, conn)

	got := readSessionRow(t, conn, older)
	if got.status != migration.StatusCancelled || got.supersededBy == nil || *got.supersededBy != newer ||
		!got.released || got.runError != migration.CauseSuperseded {
		t.Errorf("the older report reads %+v, want cancelled, superseded by %s, released, with the supersede cause", got, newer)
	}
	if got := readSessionRow(t, conn, newer); got.status != "validating" {
		t.Errorf("the newer test run reads %s, want it kept as it was", got.status)
	}
	// A run that writes keeps its name even against a newer report: it cannot be undone by a rename.
	if got := readSessionRow(t, conn, laterReport); got.status != migration.StatusCancelled ||
		got.supersededBy == nil || *got.supersededBy != writing {
		t.Errorf("a report beside a running run of its name reads %+v, want superseded by the running run", got)
	}
}

func TestAnUnnamedRunAndAFlipRunAreActiveBesideANamedOne(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	for _, run := range []legacyRun{
		{status: "validating", sourceRef: "ws/import/named", sourceName: "Fair 2026"},
		{status: "validating", sourceRef: "ws/import/u1"},
		{status: "awaiting_approval", sourceRef: "ws/import/u2"},
		{connector: "salesforce", status: "running", sourceRef: "portal", sourceName: "Fair 2026"},
		{connector: "salesforce", status: "running", sourceRef: "portal", sourceName: "Fair 2026"},
		{status: "pending", sourceRef: "ws/import/draft", sourceName: "Fair 2026"},
	} {
		seedLegacyRun(t, conn, run)
	}
}

func TestTwoActiveRunsOfOneNameAreRefused(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	seedLegacyRun(t, conn, legacyRun{status: "running", sourceRef: "ws/import/a", sourceName: "Fair 2026"})
	_, err := conn.Exec(context.Background(), `
		INSERT INTO import_run (connector, status, source_ref, source, captured_by, mapping)
		VALUES ('csv', 'awaiting_approval', 'ws/import/b', 'import_api', 'human:fixture', '{"source_name": "Fair 2026"}')`)
	if err == nil || !strings.Contains(err.Error(), "import_run_one_active_per_source_name") {
		t.Fatalf("a second active run of one name = %v, want the one-active-per-name index to refuse it", err)
	}
	// The match is exact: the namespace a name files under is case-sensitive.
	seedLegacyRun(t, conn, legacyRun{status: "awaiting_approval", sourceRef: "ws/import/c", sourceName: "fair 2026"})
}

func TestOneUnreleasedRunPerSource(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	rewindImportSession(t, conn)
	finished := seedLegacyRun(t, conn, legacyRun{status: "complete", sourceRef: "ws/import/same", age: 3 * time.Hour})
	resumable := seedLegacyRun(t, conn, legacyRun{
		status: "failed", sourceRef: "ws/import/same",
		runError: migration.CauseStopped, age: 2 * time.Hour,
	})
	report := seedLegacyRun(t, conn, legacyRun{status: "awaiting_approval", sourceRef: "ws/import/same", age: time.Hour})
	replayImportSession(t, conn)

	if got := readSessionRow(t, conn, resumable); got.released {
		t.Error("the resumable commit lost its file; a resume reads it, so it is the run that keeps the upload")
	}
	if got := readSessionRow(t, conn, finished); got.status != migration.StatusComplete || !got.released {
		t.Errorf("the finished run reads %+v, want complete and released", got)
	}
	if got := readSessionRow(t, conn, report); got.status != migration.StatusCancelled || !got.released {
		t.Errorf("the unwritten report of the same upload reads %+v, want cancelled and released", got)
	}

	ctx := context.Background()
	_, err := conn.Exec(ctx, `
		INSERT INTO import_run (connector, status, source_ref, source, captured_by)
		VALUES ('csv', 'pending', 'ws/import/same', 'import_api', 'human:fixture')`)
	if err == nil || !strings.Contains(err.Error(), "import_run_one_live_per_source_ref") {
		t.Fatalf("a second unreleased run of one upload = %v, want the one-live-per-source index to refuse it", err)
	}
	if _, err := conn.Exec(ctx, `UPDATE import_run SET source_released_at = now() WHERE id::text = $1`, resumable); err != nil {
		t.Fatal(err)
	}
	seedLegacyRun(t, conn, legacyRun{status: "pending", sourceRef: "ws/import/same"})
}

func TestTheSourceNameFollowsTheMapping(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	named := seedLegacyRun(t, conn, legacyRun{status: "validating", sourceRef: "ws/import/a", sourceName: "Fair 2026"})
	unnamed := seedLegacyRun(t, conn, legacyRun{status: "validating", sourceRef: "ws/import/b"})
	if got := readSessionRow(t, conn, named).sourceName; got != "Fair 2026" {
		t.Errorf("an old-shape insert naming its source reads source_name %q, want the mapping's name", got)
	}
	if got := readSessionRow(t, conn, unnamed).sourceName; got != "" {
		t.Errorf("an unnamed run reads source_name %q, want empty, which no index holds", got)
	}
	if _, err := conn.Exec(context.Background(), `
		UPDATE import_run SET mapping = jsonb_set(mapping, '{source_name}', '"Expo"') WHERE id::text = $1`, named); err != nil {
		t.Fatal(err)
	}
	if got := readSessionRow(t, conn, named).sourceName; got != "Expo" {
		t.Errorf("after the mapping was renamed, source_name reads %q, want Expo", got)
	}
}

func TestTheDownMigrationDeletesDraftsAndCancelledRuns(t *testing.T) {
	conn := connect(t, ownerDSNOf(t))
	headSchema(t, conn)
	seedLegacyRun(t, conn, legacyRun{status: "pending", sourceRef: "ws/import/draft"})
	seedLegacyRun(t, conn, legacyRun{status: "cancelled", sourceRef: "ws/import/cancelled"})
	kept := seedLegacyRun(t, conn, legacyRun{status: "complete", sourceRef: "ws/import/done"})

	rewound := namespaceThroughVersion(t, customNamespace(t), importSessionChecksVersion)
	if _, err := dbmigrate.Down(context.Background(), conn, rewound, 2); err != nil {
		t.Fatalf("down: %v", err)
	}
	var ids []string
	rows, err := conn.Query(context.Background(), `SELECT id::text FROM import_run`)
	if err != nil {
		t.Fatal(err)
	}
	ids, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != kept {
		t.Errorf("after down the runs are %v, want only the complete run %s: older code would resume a draft", ids, kept)
	}
	replayImportSession(t, conn)
}

func ownerDSNOf(t *testing.T) string {
	t.Helper()
	owner, _ := dsns(t)
	return owner
}
