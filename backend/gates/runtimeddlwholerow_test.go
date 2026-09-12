// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// A table an administrator can ALTER at runtime is never read whole-row.
//
// `customfields` is the one sanctioned runtime DDL: adding a custom field runs
// `ALTER TABLE <object> ADD COLUMN cf_<slug>` on the schema pool while the app
// pool's connections are live. Those connections run pgx's default
// `cache_statement` mode, so each statement is prepared on the server and its
// plan reused for the life of the connection.
//
// Postgres will replan an invalidated statement silently — EXCEPT when the new
// plan's RESULT TYPE differs from the old one, which it refuses outright with
// `0A000 cached plan must not change result type`. Adding a column changes the
// result type of exactly one shape: a projection of the whole row. `SELECT
// id, name FROM contact` is unaffected forever; `SELECT * FROM contact` fails
// once per pooled connection the moment somebody adds a field.
//
// That failure is the worst kind to diagnose. It is intermittent (only
// connections holding the stale plan), self-healing (pgx deallocates and the
// next call works), and attributable to nothing the caller did — the admin who
// added the field is not whoever was holding the request that 5xx'd.
//
// No statement in this tree projects a whole row of one of these tables today,
// which is why the exposure has never fired. The distance between that and a
// live defect is one `SELECT *`, so this is what holds it.
//
// WHAT IT CANNOT SEE, since a prohibition is only as good as its corpus:
//
//   - A statement whose projection clause is computed rather than written —
//     `"SELECT " + cols + " FROM contact"` folds to a readable statement, but
//     one where the `*` itself arrives in a variable does not.
//   - A view or a plpgsql function body that projects a whole row. Neither
//     exists here (no migration in the tree contains a whole-row projection,
//     and TestTheWholeRowProbeStillSeesAWholeRow would not know if that
//     changed) — the SQL corpus below reads migrations too, which is what makes
//     that claim checked rather than remembered.
//   - `extensions/`. A unit's SQL may not name a core table at all, which
//     extensionsqlscope_test.go holds; a unit reading `SELECT * FROM contact`
//     fails there first and for a stronger reason.
//
// The counterpart in production is httperr's `stalePlanFault`: if one of these
// does reach a caller, it answers a retryable 503 rather than an unknown 500.
// This gate is why that mapping is a floor and not the plan.

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// wholeRowProjection matches the two ways a statement asks for every column a
// table has: a bare or qualified `*` in the select list, and the same in
// RETURNING. Both carry the table's full result type into the cached plan.
//
// A `count(*)` and an `array_agg(t.*)` are deliberately not matched: the `(`
// before the star is what tells them apart, and neither projects the row —
// count answers bigint whatever the table holds.
//
// Submatch 1 is the keyword and submatch 2 the qualifier, empty for a bare
// star. Between them they say WHICH relation the row comes from, and the star
// alone never does: a bare SELECT takes it from the FROM clause after it, and a
// RETURNING from the statement's write target, which is a different clause
// entirely and in front of it.
var wholeRowProjection = regexp.MustCompile(`(?is)\b(select|returning)\s+(?:distinct\s+)?([a-z_][a-z0-9_]*\.)?\*`)

// nextFromTarget reads the relation a bare `SELECT *` projects: the first FROM
// after the star. A subquery or a VALUES list yields nothing — `FROM (` is the
// shape this tree's own whole-row projections take, every one of them over a
// CTE or a derived table rather than over a base relation.
var nextFromTarget = regexp.MustCompile(`(?is)\bfrom\s+([a-z_][a-z0-9_]*)`)

// aliasBinding resolves a qualified `t.*` to the relation bound to `t`, which
// is either an alias in a FROM/JOIN clause or the relation's own name used
// unaliased.
func aliasBinding(statement, qualifier string) string {
	bound := regexp.MustCompile(`(?is)\b(?:from|join)\s+([a-z_][a-z0-9_]*)\s+(?:as\s+)?` +
		regexp.QuoteMeta(qualifier) + `\b`)
	if m := bound.FindStringSubmatch(statement); m != nil {
		return strings.ToLower(m[1])
	}
	return strings.ToLower(qualifier)
}

// wholeRowRelations names every relation a statement projects entire.
//
// This is the step that separates the prohibition from a keyword search. A
// statement that mentions a governed table somewhere and projects a whole row
// of a CTE is not the defect — `SELECT * FROM closed UNION ALL SELECT * FROM
// moved` inside a deal report is exactly that, and reading the two facts
// separately would report it.
//
// It is a FLOOR, like every reader over SQL text here: a relation named through
// an interpolated identifier resolves to nothing, so an empty answer means
// "could not tell" and never "projects nothing".
func wholeRowRelations(statement string) []string {
	var relations []string
	for _, m := range wholeRowProjection.FindAllStringSubmatchIndex(statement, -1) {
		if m[4] >= 0 {
			qualifier := strings.TrimSuffix(statement[m[4]:m[5]], ".")
			relations = append(relations, aliasBinding(statement, qualifier))
			continue
		}
		// `RETURNING *` answers the row the statement WROTE, which no FROM
		// clause names. sqlWrites already reads that target for the ownership
		// and PII censuses, so this asks it rather than learning UPDATE/INSERT/
		// DELETE shapes a second time.
		if strings.EqualFold(statement[m[2]:m[3]], "returning") {
			relations = append(relations, sqlWriteTargets(statement)...)
			continue
		}
		if from := nextFromTarget.FindStringSubmatch(statement[m[1]:]); from != nil {
			relations = append(relations, strings.ToLower(from[1]))
		}
	}
	return relations
}

// runtimeDDLTables is what the prohibition is about, derived from the engine
// that alters them rather than restated here. FieldObjects members ARE the
// table names — BuildDDL quotes the object straight into the ALTER TABLE — and
// TestEveryRuntimeDDLObjectIsATableInTheSchema proves that rather than assuming
// it.
func runtimeDDLTables() []string { return customfields.FieldObjects }

// sqlStatement is one statement and where it was read from.
type sqlStatement struct {
	path string
	text string
}

// backendSQLCorpus collects the statements this gate judges: the Go tree's
// string literals, flattened and decoded through gatekit, plus the migrations'
// own SQL. Migrations are read because a view or a function body is a cached
// plan too, and reading only Go would leave that half of the doc above
// unchecked.
func backendSQLCorpus(t *testing.T) []sqlStatement {
	t.Helper()
	var out []sqlStatement
	for _, src := range wholeRowScope.Files(t) {
		for _, text := range gatekit.SQLStatementsOf(src.File) {
			out = append(out, sqlStatement{path: src.Path, text: text})
		}
	}
	err := filepath.WalkDir("migrations", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// Split on `;` so a projection is read against the statement that made
		// it rather than against a whole file, which names a dozen tables and
		// would resolve any star to whichever FROM came next. A function body
		// between `$$` splits at its inner semicolons too — crudely, and in the
		// safe direction: the pieces are smaller than the truth, so a star and
		// a relation that really do belong together stay together.
		for _, statement := range strings.Split(string(body), ";") {
			out = append(out, sqlStatement{path: filepath.ToSlash(path), text: statement})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the migrations: %v", err)
	}
	return out
}

// wholeRowScope proves the roots hold every whole-row projection in the module,
// which is the half of a prohibition that a regex cannot check.
var wholeRowScope = gatekit.Scope{
	// internal only: cmd/ is three main packages that wire compose and address
	// no table, and pkg/ is the extension surface, whose SQL is a unit's own
	// and gated by extensionsqlscope_test.go. A root that finds no subject
	// fails this sweep rather than passing quietly, so neither may be listed
	// speculatively — the day either grows a statement, the sweep below reports
	// it as a subject outside every root.
	Roots:   []string{"internal"},
	Subject: fileProjectsAWholeRow,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func fileProjectsAWholeRow(_ string, file *ast.File) bool {
	for _, text := range gatekit.SQLStatementsOf(file) {
		if wholeRowProjection.MatchString(text) {
			return true
		}
	}
	return false
}

func TestNoStatementProjectsAWholeRowOfATableRuntimeDDLAlters(t *testing.T) {
	t.Parallel()
	governed := make(map[string]bool, len(runtimeDDLTables()))
	for _, table := range runtimeDDLTables() {
		governed[table] = true
	}
	for _, stmt := range backendSQLCorpus(t) {
		for _, table := range wholeRowRelations(stmt.text) {
			if !governed[table] {
				continue
			}
			t.Errorf("%s projects a whole row of %q, which an administrator can ALTER at runtime:\n\t%s\n\n"+
				"Adding a custom field to %q changes this statement's result type, and every pooled "+
				"connection holding its cached plan answers the next request 0A000 — once each, "+
				"self-healing, and traceable to nothing the caller did. Name the columns instead.",
				stmt.path, table, gatekit.FirstLineOf(stmt.text), table)
		}
	}
}

// wholeRowProjectionFloor is what the probe matched when this gate landed. A
// prohibition that stops MATCHING reports a clean tree and is indistinguishable
// from one that is satisfied, so it proves itself against the statements that
// legitimately project a whole row — six of them, every one over a CTE or a
// derived table — before claiming that none projects a governed one.
//
// Pinned at the count rather than under it. Deleting one of the six is a
// one-line change to this constant and a reader who sees why; slack here would
// be a regex that has half stopped working and nothing to say so.
const wholeRowProjectionFloor = 6

// statementCorpusFloor is the same guarantee one level up: a walk that stops
// reaching this tree's SQL matches nothing, for a reason the count above cannot
// tell from a clean tree. Set below the ~7.8k found, because this one tracks
// the whole tree's SQL and every ordinary change moves it.
const statementCorpusFloor = 7000

func TestTheWholeRowProbeStillSeesAWholeRow(t *testing.T) {
	t.Parallel()
	corpus := backendSQLCorpus(t)
	if len(corpus) < statementCorpusFloor {
		t.Fatalf("the corpus is %d statement(s) and was at least %d when this gate landed: the walk "+
			"has stopped reaching this tree's SQL, and the prohibition above is passing over nothing",
			len(corpus), statementCorpusFloor)
	}
	projections := 0
	for _, stmt := range corpus {
		if wholeRowProjection.MatchString(stmt.text) {
			projections++
		}
	}
	if projections < wholeRowProjectionFloor {
		t.Errorf("the whole-row probe matched %d statement(s) and matched at least %d when this gate "+
			"landed: it has stopped recognising the shape it forbids, so the prohibition reads green "+
			"over a tree it can no longer see", projections, wholeRowProjectionFloor)
	}
}

// TestEveryRuntimeDDLObjectIsATableInTheSchema checks the step the prohibition
// depends on and never states: that a FieldObjects member is the name of a real
// table, because BuildDDL puts it straight into `ALTER TABLE <object>`. A
// member that is not — a rename, a vocabulary that drifts from the schema —
// would make the gate above search for a table nothing is called and find
// nothing, which is the shape of a pass.
func TestEveryRuntimeDDLObjectIsATableInTheSchema(t *testing.T) {
	t.Parallel()
	catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	text := string(catalog)
	for _, object := range runtimeDDLTables() {
		if !strings.Contains(text, "\npublic."+object+".") {
			t.Errorf("customfields declares %q a field object, so BuildDDL emits `ALTER TABLE %q`, "+
				"but the head catalog names no such table. Either the table was renamed and "+
				"FieldObjects was not, or the vocabulary and the schema have drifted — and "+
				"TestNoStatementProjectsAWholeRowOfATableRuntimeDDLAlters is searching for a name "+
				"nothing has", object, object)
		}
	}
}
