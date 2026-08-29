// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every end a link can have is an end that link's history is read from.
//
// A record's history includes the links made and removed on it, and `relationship`
// carries FIVE endpoint columns for SEVEN kinds. privacy.edgeEndpoints is the read's
// whole knowledge of that shape, and the failure mode of forgetting a column is
// precise rather than diffuse: a co-sell edge sits in organization_id for one company
// and counterparty_org_id for the other, so a read that knows only the first shows
// the edge on ONE of the two companies. Nothing errors. The company that made the
// link has no line saying so, and the company that didn't has one.
//
// So the columns are derived from the table's own shape CHECK constraints — the
// declarations that say which ends each kind must have — and the read is required to
// name every one of them.
//
// The derivation reads the HIGHEST-numbered migration restating each constraint, not
// the baseline. rel_project_company_shape exists only in 1787450422, which also
// restates the kind check; a census reading 0001_baseline alone under-reads, reports
// PASS, and leaves no failing assertion to notice — the one way AGENTS.md §8 says a
// census must not break. The two floors below are the guard against the other shape
// of the same failure: a parse that stops matching finds nothing to object to and
// reads exactly like a clean tree.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	// edgeReadSource holds the read's endpoint knowledge, and edgeEndpointsDecl is
	// the declaration in it. Named as constants because the failure message has to
	// tell a contributor where to go, and a message naming the wrong file is worse
	// than no message.
	edgeReadSource    = "internal/modules/privacy/edgehistory.go"
	edgeEndpointsDecl = "edgeEndpoints"
	// edgeEndpointColumnField is the struct field inside that declaration holding
	// the column name. The census reads THIS and not the file's text, because
	// "counterparty_org_id" also appears in the file's prose — a substring match
	// would pass a tree whose read had dropped the column and kept the comment.
	edgeEndpointColumnField = "column"
	// coreMigrations is where the shape constraints are declared and restated.
	coreMigrations = "migrations/core"
	// edgeShapeConstraintPrefix and Suffix bracket the constraint names that
	// declare which ends a kind must have (rel_employment_shape,
	// rel_project_company_shape, …). Matched by name because that is what a
	// migration restating one has to repeat.
	edgeShapeConstraintPrefix = "rel_"
	edgeShapeConstraintSuffix = "_shape"
	// minEdgeShapeConstraints and minEdgeEndpointColumns are FLOORS at what the
	// schema holds today: five shape constraints naming five columns. They exist so
	// a parse regression fails loudly instead of certifying an empty corpus — and
	// the constraint floor is the specific guard against reading 0001_baseline
	// alone, which declares exactly four of the five.
	minEdgeShapeConstraints = 5
	minEdgeEndpointColumns  = 5
)

func TestEveryEndpointColumnRelationshipHasIsReadFromBothEnds(t *testing.T) {
	t.Parallel()
	declared, constraints := endpointColumnsFromShapeConstraints(t)
	if len(constraints) < minEdgeShapeConstraints {
		t.Fatalf("parsed only %d shape constraint(s) out of %s, want at least %d — the parse has stopped "+
			"seeing the declarations this census is about, and an empty corpus reads as a clean tree",
			len(constraints), coreMigrations, minEdgeShapeConstraints)
	}
	if len(declared) < minEdgeEndpointColumns {
		t.Fatalf("the shape constraints %v name only %d endpoint column(s) (%v), want at least %d",
			constraints, len(declared), declared, minEdgeEndpointColumns)
	}

	read := endpointColumnsInTheRead(t)
	if len(read) == 0 {
		t.Fatalf("%s declares no %s entry the census could read — has %s been renamed or moved?",
			edgeReadSource, edgeEndpointsDecl, edgeEndpointsDecl)
	}
	for _, column := range declared {
		if !read[column] {
			t.Errorf("relationship.%s is an endpoint the table's shape constraints declare, and "+
				"%s.%s does not name it. A record sitting in that column has no link history at all, and "+
				"a link whose OTHER end is in it appears on one of the two records instead of both — with "+
				"nothing failing to say so. Add {column: %q, entityType: …, labelColumn: …} to %s.",
				column, edgeReadSource, edgeEndpointsDecl, column, edgeEndpointsDecl)
		}
	}
	// The read may know MORE columns than the constraints declare (a column no kind
	// requires yet), so the reverse direction is not an error. What it may not do is
	// name a column the table does not have, which the SQL itself would refuse.
}

// endpointColumnsFromShapeConstraints answers the endpoint columns relationship's
// shape CHECKs declare, plus the constraint names it read them from.
//
// Per CONSTRAINT and last-wins: the vocabulary grows by dropping a CHECK and re-adding
// a wider one, so the effective body of each constraint is whatever the highest-numbered
// migration naming it says. core's zero-padded sequence sorts below the later
// unix-second stamps, which makes lexical order migration order.
func endpointColumnsFromShapeConstraints(t *testing.T) ([]string, []string) {
	t.Helper()
	bodies := map[string]string{}
	for _, path := range sortedMigrationsUp(t) {
		raw, err := os.ReadFile(path) // #nosec G304 -- a *.up.sql name from the trusted migrations tree, test-only
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for name, body := range shapeCheckBodies(t, path, string(raw)) {
			bodies[name] = body
		}
	}
	columns := map[string]bool{}
	names := make([]string, 0, len(bodies))
	for name, body := range bodies {
		names = append(names, name)
		for _, column := range endpointIdentifiers(body) {
			columns[column] = true
		}
	}
	sort.Strings(names)
	return sortedEndpointColumns(columns), names
}

// shapeCheckBodies extracts the CHECK expression of every rel_*_shape constraint the
// text declares, keyed by constraint name. The body is taken by BALANCED parentheses
// from the CHECK keyword, because the two forms in the tree disagree about layout: the
// baseline states each constraint on one line inside CREATE TABLE, and a later
// migration states one across several lines in an ALTER TABLE.
func shapeCheckBodies(t *testing.T, path, text string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for cursor := 0; ; {
		start := strings.Index(text[cursor:], edgeShapeConstraintPrefix)
		if start == -1 {
			return out
		}
		start += cursor
		cursor = start + len(edgeShapeConstraintPrefix)
		name, rest, found := constraintNameAt(text[start:])
		if !found {
			continue
		}
		check := strings.Index(rest, "(")
		if !strings.HasPrefix(strings.TrimSpace(rest), "CHECK") || check == -1 {
			continue
		}
		body, closed := balancedParens(rest[check:])
		if !closed {
			t.Fatalf("%s: the CHECK body of %s never closes its parenthesis", path, name)
		}
		out[name] = body
		cursor = start + len(name)
	}
}

// constraintNameAt reads a rel_*_shape constraint name at the head of text, answering
// it and whatever follows. It is how the scan tells a constraint DECLARATION from a
// mention of the same prefix in prose (rel_dates, or a comment naming one).
func constraintNameAt(text string) (name, rest string, found bool) {
	end := 0
	for end < len(text) && (text[end] == '_' || (text[end] >= 'a' && text[end] <= 'z')) {
		end++
	}
	name = text[:end]
	if !strings.HasSuffix(name, edgeShapeConstraintSuffix) {
		return "", "", false
	}
	return name, text[end:], true
}

// balancedParens returns the contents of the parenthesised group starting at text[0],
// brackets included, and whether it closed. String literals inside a CHECK are
// single-quoted and hold no parentheses in this schema, so depth counting is enough.
func balancedParens(text string) (string, bool) {
	depth := 0
	for i, c := range text {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[:i+1], true
			}
		}
	}
	return "", false
}

// endpointIdentifiers pulls the *_id column names out of a CHECK body. A shape
// constraint names the kind column and the endpoints and nothing else, so the `_id`
// suffix is the whole discriminator — no list of expected columns, which is what would
// make this census a second copy of the thing it protects.
func endpointIdentifiers(body string) []string {
	var out []string
	word := strings.Builder{}
	flush := func() {
		if strings.HasSuffix(word.String(), "_id") {
			out = append(out, word.String())
		}
		word.Reset()
	}
	for _, c := range body {
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			word.WriteRune(c)
			continue
		}
		flush()
	}
	flush()
	return out
}

// endpointColumnsInTheRead parses the read's endpoint declaration and answers the
// column names it holds. AST rather than text: the file's prose names these columns
// too, and a census satisfied by a comment is a census that passes a broken read.
func endpointColumnsInTheRead(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, edgeReadSource, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", edgeReadSource, err)
	}
	out := map[string]bool{}
	for _, decl := range file.Decls {
		spec, named := endpointDeclSpec(decl)
		if !named {
			continue
		}
		ast.Inspect(spec, func(node ast.Node) bool {
			kv, isPair := node.(*ast.KeyValueExpr)
			if !isPair {
				return true
			}
			key, isIdent := kv.Key.(*ast.Ident)
			lit, isLit := kv.Value.(*ast.BasicLit)
			if !isIdent || !isLit || key.Name != edgeEndpointColumnField || lit.Kind != token.STRING {
				return true
			}
			column, unquoteErr := strconv.Unquote(lit.Value)
			if unquoteErr != nil {
				t.Fatalf("%s: %s value %s is not a readable string: %v",
					edgeReadSource, edgeEndpointColumnField, lit.Value, unquoteErr)
			}
			out[column] = true
			return true
		})
	}
	return out
}

// endpointDeclSpec finds the var spec the census reads, and reports whether this
// declaration is it.
func endpointDeclSpec(decl ast.Decl) (*ast.ValueSpec, bool) {
	gen, isGen := decl.(*ast.GenDecl)
	if !isGen || gen.Tok != token.VAR {
		return nil, false
	}
	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for _, name := range value.Names {
			if name.Name == edgeEndpointsDecl {
				return value, true
			}
		}
	}
	return nil, false
}

// sortedMigrationsUp lists every applied-direction core migration in migration order.
func sortedMigrationsUp(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(coreMigrations, "*.up.sql"))
	if err != nil {
		t.Fatalf("listing %s: %v", coreMigrations, err)
	}
	if len(matches) == 0 {
		t.Fatalf("no *.up.sql under %s — the census is reading the wrong directory", coreMigrations)
	}
	sort.Strings(matches)
	return matches
}

// sortedEndpointColumns spells the derived column set in a stable order, so the
// failure list reads the same on every run.
func sortedEndpointColumns(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// The EXPORT of relationships tests the same endpoints the history read does.
//
// compose.relationshipExportScope carries the same obligation as the read —
// "every non-null endpoint must be visible, so an edge never discloses a record
// on the far side the caller cannot see" — and it carries it as a hand-written
// list. A column the table has and that list omits is an edge exported without
// its far side being tested, which is the disclosure its own comment says
// cannot happen. Two surfaces answering one question, so the corpus is derived
// once and held against both.
const (
	edgeExportSource = "internal/compose/export_scope.go"
	edgeExportFunc   = "relationshipExportScope"
)

func TestTheRelationshipExportTestsEveryEndpointTheTableHas(t *testing.T) {
	t.Parallel()
	declared, _ := endpointColumnsFromShapeConstraints(t)
	if len(declared) < minEdgeEndpointColumns {
		t.Fatalf("the shape constraints name only %d endpoint column(s) (%v), want at least %d — "+
			"an empty corpus certifies a clean tree", len(declared), declared, minEdgeEndpointColumns)
	}
	scoped := endpointColumnsInTheExport(t)
	if len(scoped) == 0 {
		t.Fatalf("%s declares no endpoint list the census could read inside %s — renamed or moved?",
			edgeExportSource, edgeExportFunc)
	}
	for _, column := range declared {
		if !scoped[column] {
			t.Errorf("relationship.%s is an endpoint the table's shape constraints declare, and %s's "+
				"%s does not test it. An edge whose far side sits in that column is exported without "+
				"the caller's visibility of that record being checked — the disclosure that function's "+
				"own comment says it prevents. Add {%q, <its table>} to the endpoint list.",
				column, edgeExportSource, edgeExportFunc, column)
		}
	}
}

// endpointColumnsInTheExport reads the FIRST string of each element in the
// endpoint list inside relationshipExportScope. The elements are unkeyed
// (`{"person_id", "person"}`), so there is no field name to match on — position
// is the contract, and the column is first.
func endpointColumnsInTheExport(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, edgeExportSource, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", edgeExportSource, err)
	}
	out := map[string]bool{}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Name == nil || fn.Name.Name != edgeExportFunc {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			lit, isComposite := node.(*ast.CompositeLit)
			if !isComposite || len(lit.Elts) == 0 {
				return true
			}
			for _, elt := range lit.Elts {
				pair, isPair := elt.(*ast.CompositeLit)
				if !isPair || len(pair.Elts) == 0 {
					continue
				}
				first, isLit := pair.Elts[0].(*ast.BasicLit)
				if !isLit || first.Kind != token.STRING {
					continue
				}
				column, unquoteErr := strconv.Unquote(first.Value)
				if unquoteErr != nil {
					t.Fatalf("%s: %s is not a readable string: %v", edgeExportSource, first.Value, unquoteErr)
				}
				out[column] = true
			}
			return true
		})
	}
	return out
}

// The ROW SCOPE of relationships tests the same endpoints the read and the
// export do.
//
// auth.RelationshipEndpointScope is the conjunction every list and probe of the
// edge table applies — "every non-null endpoint must be visible" — and it too
// carries the endpoints as a hand-written list. A column the table has and that
// list omits is an edge row-scoped on one end only: the far side's record is
// disclosed to a caller whose scope excludes it, which is precisely the
// disclosure the conjunction exists to refuse. Three surfaces answering one
// question; the corpus is derived once and held against all three.
const (
	edgeScopeSource = "internal/platform/auth/inheritedscope.go"
	edgeScopeDecl   = "relationshipEndpointColumns"
)

func TestTheRelationshipRowScopeTestsEveryEndpointTheTableHas(t *testing.T) {
	t.Parallel()
	declared, _ := endpointColumnsFromShapeConstraints(t)
	if len(declared) < minEdgeEndpointColumns {
		t.Fatalf("the shape constraints name only %d endpoint column(s) (%v), want at least %d — "+
			"an empty corpus certifies a clean tree", len(declared), declared, minEdgeEndpointColumns)
	}
	scoped := endpointColumnsInScopeVar(t)
	if len(scoped) == 0 {
		t.Fatalf("%s declares no %s list the census could read — renamed or moved?",
			edgeScopeSource, edgeScopeDecl)
	}
	for _, column := range declared {
		if !scoped[column] {
			t.Errorf("relationship.%s is an endpoint the table's shape constraints declare, and %s's "+
				"%s does not scope it. An edge whose far side sits in that column is row-scoped on one "+
				"end only, so the far record is disclosed to a caller whose scope excludes it. "+
				"Add {%q, <its table>} to the list.",
				column, edgeScopeSource, edgeScopeDecl, column)
		}
	}
}

// endpointColumnsInScopeVar reads the FIRST string of each element of the
// package-level relationshipEndpointColumns literal — position is the
// contract there exactly as it is in the export's list.
func endpointColumnsInScopeVar(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, edgeScopeSource, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", edgeScopeSource, err)
	}
	out := map[string]bool{}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) == 0 || value.Names[0].Name != edgeScopeDecl {
				continue
			}
			for _, v := range value.Values {
				ast.Inspect(v, func(node ast.Node) bool {
					pair, isPair := node.(*ast.CompositeLit)
					if !isPair || len(pair.Elts) == 0 {
						return true
					}
					first, isLit := pair.Elts[0].(*ast.BasicLit)
					if !isLit || first.Kind != token.STRING {
						return true
					}
					column, unquoteErr := strconv.Unquote(first.Value)
					if unquoteErr != nil {
						t.Fatalf("%s: %s is not a readable string: %v", edgeScopeSource, first.Value, unquoteErr)
					}
					out[column] = true
					return true
				})
			}
		}
	}
	return out
}
