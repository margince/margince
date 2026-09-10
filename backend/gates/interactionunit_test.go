// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every count that becomes a relationship-strength number counts the shared
// UNIT, not rows.
//
// A channel message arrives as one row per line, so an afternoon of chat is
// dozens of rows and one conversation. relstrength.InteractionUnitSQL is what
// collapses those to the day they happened on, and a fold that counts rows
// instead reports a chat-only account as the warmest relationship in the
// workspace — a number nothing else on the screen contradicts, so nobody
// notices it is wrong.
//
// It is a gate rather than a test beside relstrength because the package holds
// no query: the folds that must obey live in three other modules, and one of
// them counts from a different table than the rest. A test inside relstrength
// could only assert that the renderer renders.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// unitSentinel stands where a statement interpolates the shared unit. Reading
// the folded text for it, rather than reading the identifier's name, is what
// makes a hand-written second copy of the expression fail: only a var assigned
// from relstrength.InteractionUnitSQL resolves to this, so a local re-spelling
// folds to gatekit.ComputedFragment and is reported.
const unitSentinel = "<relstrength-interaction-unit>"

// countsTheSharedUnit is the shape a compliant count has once folded.
var countsTheSharedUnit = "count(DISTINCT " + unitSentinel + ")"

// deliberateOtherCounts are the counts inside a qualifying statement that are
// not counting interactions at all. Each says what it counts instead, because
// "it is not an interaction count" is the only reason that can be right here.
var deliberateOtherCounts = gatekit.Waive(map[string]string{
	"internal/compose/org360/graphourside.go :: count(*) FROM colleagues)": "counts the colleagues an account has, not the interactions with any of them — it is the total the capped list is a page of",
})

// TestEveryInteractionCountReadsTheSharedUnit is the census.
func TestEveryInteractionCountReadsTheSharedUnit(t *testing.T) {
	t.Parallel()
	defer deliberateOtherCounts.AssertAllMatched(t)

	group := relstrength.InteractionKindSQLGroup()
	if !strings.HasPrefix(group, "('") {
		t.Fatalf("the scoring kind group renders as %q, which no statement can contain — this census would qualify nothing", group)
	}
	projections := interactionProjectionTables(t)

	var viaGroup, viaProjection int
	for _, pkg := range goPackagesUnder(t, ".") {
		consts := relstrengthConsts(t, pkg, group)
		for _, file := range pkg.files {
			for _, stmt := range sqlStatementsWith(t, file, consts) {
				if !strings.Contains(strings.ToLower(stmt), "count(") {
					continue
				}
				byGroup := strings.Contains(stmt, group)
				byProjection := namesAny(stmt, projections)
				if !byGroup && !byProjection {
					continue
				}
				if byGroup {
					viaGroup++
				}
				if byProjection {
					viaProjection++
				}
				judgeCounts(t, file.relPath, stmt)
			}
		}
	}

	// BOTH arms live, or the census is judging half the tree and reporting a
	// clean one over the other half. The workspace score is reached through the
	// kind group and the colleague edge through the projection table, and
	// neither arm can see the other's statements.
	if viaGroup == 0 {
		t.Error("no statement counts under the scoring kind group — the arm that reaches the workspace score sees nothing")
	}
	if viaProjection == 0 {
		t.Error("no statement counts over an interaction projection — the arm that reaches the colleague and contact edges sees nothing")
	}
}

// judgeCounts reports each count in one qualifying statement that reads
// something other than the shared unit.
func judgeCounts(t *testing.T, relPath, stmt string) {
	t.Helper()
	for _, found := range countsIn(stmt) {
		if strings.HasPrefix(found, countsTheSharedUnit) {
			continue
		}
		key := relPath + " :: " + found
		if deliberateOtherCounts.Waived(t, key) {
			continue
		}
		t.Errorf("%s counts %s — a relationship count reads relstrength.InteractionUnitSQL, or a day of chat outscores a quarter of meetings. If this counts something other than interactions, say what in deliberateOtherCounts under the key %q", relPath, found, key)
	}
}

// countsIn returns each count in the statement as its expression plus enough of
// what follows to tell two counts in one statement apart — the form a waiver is
// keyed on, whitespace collapsed so re-wrapping a long line does not move it.
func countsIn(stmt string) []string {
	var out []string
	lower := strings.ToLower(stmt)
	for at := 0; ; {
		found := strings.Index(lower[at:], "count(")
		if found < 0 {
			return out
		}
		start := at + found
		end := start + len(countTail(stmt[start:]))
		out = append(out, collapse(stmt[start:end]))
		at = start + len("count(")
	}
}

// countTail is the count call plus the following twenty characters: the call
// alone does not separate two `count(*)`s in one statement, and the whole
// statement as a key would move every time a line beside it changed.
func countTail(from string) string {
	depth := 0
	for i, r := range from {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end := i + 21
				if end > len(from) {
					end = len(from)
				}
				return from[:end]
			}
		}
	}
	return from
}

var whitespace = regexp.MustCompile(`\s+`)

func collapse(text string) string { return whitespace.ReplaceAllString(strings.TrimSpace(text), " ") }

func namesAny(stmt string, tables []string) bool {
	for _, table := range tables {
		if strings.Contains(stmt, table) {
			return true
		}
	}
	return false
}

// interactionProjectionTables reads the tables that STORE an interaction count
// out of the schema, rather than naming them here. A third projection added
// beside the colleague and contact edges is then judged without anybody
// remembering to add it, which is the whole reason this is derived: a census
// carrying its own list of subjects grows only when somebody grows it, and a
// missed subject reports PASS.
//
// It reads the head CATALOG and not the migrations, because the catalog is the
// schema as of head in one file. A reader pointed at the baseline sees only the
// tables that shipped in it, and a projection introduced by a later migration
// is then invisible to a census that says nothing about it.
func interactionProjectionTables(t *testing.T) []string {
	t.Helper()
	const catalog = "migrations/testdata/head_catalog.txt"
	schema, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatalf("reading the schema head: %v", err)
	}
	var tables []string
	for _, line := range strings.Split(string(schema), "\n") {
		qualified, _, isColumn := strings.Cut(line, " ")
		if !isColumn || !strings.HasSuffix(qualified, ".count_90d") {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(qualified, ".count_90d"), ".")
		tables = append(tables, parts[len(parts)-1])
	}
	if len(tables) == 0 {
		t.Fatal("no table at schema head stores a count_90d — either the projections moved or this reader stopped seeing them, and either way the census below judges nothing")
	}
	return tables
}

// relstrengthConsts resolves the package's own names for relstrength's
// renderers, so a statement that interpolates them folds to text this census
// can read.
//
// Resolved per PACKAGE and not per file: `graphInteractionUnit` is declared in
// one file of `search` and read by both, and a per-file reading would see the
// second file's counts as interpolating something it cannot name — which reads
// exactly like the violation it would then report.
func relstrengthConsts(t *testing.T, pkg goPackage, group string) map[string]string {
	t.Helper()
	consts := map[string]string{}
	for _, file := range pkg.files {
		if !gatekit.References(file.ast, relstrengthImport, "InteractionUnitSQL") &&
			!gatekit.References(file.ast, relstrengthImport, "InteractionKindSQLGroup") {
			continue
		}
		ast.Inspect(file.ast, func(node ast.Node) bool {
			spec, isSpec := node.(*ast.ValueSpec)
			if !isSpec || len(spec.Names) != 1 || len(spec.Values) != 1 {
				return true
			}
			call, isCall := spec.Values[0].(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			switch selector.Sel.Name {
			case "InteractionUnitSQL":
				consts[spec.Names[0].Name] = unitSentinel
			case "InteractionKindSQLGroup":
				consts[spec.Names[0].Name] = group
			}
			return true
		})
	}
	return consts
}

const relstrengthImport = "github.com/margince/margince/backend/internal/shared/kernel/relstrength"

// sqlStatementsWith is gatekit.SQLStatementsOf with the package's relstrength
// names resolved — one entry per statement, flattened across the `+` chains the
// folds are written as.
func sqlStatementsWith(t *testing.T, file goFile, consts map[string]string) []string {
	t.Helper()
	var out []string
	ast.Inspect(file.ast, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op != token.ADD {
				return true
			}
			if text, isString := gatekit.StringExpr(typed, consts, gatekit.FoldTotal); isString {
				out = append(out, text)
				return false
			}
		case *ast.BasicLit:
			if text, isString := gatekit.LiteralText(typed); isString {
				out = append(out, text)
			}
		}
		return true
	})
	return out
}

type goFile struct {
	relPath string
	ast     *ast.File
}

type goPackage struct {
	files []goFile
}

// goPackagesUnder parses every non-test Go file in the module, grouped by
// directory. The generated contract is skipped: it holds no statement, and
// parsing it costs more than every other file together.
func goPackagesUnder(t *testing.T, root string) []goPackage {
	t.Helper()
	byDir := map[string][]goFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/internal/contracts/") {
			return err
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		dir := filepath.Dir(path)
		byDir[dir] = append(byDir[dir], goFile{relPath: filepath.ToSlash(path), ast: parsed})
		return nil
	})
	if err != nil {
		t.Fatalf("reading the tree: %v", err)
	}
	if len(byDir) == 0 {
		t.Fatal("no Go package under the module — this census would judge nothing")
	}
	out := make([]goPackage, 0, len(byDir))
	for _, files := range byDir {
		out = append(out, goPackage{files: files})
	}
	return out
}
