// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/vacuumhealth"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// probeTable creates a table the app role can read and runs the given steps on
// it, with {t} standing for its quoted name. It flushes the owner's statistics
// last, so the update and delete counters the read filters on are current.
func probeTable(t *testing.T, e *apptest.AppEnv, columns string, steps ...string) string {
	t.Helper()
	ctx := context.Background()
	name := "vacuum_probe_" + strings.ReplaceAll(ids.NewV7().String(), "-", "")
	var appRole string
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT current_user`).Scan(&appRole)
	}); err != nil {
		t.Fatalf("app role: %v", err)
	}
	table := pgx.Identifier{"public", name}.Sanitize()
	// Autovacuum stays off, so only the test's own ANALYZE sets the statistics.
	statements := append([]string{`CREATE TABLE {t} (` + columns + `) WITH (autovacuum_enabled = false)`}, steps...)
	statements = append(statements, `GRANT SELECT ON {t} TO `+pgx.Identifier{appRole}.Sanitize(),
		`SELECT pg_stat_force_next_flush()`)
	for _, statement := range statements {
		statement = strings.ReplaceAll(statement, "{t}", table)
		if _, err := e.Owner.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		if _, err := e.Owner.Exec(context.Background(), `DROP TABLE `+table); err != nil {
			t.Error(err)
		}
	})
	return name
}

// churnedTable holds 20,000 rows, of which the given percent are deleted
// before ANALYZE. The file keeps its size.
func churnedTable(t *testing.T, e *apptest.AppEnv, deletePercent int) string {
	t.Helper()
	return probeTable(t, e, `id int PRIMARY KEY, body text NOT NULL`,
		`INSERT INTO {t} SELECT g, repeat('x', 200) FROM generate_series(1, 20000) g`,
		fmt.Sprintf(`DELETE FROM {t} WHERE id %% 100 < %d`, deletePercent),
		`ANALYZE {t}`)
}

func vacuumReport(t *testing.T, e *apptest.AppEnv, limits vacuumhealth.Thresholds) crmcontracts.VacuumRecommendations {
	t.Helper()
	var out crmcontracts.VacuumRecommendations
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		var err error
		out, err = vacuumhealth.Read(context.Background(), tx, clocktest.Now(t), limits)
		return err
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	return out
}

func vacuumListed(report crmcontracts.VacuumRecommendations, table string) *crmcontracts.VacuumRecommendation {
	for i := range report.Tables {
		if report.Tables[i].Table == table {
			return &report.Tables[i]
		}
	}
	return nil
}

// The test tables are a few megabytes, so the byte floor is lowered to one
// megabyte; the share threshold is the shipped one.
func TestATableWithMostRowsDeletedIsRecommendedAndALightlyChurnedOneIsNot(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	heavy := churnedTable(t, e, 60)
	light := churnedTable(t, e, 5)
	limits := vacuumhealth.Thresholds{MinWastePercent: vacuumhealth.Default.MinWastePercent, MinWasteBytes: 1 << 20}

	report := vacuumReport(t, e, limits)
	got := vacuumListed(report, heavy)
	if got == nil {
		t.Fatalf("a table with 60%% of its rows deleted is not recommended: %+v", report.Tables)
	}
	if got.WastePercent < 50 || got.WastePercent > 70 || got.WasteBytes < 1<<20 {
		t.Fatalf("the estimate is off: %d%% of %d bytes, waste %d", got.WastePercent, got.TableBytes, got.WasteBytes)
	}
	if got.Statement != `VACUUM (FULL, ANALYZE) "public"."`+heavy+`";` || got.Owner == "" {
		t.Fatalf("the advice to run is incomplete: %+v", got)
	}
	// With no byte floor, only the share keeps the lightly churned table off.
	if lightly := vacuumListed(vacuumReport(t, e, vacuumhealth.Thresholds{MinWastePercent: limits.MinWastePercent}), light); lightly != nil {
		t.Fatalf("a table with 5%% of its rows deleted is recommended: %+v", lightly)
	}

	limits.MinWasteBytes = 1 << 30
	if over := vacuumListed(vacuumReport(t, e, limits), heavy); over != nil {
		t.Fatalf("a table under the byte floor is recommended: %+v", over)
	}
}

func TestOnlyAdminsAndOpsReadTheVacuumRecommendations(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var report crmcontracts.VacuumRecommendations
	if status := e.Call(t, "GET", "/v1/admin/vacuum-recommendations", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("admin read: %d", status)
	}
	if report.MinWastePercent != vacuumhealth.Default.MinWastePercent || int64(report.MinWasteBytes) != vacuumhealth.Default.MinWasteBytes {
		t.Fatalf("the report states thresholds it did not use: %+v", report)
	}
	assignSessionRole(t, e, "rep")
	if status := e.Call(t, "GET", "/v1/admin/vacuum-recommendations", nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("a rep read the recommendations: %d", status)
	}
}

// Four bool and bigint pairs: each bigint is padded to 8 bytes. A model that only
// adds up the widths reads about a quarter of this packed file as empty.
// One updated row brings it past the read's filter for tables never changed.
func TestAFreshlyLoadedTableWithPaddedColumnsIsNotRecommended(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	padded := probeTable(t, e, `a bool, b int8, c bool, d int8, f bool, g int8, h bool, i int8`,
		`INSERT INTO {t} SELECT true, g, false, g, true, g, false, g FROM generate_series(1, 40000) g`,
		`UPDATE {t} SET b = 0 WHERE b = 1`,
		`ANALYZE {t}`)
	// No byte floor: the test table is a few megabytes, and the share alone must keep it off.
	limits := vacuumhealth.Thresholds{MinWastePercent: vacuumhealth.Default.MinWastePercent}
	if got := vacuumListed(vacuumReport(t, e, limits), padded); got != nil {
		t.Fatalf("a packed table with no deletes is recommended: %+v", got)
	}
}

// Rows written after ANALYZE grow the file but not the row count ANALYZE took.
// The size must come from that same ANALYZE, or the new rows read as empty space.
// The share threshold is 10% with no byte floor, under the 13% such a read would show.
func TestRowsAddedSinceTheLastAnalyzeAreNotReadAsWaste(t *testing.T) {
	t.Parallel()
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	grown := probeTable(t, e, `id int PRIMARY KEY, body text NOT NULL`,
		`INSERT INTO {t} SELECT g, repeat('x', 200) FROM generate_series(1, 20000) g`,
		`DELETE FROM {t} WHERE id % 100 = 0`,
		`ANALYZE {t}`,
		`INSERT INTO {t} SELECT g, repeat('x', 200) FROM generate_series(20001, 23000) g`)
	limits := vacuumhealth.Thresholds{MinWastePercent: 10}
	if got := vacuumListed(vacuumReport(t, e, limits), grown); got != nil {
		t.Fatalf("a table that only grew since ANALYZE is recommended: %+v", got)
	}
}
