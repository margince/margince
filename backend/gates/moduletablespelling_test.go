// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A package that names its table does not pass that name as a bare string.
//
// A store hands its table to storekit and to auth as a plain string. The word
// is usually the RBAC object as well, and often a wire field, so one spelling
// ends up doing three jobs with nothing at a given site saying which was meant.
// Naming the table — `dealTable`, `contractTable` — separates the subjects.
//
// What this gate holds is narrower than that convention, and the difference
// matters: it reads the table ARGUMENTS of the calls tableAndObjectArgs names,
// and nothing else. A table spelled inside SQL text is out of view on purpose
// (tableownership_test.go reads write targets out of that text, so a
// constant there would take the statement out of the ownership census), and so
// is any call family absent from that map.
//
// Two shapes fail. A bare literal beside a constant that already names it: the
// reader sees the constant, assumes the sites follow it, and the rename leaves
// the literals behind. And a SWAP — the package's other constant holding the
// same word, passed where the table belongs, or the table's passed where the
// RBAC object belongs. Nothing else can see that one: both are untyped strings
// of one value, so the exchange compiles, type-checks and reads correctly until
// the day one of the names moves.
//
// The rule binds only packages that OPTED IN by declaring the constant.
// Deliberately not "every package must declare one": a table named at a single
// site is not a duplication, and demanding the constant everywhere would invent
// work rather than hold an invariant somebody chose. Scope is this module —
// extensions/ declares table constants of its own, built by concatenation, and
// both this sweep and its literal reader stop at the module root.

import (
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// tableConstName matches a constant whose name declares it to BE a table:
// `dealTable`, `contractTable`. The suffix is the opt-in.
var tableConstName = regexp.MustCompile(`^[a-z][A-Za-z0-9]*Table$`)

// authorityPackages define the calls this gate reads. They are parsed rather
// than listed, because the distinction the gate turns on — is this argument a
// TABLE or an RBAC OBJECT — is one these packages already draw, in the names
// they give their own parameters.
//
// A hand-kept list was short by nine calls the day it was written, and the
// comment claiming it came from auth is what stopped the next reader checking.
// Derived, it also picks up whatever either package grows next.
var authorityPackages = []string{
	"internal/platform/auth",
	"internal/platform/database/storekit",
}

// The parameter names that say which subject an argument is. `entityType` is
// the audit row's — storekit.Audit's fourth argument is the same subject as
// auth.Require's second, under a different word.
var (
	tableParams  = map[string]bool{"table": true}
	objectParams = map[string]bool{"object": true, "entityType": true}
)

// authorityArgs maps each exported call in authorityPackages to the argument
// position at which it takes a table, and to the position at which it takes an
// RBAC object. A call taking neither is absent from both.
//
// Positions are ARGUMENT positions, so a method's receiver does not count and a
// call written `p.ApplyGuarded(ctx, tx, table, …)` lines up with the signature
// exactly as a free function does.
func authorityArgs(t *testing.T, fset *token.FileSet) (tables, objects map[string]int) {
	t.Helper()
	tables, objects = map[string]int{}, map[string]int{}
	for _, dir := range authorityPackages {
		for _, file := range parsePackageDir(t, fset, dir) {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || !fn.Name.IsExported() || fn.Type.Params == nil {
					continue
				}
				position := 0
				for _, field := range fn.Type.Params.List {
					for _, name := range namesOf(field) {
						if isStringType(field.Type) {
							if tableParams[name] {
								tables[fn.Name.Name] = position
							}
							if objectParams[name] {
								objects[fn.Name.Name] = position
							}
						}
						position++
					}
				}
			}
		}
	}
	if len(tables) < 10 || len(objects) < 3 {
		t.Fatalf("derived %d table-taking and %d object-taking call(s) from %v — the parameter names "+
			"this derivation reads have moved, and a short list is a gate that judges fewer sites "+
			"while reporting a clean tree", len(tables), len(objects), authorityPackages)
	}
	return tables, objects
}

// namesOf yields a parameter field's names, and one empty name for an unnamed
// parameter so the position still advances.
func namesOf(field *ast.Field) []string {
	if len(field.Names) == 0 {
		return []string{""}
	}
	out := make([]string, 0, len(field.Names))
	for _, name := range field.Names {
		out = append(out, name.Name)
	}
	return out
}

func isStringType(expr ast.Expr) bool {
	ident, isIdent := expr.(*ast.Ident)
	return isIdent && ident.Name == "string"
}

func TestAModuleThatNamedItsTableUsesThatName(t *testing.T) {
	t.Parallel()

	// The sweep finds the DECLARATIONS, and proves the roots are where they are.
	// The judgement then reads each declaring package WHOLE: a store keeps its
	// constants in one file and its writes in several, so a corpus of declaring
	// files alone would judge the one file least likely to contain a call.
	declared := map[string]map[string]string{} // package dir -> const name -> table
	for _, parsed := range (gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return len(tableConstsIn(file)) > 0 },
	}).Files(t) {
		dir := filepath.Dir(parsed.Path)
		if declared[dir] == nil {
			declared[dir] = map[string]string{}
		}
		maps.Copy(declared[dir], tableConstsIn(parsed.File))
	}

	fset := token.NewFileSet()
	tableArgs, objectArgs := authorityArgs(t, fset)

	judged, gated, opted := 0, 0, 0
	for _, dir := range slices.Sorted(maps.Keys(declared)) {
		opted++
		// One walk per package, feeding both the constant index and the
		// judgement: a table argument written as an identifier is resolved
		// against the constants of the very files being judged.
		files := packageFiles(t, fset, dir)
		consts := map[string]string{}
		for _, parsed := range files {
			collectStringConstants(parsed.File, consts)
		}
		for _, parsed := range files {
			judged += judgeTableArgs(t, fset, parsed, declared[dir], consts, tableArgs)
			gated += judgeObjectArgs(t, fset, parsed, objectArgs)
		}
	}

	// Floors under the REAL numbers, and one per half. Three packages opt in
	// today, carrying 67 table arguments and 94 object arguments between them —
	// measured, not estimated, because the corpus doubled when the derivation
	// replaced a hand-list and a floor calibrated against the old number would
	// have let a whole package fall out unnoticed.
	//
	// The object half needs its own floor for the reason the table half has one:
	// it reports findings and returns a count, and a count nobody checks is a
	// census that can go to zero while the gate reports a clean tree.
	if opted < 3 || judged < 60 || gated < 80 {
		t.Errorf("judged %d package(s) that name a table, %d table argument(s) and %d object "+
			"argument(s) among them — one of the ways of finding a call has stopped working",
			opted, judged, gated)
	}
}

// judgeTableArgs reports every table argument in one file written as a bare
// string its own package has already named, or as the package's OTHER constant
// holding that same string, and returns how many table arguments it read.
func judgeTableArgs(t *testing.T, fset *token.FileSet, parsed gatekit.ParsedFile,
	named, siblings map[string]string, reads map[string]int,
) int {
	t.Helper()
	seen := 0
	ast.Inspect(parsed.File, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		position, reading := reads[selector.Sel.Name]
		if !reading || position >= len(call.Args) {
			return true
		}
		seen++
		where := fset.Position(call.Pos())
		if value, isLiteral := plainStringLiteral(call.Args[position]); isLiteral {
			for name, declared := range named {
				if declared != value {
					continue
				}
				t.Errorf("%s:%d passes %q to %s, and this package already names that table %s.\n"+
					"\tUse the constant. A package that declares one and keeps the literal beside it reads "+
					"as though the sites follow it, and the rename that moves the table leaves them behind.",
					parsed.Path, where.Line, value, selector.Sel.Name, name)
			}
			return true
		}
		// A constant that is not the table's, but holds the table's exact string.
		//
		// This is the swap the convention exists to prevent, and it is the one
		// shape nothing else can see: `contractObject` and `contractTable` are
		// untyped strings of the same word, so passing either where the other
		// belongs compiles, type-checks and reads correctly. Narrow on purpose —
		// a constant holding a DIFFERENT value is a different table or a
		// different object, and saying anything about it would be this gate
		// legislating a naming scheme rather than holding an ambiguity.
		ident, isIdent := call.Args[position].(*ast.Ident)
		if !isIdent || tableConstName.MatchString(ident.Name) {
			return true
		}
		for name, declared := range named {
			if siblings[ident.Name] != declared {
				continue
			}
			t.Errorf("%s:%d passes %s to %s, which takes a TABLE — and %s holds %q, the same string "+
				"this package named %s.\n"+
				"\tOne of the two is the RBAC object or the audit entity type. Passing it here writes "+
				"one row and gates on another the day either name moves, and the compiler cannot tell "+
				"them apart to say so.",
				parsed.Path, where.Line, ident.Name, selector.Sel.Name, ident.Name, declared, name)
		}
		return true
	})
	return seen
}

// judgeObjectArgs is the mirror of judgeTableArgs: a `…Table` constant reaching
// a call that takes an RBAC OBJECT. It returns how many object arguments it
// read, so the caller can floor it.
//
// The two directions are one defect. A package that names its table has two
// untyped string constants holding the same word, so passing either where the
// other belongs compiles, type-checks, and reads correctly at a glance. The
// swap only becomes visible on the day one of the two names moves — and by then
// it is a permissions change, or an audit row filed under a table, that nobody
// wrote down.
func judgeObjectArgs(t *testing.T, fset *token.FileSet, parsed gatekit.ParsedFile,
	objectArgs map[string]int,
) int {
	t.Helper()
	seen := 0
	ast.Inspect(parsed.File, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		position, takesObject := objectArgs[selector.Sel.Name]
		if !takesObject || position >= len(call.Args) {
			return true
		}
		seen++
		ident, isIdent := call.Args[position].(*ast.Ident)
		if isIdent && tableConstName.MatchString(ident.Name) {
			t.Errorf("%s:%d passes %s to %s, which takes an RBAC OBJECT — and %s names a TABLE.\n"+
				"\tThe two are equal strings in this package and the compiler cannot tell them apart, "+
				"so this reads correctly right up until one of the names moves and the call starts "+
				"asking about a permission nobody granted, or filing an audit row under a table.",
				parsed.Path, fset.Position(call.Pos()).Line, ident.Name, selector.Sel.Name, ident.Name)
		}
		return true
	})
	return seen
}

// tableConstsIn indexes a file's package-level `…Table` string constants by
// name. The suffix is what makes this a rule the package chose rather than one
// imposed on it.
func tableConstsIn(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) || !tableConstName.MatchString(name.Name) {
					continue
				}
				if literal, ok := plainStringLiteral(value.Values[i]); ok {
					out[name.Name] = literal
				}
			}
		}
	}
	return out
}

// plainStringLiteral reads a bare string literal and nothing else. Distinct
// from replayscope's stringLiteral, which also resolves identifiers through a
// cache that gate fills as it walks: borrowing it would make this gate's answer
// depend on whether that one had run first.
func plainStringLiteral(expr ast.Expr) (string, bool) {
	literal, isLiteral := expr.(*ast.BasicLit)
	if !isLiteral || literal.Kind != token.STRING {
		return "", false
	}
	unquoted, err := strconv.Unquote(literal.Value)
	return unquoted, err == nil
}
