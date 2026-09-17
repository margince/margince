// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package schemaready

// Ready must mean the schema this binary was built for is there.
//
// Three states of one database, in order, because the middle one is the whole
// reason this check exists. A composed binary against a database that never
// applied a unit's namespace is the easy case; the case that motivated the
// ticket is the ordinary rolling-deploy window, where the tracking table is
// PRESENT and BEHIND. A check that only asked whether the namespace exists
// would report ready for exactly that shape.
//
// The third state is the claim the code makes about the failure: unready, not
// a refused boot, so applying the migration clears it WITHOUT a restart. That
// is only provable by moving the database under a server that keeps running.
//
// OWN PACKAGE, like humanonlyext beside it: compose.RegisterExtensions is a
// process-wide reconciliation that stays applied for the rest of the test
// binary, and a synthetic unit registered in the parent package would be
// visible to that package's whole-surface censuses.

import (
	"context"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/pkg/extension"
)

// unitName is the synthetic unit. Its namespace is ext_readydemo and nothing
// else in the tree owns it, so the states below are this suite's alone.
const unitName = "readydemo"

// The unit's two migrations. Trivial on purpose — what is under test is the
// ledger, not the DDL — but real table DDL rather than a no-op statement, so
// the applied state is one Postgres actually distinguishes.
const (
	firstUp    = `CREATE TABLE ext.ext_readydemo_note (id uuid PRIMARY KEY, body text NOT NULL);`
	firstDown  = `DROP TABLE ext.ext_readydemo_note;`
	secondUp   = `ALTER TABLE ext.ext_readydemo_note ADD COLUMN title text;`
	secondDown = `ALTER TABLE ext.ext_readydemo_note DROP COLUMN title;`
)

// unitMigrations is the unit's embedded layer as the SDK expects it —
// migrations/NNNN_name.up.sql beside its .down.sql.
func unitMigrations() fstest.MapFS {
	return fstest.MapFS{
		extension.MigrationsDir + "/0001_note.up.sql":    {Data: []byte(firstUp)},
		extension.MigrationsDir + "/0001_note.down.sql":  {Data: []byte(firstDown)},
		extension.MigrationsDir + "/0002_title.up.sql":   {Data: []byte(secondUp)},
		extension.MigrationsDir + "/0002_title.down.sql": {Data: []byte(secondDown)},
	}
}

func TestReadyMeansTheComposedUnitsSchemaIsApplied(t *testing.T) {
	ctx := context.Background()
	if err := compose.RegisterExtensions([]extension.Extension{{
		Name:        unitName,
		Version:     "1.0.0",
		Description: "A synthetic unit, composed only for this suite's own test binary.",
		Migrations:  unitMigrations(),
	}}, nil, nil); err != nil {
		t.Fatalf("registering the synthetic extension set: %v", err)
	}

	e := apptest.SetupApp(t)
	namespace, err := dbmigrate.NamespaceFor(unitName)
	if err != nil {
		t.Fatalf("resolving the unit's namespace: %v", err)
	}

	// 1 — never applied here. The tracking table does not exist at all.
	if status := readyz(t, e); status != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz = %d with %s never applied, want 503", status, namespace)
	}
	short := pending(ctx, t, e)
	if len(short) != 1 || short[0].Namespace != namespace || !short[0].Untracked {
		t.Fatalf("the probe reports %+v, want one untracked shortfall on %s — read through the APP pool, "+
			"so this also fails if the runtime role cannot read the tracking tables at all", short, namespace)
	}

	// 2 — applied, and BEHIND. The state a check asking only "does the
	// namespace exist" reports ready for, which is the deployment shape this
	// whole check was filed about.
	applyThrough(ctx, t, e, namespace, 1)
	if status := readyz(t, e); status != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz = %d with %s applied to 0001 and 0002 outstanding, want 503 — "+
			"the tracking table EXISTS here, so this is the case the cheap check cannot see", status, namespace)
	}
	short = pending(ctx, t, e)
	if len(short) != 1 || short[0].Untracked || len(short[0].Missing) != 1 || short[0].Missing[0] != "0002" {
		t.Fatalf("the probe reports %+v, want %s missing exactly 0002", short, namespace)
	}

	// 3 — applied. The same running process answers ready, with no restart:
	// the failure was a state of the database, and the operator's fix is to
	// migrate it.
	applyThrough(ctx, t, e, namespace, 2)
	if status := readyz(t, e); status != http.StatusOK {
		t.Fatalf("GET /readyz = %d once %s is at head, want 200 — the process must recover without a restart", status, namespace)
	}
	if short = pending(ctx, t, e); len(short) != 0 {
		t.Fatalf("the probe still reports %+v with every namespace at head", short)
	}
}

// readyz calls the real probe on the assembled server.
func readyz(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	return e.Call(t, http.MethodGet, "/readyz", nil, nil, nil)
}

// pending asks the same question the probe asks, through the APP pool, so the
// answer names the namespace — the probe body carries only the check's name,
// because the detail belongs in the server log.
func pending(ctx context.Context, t *testing.T, e *apptest.AppEnv) []dbmigrate.Shortfall {
	t.Helper()
	ns, err := composedNamespace(t)
	if err != nil {
		t.Fatalf("assembling the unit's namespace: %v", err)
	}
	short, err := dbmigrate.Pending(ctx, e.Pool, ns)
	if err != nil {
		t.Fatalf("reading migration state as the runtime role: %v", err)
	}
	return short
}

func composedNamespace(t *testing.T) (dbmigrate.Namespace, error) {
	t.Helper()
	namespaces, err := dbmigrate.ExtensionNamespaces(compose.ComposedExtensions())
	if err != nil {
		return dbmigrate.Namespace{}, err
	}
	if len(namespaces) != 1 {
		t.Fatalf("the composed set yields %d namespaces, want the one synthetic unit", len(namespaces))
	}
	return namespaces[0], nil
}

// applyThrough migrates the unit's namespace up to and including the nth
// migration, through the REAL engine: the tracking table, its grant to the
// runtime role and the ledger rows all have to be the ones production writes,
// or the probe above would be reading a fixture of this test's own making.
//
// e.Owner is the migration owner the harness already holds — the same role
// cmd/migrate connects as, and the only one that may create a table in ext.
func applyThrough(ctx context.Context, t *testing.T, e *apptest.AppEnv, namespace string, n int) {
	t.Helper()
	full, err := composedNamespace(t)
	if err != nil {
		t.Fatalf("assembling the unit's namespace: %v", err)
	}
	if n > len(full.Migrations) {
		t.Fatalf("asked to apply %d migrations, the unit ships %d", n, len(full.Migrations))
	}
	if _, err := dbmigrate.Up(ctx, e.Owner, dbmigrate.Namespace{
		Name: namespace, Migrations: full.Migrations[:n],
	}); err != nil {
		t.Fatalf("applying %s through %d: %v", namespace, n, err)
	}
}
