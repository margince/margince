// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// Every reference an exported row carries to a row-scoped record is one the
// export withholds from a reader who could not open it.
//
// The pairs live in compose (referencesByTable) and this derives the SAME set
// from the schema's own foreign keys, so the two cannot drift. A column added
// next year that names an organization is covered the day its FK is declared,
// rather than the day somebody remembers the export.
//
// UNDER-RECOGNITION is the whole risk here. A missing pair is not a failing
// test anywhere: the export keeps working, the column keeps going out, and what
// leaves is the id of a record the reader's own list would have withheld. There
// is nothing to notice, which is why this is derived rather than reviewed.
//
// BETWEEN row-scoped tables only. A column naming a table every reader may read
// discloses nothing by naming it, and withholding one would blank a field for
// no reason — the availability regression that wears a security fix's clothes.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// foreignKeyToRowScoped matches a catalog line declaring one table's FK to
// another: `public.<table>.<name> FOREIGN KEY (<column>) REFERENCES <target>(id)`.
var foreignKeyToRowScoped = regexp.MustCompile(
	`^public\.(\w+)\.\w+ FOREIGN KEY \((\w+)\) REFERENCES (\w+)\(id\)`)

func TestEveryExportedReferenceToARowScopedTableIsWithheld(t *testing.T) {
	t.Parallel()

	scoped := rowScopedTables(t)
	if len(scoped) < 5 {
		t.Fatalf("read %d row-scoped tables from auth — far fewer than this product has, so a green result here would mean nothing", len(scoped))
	}

	want := map[string][]string{}
	catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	for _, line := range strings.Split(string(catalog), "\n") {
		m := foreignKeyToRowScoped.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		table, column, target := m[1], m[2], m[3]
		if !scoped[table] || !scoped[target] {
			continue
		}
		want[table] = append(want[table], column+"->"+target)
	}
	if len(want) == 0 {
		t.Fatal("the catalog declared no foreign key between two row-scoped tables — the scan read the wrong file or the wrong shape, and an empty expectation passes against anything")
	}

	got := declaredExportReferences(t)
	for table, columns := range want {
		sort.Strings(columns)
		declared := got[table]
		sort.Strings(declared)
		if !slices.Equal(columns, declared) {
			t.Errorf("compose's referencesByTable[%q] = %v, want %v — a reference the export does not withhold leaves the id of a record this reader's own list would have withheld, and nothing fails to say so",
				table, declared, columns)
		}
	}
	for table := range got {
		if _, expected := want[table]; !expected {
			t.Errorf("compose's referencesByTable declares %q, which the catalog gives no row-scoped reference — withholding a column for no reason blanks a field a reader is entitled to", table)
		}
	}
}

// declaredExportReferences reads compose's pairs as `column->target` strings,
// the same spelling the catalog side is rendered in.
func declaredExportReferences(t *testing.T) map[string][]string {
	t.Helper()
	const declaration = "internal/compose/exportreferences.go"
	file, err := parser.ParseFile(token.NewFileSet(), declaration, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", declaration, err)
	}
	constants := composeStringConstants(t)
	out := map[string][]string{}
	for _, spec := range valueSpecsNamed(file, "referencesByTable") {
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatalf("referencesByTable is not a composite literal; this gate can only read one")
		}
		for _, entry := range lit.Elts {
			kv, ok := entry.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			table := stringValue(t, constants, kv.Key)
			refs, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				t.Fatalf("referencesByTable[%q] is not a list", table)
			}
			for _, r := range refs.Elts {
				ref, ok := r.(*ast.CompositeLit)
				if !ok {
					continue
				}
				var column, target string
				for _, f := range ref.Elts {
					fkv, ok := f.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					name, _ := fkv.Key.(*ast.Ident)
					switch {
					case name == nil:
					case name.Name == "column":
						column = stringValue(t, constants, fkv.Value)
					case name.Name == "target":
						target = stringValue(t, constants, fkv.Value)
					}
				}
				out[table] = append(out[table], column+"->"+target)
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s declares no referencesByTable entries — the export withholds nothing and this gate read nothing", declaration)
	}
	return out
}

func valueSpecsNamed(file *ast.File, name string) []*ast.ValueSpec {
	var out []*ast.ValueSpec
	for _, decl := range file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if ok && len(value.Names) == 1 && value.Names[0].Name == name && len(value.Values) == 1 {
				out = append(out, value)
			}
		}
	}
	return out
}

// composeStringConstants reads every string constant the compose package
// declares. referencesByTable names its tables and columns through those
// constants rather than repeating literals, and the constants are spread over
// the files that own each vocabulary — so this reads the package, not one file.
func composeStringConstants(t *testing.T) map[string]string {
	t.Helper()
	sources, err := filepath.Glob("internal/compose/*.go")
	if err != nil {
		t.Fatalf("listing compose sources: %v", err)
	}
	out := map[string]string{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				if lit, ok := value.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					unquoted, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("unquoting %s in %s: %v", lit.Value, source, err)
					}
					out[value.Names[0].Name] = unquoted
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("compose declares no string constants — the scan read the wrong directory, and every name would then resolve to nothing")
	}
	return out
}

// stringValue reads a literal, or the value of the constant an identifier names.
func stringValue(t *testing.T, constants map[string]string, e ast.Expr) string {
	t.Helper()
	switch node := e.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			t.Fatalf("expected a string literal, got %s", node.Kind)
		}
		v, err := strconv.Unquote(node.Value)
		if err != nil {
			t.Fatalf("unquoting %s: %v", node.Value, err)
		}
		return v
	case *ast.Ident:
		v, declared := constants[node.Name]
		if !declared {
			t.Fatalf("referencesByTable names %s, which compose declares no string constant for", node.Name)
		}
		return v
	default:
		t.Fatalf("expected a string literal or a constant name, got %T", e)
		return ""
	}
}

// The literal-scanning census cannot see these reads at all. Every OTHER
// compose reader of a row-scoped reference names its column in a SQL string,
// which is how TestEveryComposeReadOfARecordReferenceAppliesItsRowScope finds
// its sites; the export builds its select list from the live catalog, so
// export.go, filteredexport.go and filterpreview.go contain no such literal and
// were never subjects of it. Not waived there — invisible to it, which is the
// failure mode that reports PASS.
//
// So the class gets its own census, keyed on the mechanism that creates the
// blind spot rather than on the three functions that have it today: a read
// whose columns come from the catalog serves whatever the catalog holds,
// including a column added next year by a migration nobody read.
func TestEveryCatalogColumnedReadWithholdsWhatItCannotShow(t *testing.T) {
	t.Parallel()

	const (
		catalogSource = "exportableColumns"
		withholding   = "withholdUnreadableReferences"
	)
	graph := packageCallGraph(t, "internal/compose")
	if _, declared := graph[withholding]; !declared {
		t.Fatalf("compose declares no %s — this gate's obligation does not exist, so every subject would pass it", withholding)
	}

	var roots []string
	for fn, entry := range graph {
		if fn != catalogSource && entry.calls[catalogSource] {
			roots = append(roots, fn)
		}
	}
	if len(roots) == 0 {
		t.Fatalf("no compose function calls %s — the call graph read nothing, and an empty census passes against anything", catalogSource)
	}
	sort.Strings(roots)

	for _, root := range roots {
		if reaches(graph, root, withholding) {
			continue
		}
		t.Errorf("compose's %s takes its columns from the catalog but never reaches %s — it serves whatever the catalog holds, so a reference column added by a later migration leaves without anyone deciding it should",
			root, withholding)
	}
}
