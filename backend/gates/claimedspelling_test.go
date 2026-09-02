// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A constant whose doc comment says it is spelled once is making a checkable
// statement, and until now nothing checked it. The failure it describes is a
// second WRITER: a new audit image, event delta or clause that types the value
// instead of taking the name, after which the two sites the comment promised
// would agree drift on the next rename of either.
//
// Scope is derived, not declared. A claim's blast radius is the set of files
// that USE the constant — those are the sites the comment is about, and they
// are the sites a raw spelling would sit beside. Reading the prose for a scope
// ("the three merges", "the two refusal sites") would make this gate a second
// reading of the same sentence.
//
// Non-test sources only. A literal in a test is the test naming a value on
// purpose, and a test written through the constant proves LESS: it would agree
// with the code about a value neither of them spells correctly.
//
// The corpus is the claims themselves, so this gate cannot drift from what the
// register describes. Claims already bound to another gate are out: their own
// gate is what holds them, and re-judging them here would make binding a claim
// depend on satisfying a rule its author never wrote.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// thisGate is the name a claim writes in `Held by:` to be judged here. Spelled
// as the constant the test function is named after, so a rename moves both.
const thisGate = "TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed"

// driftShape is the claim shape this gate judges: a comment promising that two
// sites stay in step is promising they read one name, which is what a raw
// spelling breaks. The other shapes make different promises — `once` is about a
// declaration rather than a value — and each one that joins this corpus brings
// its own findings to read and rule on, so widening is its own piece of work
// and not a line change here.
//
// The key is checked against the detector rather than assumed, so a shape that
// is renamed or retired empties this gate loudly instead of quietly.
const driftShape = "cannot-drift"

// sharedSpellings ratifies a value whose second appearance is a DIFFERENT
// subject that happens to be spelled the same way. Keyed by the constant, never
// by the file: one file can hold two claims, and a file-keyed entry would
// ratify the one nobody read.
//
// Each entry says what the other spelling IS. That is the whole judgement — a
// reader checking this entry later has to be able to tell "different subject"
// from "second writer", and only naming the other subject does that.
var sharedSpellings = gatekit.Waive(map[string]string{
	"internal/modules/ai/feedback.go:fieldSubjectType":           "the second is the same word as a key in the AuditEvent image, where it names a column of the ledger row rather than the request field a refusal points at",
	"internal/modules/search/queryjoins.go:objectRelationship":   "the second is the relationship TABLE's name in the join spec beside it, not the RBAC object governing it — the two agree today and are free to diverge",
	"internal/shared/ports/datasource/shapewords.go:objectShape": "the second is the same two words inside a rendered sentence, where they are English rather than the shape word this constant names",
	"internal/modules/customfields/engine.go:fieldObject":        "the second is the English word in structuralKeywords, which a user's LABEL is smell-tested against — \"add an object for invoices\" is refused as a request to model something new. It is a word somebody typed, not the request field this constant names, and tying the two would make the heuristic's vocabulary follow the wire.",
})

// claimedConst is one string constant a claim sits on.
type claimedConst struct {
	claimPath string // the file the claim was found in, as the register keys it
	dir       string // the package directory the scope is swept over
	name      string
	value     string
}

func TestAClaimedSpellingIsTheOnlySpellingWhereItIsUsed(t *testing.T) {
	t.Parallel()
	if claimShapes[driftShape] == nil {
		t.Fatalf("the detector no longer declares the %q shape, so this gate's corpus is empty and "+
			"every claim it used to judge is unjudged", driftShape)
	}
	consts := claimedConsts(t)
	if len(consts) == 0 {
		t.Fatal("no claimed string constant found — the corpus derivation has broken, and a " +
			"gate with an empty corpus reads exactly like a tree with nothing to fix")
	}
	for _, c := range consts {
		key := fmt.Sprintf("%s:%s", c.claimPath, c.name)
		raw := rawSpellingsOf(t, c)
		if len(raw) == 0 {
			if sharedSpellings.Waived(t, key) {
				t.Errorf("%s %s is ratified as sharing its spelling with another subject and no "+
					"second spelling remains. A waiver over a value spelled once ratifies nothing "+
					"and hides the next raw one: drop the entry.", c.claimPath, c.name)
			}
			continue
		}
		if sharedSpellings.Waived(t, key) {
			continue
		}
		t.Errorf("%s %s claims one spelling of %q and the files that use it also spell it raw, at "+
			"%s.\n\nThe comment promises those sites cannot drift; a raw spelling is how they do — "+
			"the next rename moves the constant and leaves the literal behind. Take the constant "+
			"at that site, or, if the second is a different subject wearing the same word, ratify "+
			"it in sharedSpellings and say what that subject is.",
			c.claimPath, c.name, c.value, strings.Join(raw, ", "))
	}
	sharedSpellings.AssertAllMatched(t)
}

// claimedConsts reads every claim sitting on a const declaration and returns
// the string values it binds.
func claimedConsts(t *testing.T) []claimedConst {
	t.Helper()
	seen := map[string]bool{}
	var out []claimedConst
	fset := token.NewFileSet()
	for _, c := range allClaims(t) {
		if c.shape != driftShape || !strings.Contains(c.decl, "const") ||
			(c.held != "" && c.held != thisGate) {
			continue
		}
		file, err := parser.ParseFile(fset, c.path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", c.path, err)
		}
		for name, value := range constValuesAt(fset, file, c.line) {
			// One const block can match two claim shapes. The subject is the
			// constant, so judging it twice reports one finding twice.
			if key := c.path + ":" + name; !seen[key] {
				seen[key] = true
				out = append(out, claimedConst{
					claimPath: c.path, dir: filepath.Dir(c.path), name: name, value: value,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].claimPath+out[i].name < out[j].claimPath+out[j].name
	})
	return out
}

// constValuesAt returns the string constants the declaration at line binds.
//
// The line the claim carries is the doc comment's, not the keyword's, so the
// declaration is matched by the span that INCLUDES its doc — a claim sits on
// the comment, and the comment is what names the constant below it.
func constValuesAt(fset *token.FileSet, file *ast.File, line int) map[string]string {
	values := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		start := fset.Position(gen.Pos()).Line
		if gen.Doc != nil {
			start = fset.Position(gen.Doc.Pos()).Line
		}
		if line < start || line > fset.Position(gen.End()).Line {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING || i >= len(vs.Names) {
					continue
				}
				if text, err := strconv.Unquote(lit.Value); err == nil && text != "" {
					values[vs.Names[i].Name] = text
				}
			}
		}
	}
	return values
}

// rawSpellingsOf reports every string literal equal to the constant's value in
// the non-test files of its package that reference it, other than the
// declaration's own.
func rawSpellingsOf(t *testing.T, c claimedConst) []string {
	t.Helper()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		t.Fatalf("reading %s: %v", c.dir, err)
	}
	var raw []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(c.dir, entry.Name())
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		if !mentions(file, c.name) {
			continue
		}
		raw = append(raw, rawLiteralsIn(fset, file, c)...)
	}
	sort.Strings(raw)
	return raw
}

func mentions(file *ast.File, name string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// rawLiteralsIn reports the raw spellings in one file.
//
// Every declared value is skipped, not just this constant's own: a sibling
// constant binding the same string is a second NAME for the value, which is a
// different finding with a different fix, and reporting it here would send the
// reader to replace a constant with itself.
func rawLiteralsIn(fset *token.FileSet, file *ast.File, c claimedConst) []string {
	declared := map[ast.Expr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for _, value := range vs.Values {
			declared[value] = true
		}
		return true
	})
	var raw []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || declared[ast.Expr(lit)] {
			return true
		}
		if text, err := strconv.Unquote(lit.Value); err == nil && text == c.value {
			raw = append(raw, fmt.Sprint(fset.Position(lit.Pos())))
		}
		return true
	})
	return raw
}
