// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A table that declares a lifecycle column has a statement that keeps it.
//
// On a row that is only inserted, `version` and `updated_at` repeat the insert,
// and `version` promises a guarded update path that does not exist. updated_at
// needs a production statement that updates the table. version needs one that
// assigns it, or an update the table's trigger answers by assigning NEW.version.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// lifecycleColumn reads a catalog line declaring an integer version counter or
// an updated_at timestamp. A text `version` names a wording, not a counter.
var lifecycleColumn = regexp.MustCompile(`^public\.(\w+)\.(version (?:bigint|integer)|updated_at timestamp)`)

// lifecycleTableFloor is the vacuity floor: the catalog held 130 such tables
// when this gate was written, and far fewer means the reader stopped matching.
const lifecycleTableFloor = 100

// rowWritingCalls are the storekit entry points whose table argument names a
// row they update, with that argument's position. LockRow and LockPair stand in
// for ApplyLocked, which takes only the lock. None of them assigns version.
var rowWritingCalls = map[string]int{
	"ApplyWithVersion": 2, "ApplyGuarded": 2, "ApplyGuardedIn": 2, "LockRow": 2, "LockPair": 2,
	"ClaimOwnership": 2,
}

var (
	// sqlFunction is one function a migration installs, with its body.
	sqlFunction = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\b.*?\bAS\s+(\$\w*\$)(.*?)\$\w*\$`)
	// updateTrigger is a row trigger that runs before an update, with or without
	// a `WHEN` clause.
	updateTrigger = regexp.MustCompile(`^public\.(\w+)\.\w+ CREATE TRIGGER \w+ BEFORE UPDATE ON public\.\w+ FOR EACH ROW (?:WHEN \(.*\) )?EXECUTE FUNCTION (\w+)\(\)$`)
	// versionIncrement is a SET item a writer keeps apart and joins into its
	// statement at run time, so no whole statement holds it.
	versionIncrement = regexp.MustCompile(`(?i)^\s*version\s*=\s*version\s*\+\s*1\s*$`)
)

// legacyEvidenceReason is why a retired evidence table keeps its lifecycle.
const legacyEvidenceReason = "A legacy row only moves out to field_provenance, and the move carries its version " +
	"and updated_at, so the columns hold the values the row had before the table was retired."

// appendOnlyLifecycle names each table, or table column, that keeps a lifecycle
// column although no statement keeps it, with why.
var appendOnlyLifecycle = gatekit.Waive(map[string]string{
	"company_fact_legacy":          legacyEvidenceReason,
	"company_profile_field_legacy": legacyEvidenceReason,
	"contact_profile_field_legacy": legacyEvidenceReason,
	"deal_product.updated_at": "Each write deletes a deal's allocations and inserts them again, so updated_at is when " +
		"the row was written. ProductSetTag hashes it to tell one allocation set from the next.",
	"list_revision.version": "version copies the list's own version at the revision it records. It is a snapshot " +
		"of the counter on list, not a counter of this row.",
})

// lifecycleCatalog is what the head catalog says: the lifecycle columns of each
// table, and the tables whose update trigger assigns NEW.version.
type lifecycleCatalog struct {
	columns       map[string][]string
	versionBumped map[string]bool
}

func readLifecycleCatalog(t *testing.T) lifecycleCatalog {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	catalog := lifecycleCatalog{columns: map[string][]string{}, versionBumped: map[string]bool{}}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := lifecycleColumn.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			column, _, _ := strings.Cut(m[2], " ")
			catalog.columns[m[1]] = append(catalog.columns[m[1]], column)
		}
		if m := updateTrigger.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			catalog.versionBumped[m[1]] = catalog.versionBumped[m[1]] || slices.Contains(triggerWrites[m[2]], "version")
		}
	}
	return catalog
}

// lifecycleEvidence is what production statements do to each table. It records
// whether any updates its rows and which columns they assign. It also records
// whether one assembles its SET list at run time.
type lifecycleEvidence struct {
	updated   map[string]bool
	assigned  map[string]map[string]bool
	assembled map[string]bool
}

func newLifecycleEvidence() lifecycleEvidence {
	return lifecycleEvidence{updated: map[string]bool{}, assigned: map[string]map[string]bool{}, assembled: map[string]bool{}}
}

func (e lifecycleEvidence) assign(table, column string) {
	if e.assigned[table] == nil {
		e.assigned[table] = map[string]bool{}
	}
	e.assigned[table][column] = true
}

// note reads one statement or function body.
func (e lifecycleEvidence) note(sql string) {
	for _, segment := range writeSegments(sql) {
		e.updated[segment.table] = true
		if segment.unreadable || segment.assignments == "" {
			e.assembled[segment.table] = true
		}
		for _, column := range assignedColumns(segment.assignments) {
			e.assign(segment.table, column)
		}
	}
}

// kept reports whether a production statement keeps column of table.
func (e lifecycleEvidence) kept(catalog lifecycleCatalog, table, column string) bool {
	if column == "version" {
		return e.assigned[table]["version"] || e.updated[table] && catalog.versionBumped[table]
	}
	return e.updated[table]
}

// readLifecycleEvidence walks every production statement.
func readLifecycleEvidence(t *testing.T) lifecycleEvidence {
	t.Helper()
	evidence := newLifecycleEvidence()
	packages := map[string]lifecycleEvidence{}
	increments := map[string]bool{}
	packageConstants := map[string]map[string]string{}
	for _, root := range []string{"internal", "pkg", "../extensions"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch {
			case d.IsDir() && d.Name() == "testdata":
				return filepath.SkipDir
			case d.IsDir():
				return nil
			case strings.HasSuffix(path, ".sql") && !strings.HasSuffix(path, ".down.sql"):
				return noteFunctionBodies(path, evidence.note)
			case !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path):
				return nil
			}
			src, err := gatekit.ReadFile(path)
			if err != nil {
				return err
			}
			dir := filepath.Dir(path)
			if _, ok := packages[dir]; !ok {
				packages[dir] = newLifecycleEvidence()
			}
			if packages[dir].noteGoFile(t, path, string(src), packageConstants) {
				increments[dir] = true
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	for dir, pkg := range packages {
		evidence.merge(pkg, increments[dir])
	}
	for _, dir := range []string{"migrations/baseline/core", "migrations/core"} {
		entries, err := gatekit.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".up.sql") {
				if err := noteFunctionBodies(filepath.Join(dir, entry.Name()), evidence.note); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return evidence
}

// noteGoFile reads one Go file's statements and storekit row writers into its
// package's evidence, and reports whether the file holds a version increment.
func (e lifecycleEvidence) noteGoFile(t *testing.T, path, src string, packageConstants map[string]map[string]string) bool {
	t.Helper()
	increments := false
	for _, sql := range gatekit.SQLStatementsIn(t, path, src) {
		e.note(sql)
		increments = increments || versionIncrement.MatchString(sql)
	}
	e.noteRowWritingCalls(t, path, packageConstants)
	return increments
}

// merge adds a package's evidence. A version increment kept apart from its
// statement counts only on a table whose SET list the package assembles.
func (e lifecycleEvidence) merge(pkg lifecycleEvidence, increments bool) {
	for table := range pkg.updated {
		e.updated[table] = true
		if increments && pkg.assembled[table] {
			e.assign(table, "version")
		}
	}
	for table, columns := range pkg.assigned {
		for column := range columns {
			e.assign(table, column)
		}
	}
}

func noteFunctionBodies(path string, note func(string)) error {
	raw, err := gatekit.ReadFile(path)
	if err != nil {
		return err
	}
	for _, m := range sqlFunction.FindAllStringSubmatch(string(raw), -1) {
		note(m[2])
	}
	return nil
}

// noteRowWritingCalls records the table a storekit row writer is handed, as a
// literal or as a constant the package declares.
func (e lifecycleEvidence) noteRowWritingCalls(t *testing.T, path string, packageConstants map[string]map[string]string) {
	t.Helper()
	file, err := gatekit.ParseFile(path, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	constants := packageStringConstants(t, filepath.Dir(path), packageConstants)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		at, ok := rowWritingCalls[calledName(call)]
		if !ok || len(call.Args) <= at {
			return true
		}
		switch arg := call.Args[at].(type) {
		case *ast.BasicLit:
			if table, err := strconv.Unquote(arg.Value); err == nil {
				e.updated[table] = true
			}
		case *ast.Ident:
			if table, ok := constants[arg.Name]; ok {
				e.updated[table] = true
			}
		}
		return true
	})
}

func packageStringConstants(t *testing.T, dir string, packageConstants map[string]map[string]string) map[string]string {
	t.Helper()
	if cached, ok := packageConstants[dir]; ok {
		return cached
	}
	constants := map[string]string{}
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(dir, entry.Name()), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					if lit, ok := value.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if text, err := strconv.Unquote(lit.Value); err == nil {
							constants[name.Name] = text
						}
					}
				}
			}
		}
	}
	packageConstants[dir] = constants
	return constants
}

func TestALifecycleColumnSitsOnARowThatIsUpdated(t *testing.T) {
	t.Parallel()
	defer appendOnlyLifecycle.AssertAllMatched(t)
	catalog := readLifecycleCatalog(t)
	if len(catalog.columns) < lifecycleTableFloor {
		t.Fatalf("the catalog yields %d tables with a lifecycle column, under the floor of %d; "+
			"the reader has stopped matching the catalog's line shape", len(catalog.columns), lifecycleTableFloor)
	}
	if len(catalog.versionBumped) == 0 {
		t.Fatal("no table's update trigger assigns NEW.version; the reader has stopped matching function bodies")
	}
	evidence := readLifecycleEvidence(t)
	for _, table := range slices.Sorted(maps.Keys(catalog.columns)) {
		for _, column := range catalog.columns[table] {
			if evidence.kept(catalog, table, column) || appendOnlyLifecycle.Waived(t, table) ||
				appendOnlyLifecycle.Waived(t, table+"."+column) {
				continue
			}
			t.Errorf("%s.%s has no production statement that keeps it. Drop the column in a migration, "+
				"or waive it in appendOnlyLifecycle with why it stays.", table, column)
		}
	}
}

func TestTheLifecycleEvidenceRuleFiresOnPlantedStatements(t *testing.T) {
	t.Parallel()
	catalog := lifecycleCatalog{versionBumped: map[string]bool{"bumped": true}}
	cases := []struct {
		name, sql, table, column string
		kept                     bool
	}{
		{"an insert alone keeps nothing", `INSERT INTO ledger (id) VALUES ($1)`, "ledger", "updated_at", false},
		{"an update keeps updated_at", `UPDATE ledger SET note = $2 WHERE id = $1`, "ledger", "updated_at", true},
		{"an update without a bump trigger keeps no version", `UPDATE ledger SET note = $2 WHERE id = $1`, "ledger", "version", false},
		{"an update the trigger answers keeps version", `UPDATE bumped SET note = $2 WHERE id = $1`, "bumped", "version", true},
		{"an assigned increment keeps version", `UPDATE ledger SET version = version + 1 WHERE id = $1`, "ledger", "version", true},
		{"a version named in WHERE is no assignment", `UPDATE ledger SET note = $2 WHERE version = $1`, "ledger", "version", false},
		{"an upsert after an insert keeps only its own table",
			`INSERT INTO ledger (id) VALUES ($1); INSERT INTO other (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET note = $2`,
			"ledger", "updated_at", false},
		{"an upsert keeps its table", `INSERT INTO other (id) VALUES ($1) ON CONFLICT (id) DO UPDATE SET note = $2`,
			"other", "updated_at", true},
	}
	for _, c := range cases {
		evidence := newLifecycleEvidence()
		evidence.note(c.sql)
		if got := evidence.kept(catalog, c.table, c.column); got != c.kept {
			t.Errorf("%s: kept(%s.%s) = %v, want %v", c.name, c.table, c.column, got, c.kept)
		}
	}
}

func TestALoneIncrementCountsOnlyOnAnAssembledUpdate(t *testing.T) {
	t.Parallel()
	pkg := newLifecycleEvidence()
	pkg.note(`UPDATE assembled SET `)
	pkg.note(`UPDATE written SET note = $2 WHERE id = $1`)
	merged := newLifecycleEvidence()
	merged.merge(pkg, true)
	catalog := lifecycleCatalog{versionBumped: map[string]bool{}}
	if !merged.kept(catalog, "assembled", "version") {
		t.Error("an increment joined into an assembled SET list does not keep version")
	}
	if merged.kept(catalog, "written", "version") {
		t.Error("an increment in the package keeps version on a table whose statement never names it")
	}
}
