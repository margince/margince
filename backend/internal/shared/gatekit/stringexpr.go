// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Reading what string a Go expression holds, for the censuses that judge one.
//
// ONE reader, with the question it is answering made a PARAMETER. There were
// two, in two files, and neither was a superset of the other: one followed
// `string("v1")` and the other did not; one refused a concatenation with an
// unresolvable half and the other folded the half it could read. Both
// behaviours are legitimate — a census asking "is this DEFINITELY this string"
// wants the first, one asking "what can I SEE of this string" wants the second
// — but which a census got was decided by which file its author had read most
// recently. A census's blast radius is exactly what its reader can see, and
// picking the narrower one does not fail, error, or look any different: it
// reports a clean tree over a shape the other reader would have caught.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// StringFold says what a reading does with a part it cannot resolve.
type StringFold int

const (
	// FoldStrict answers "is this expression definitely this string". A part it
	// cannot resolve makes the whole expression not a string, so a census built
	// on it never judges text it half-invented.
	FoldStrict StringFold = iota
	// FoldTotal answers "what can I see of this string". A part it cannot
	// resolve becomes ComputedFragment and the fold carries on, so a statement
	// assembled from a literal and a variable is still judged on the literal.
	FoldTotal
)

// ComputedFragment stands in for a fragment a total fold cannot read — a
// function call, a parameter, an identifier from another package. It is one
// character so that a shape test over the folded text reads the fragment as a
// hole rather than as words the author did not write.
const ComputedFragment = "?"

// StringExpr reads expr as the string it produces, resolving identifiers
// through consts.
//
// It takes ast.Node rather than ast.Expr because most callers reach it from
// inside an ast.Inspect, where every node is a Node and asserting each one to
// an Expr first is a line that answers nothing.
//
// The bool means "this is definitely a string", not "the text is complete":
// under FoldTotal a concatenation with one readable half returns that half with
// true, and an unresolvable identifier on its own returns ComputedFragment with
// FALSE — which is what keeps a caller's walk descending into a call's
// arguments instead of stopping at the call.
func StringExpr(expr ast.Node, consts map[string]string, fold StringFold) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		return basicLitString(node, fold)
	case *ast.ParenExpr:
		return StringExpr(node.X, consts, fold)
	case *ast.Ident:
		if text, known := consts[node.Name]; known {
			return text, true
		}
		if fold == FoldTotal {
			return ComputedFragment, false
		}
		return "", false
	case *ast.CallExpr:
		return conversionString(node, consts, fold)
	case *ast.BinaryExpr:
		return concatString(node, consts, fold)
	}
	if fold == FoldTotal {
		return ComputedFragment, false
	}
	return "", false
}

func basicLitString(lit *ast.BasicLit, fold StringFold) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	if err == nil {
		return text, true
	}
	// A STRING literal the parser accepted and strconv cannot decode is not a
	// shape Go admits. A total fold stands a hole in for it rather than
	// inventing text; a strict one calls the expression unreadable.
	if fold == FoldTotal {
		return ComputedFragment, true
	}
	return "", false
}

// conversionString follows `string("gmail.com")`, which is a constant
// conversion and not a function this reader has to run — it is the identity on
// a string constant. Only the builtin spelled `string` with exactly one
// argument qualifies; a package-qualified or shadowed name is a different thing
// and is not followed.
func conversionString(call *ast.CallExpr, consts map[string]string, fold StringFold) (string, bool) {
	name, isIdent := call.Fun.(*ast.Ident)
	if !isIdent || name.Name != "string" || len(call.Args) != 1 {
		if fold == FoldTotal {
			return ComputedFragment, false
		}
		return "", false
	}
	return StringExpr(call.Args[0], consts, fold)
}

func concatString(expr *ast.BinaryExpr, consts map[string]string, fold StringFold) (string, bool) {
	if expr.Op != token.ADD {
		return "", false
	}
	left, leftIsString := StringExpr(expr.X, consts, fold)
	right, rightIsString := StringExpr(expr.Y, consts, fold)
	if fold == FoldTotal {
		// One readable half is enough: a statement assembled from a literal and
		// a variable is still judged on the literal, which is the half that
		// carries the words a census looks for.
		//
		// With NEITHER half readable the text is still both holes, not empty.
		// Returning "" there let a surrounding concatenation close over the gap
		// — `"a" + (x + y) + "b"` folding to `ab` — and a census then reads a
		// word the source does not contain, which is the one way a total fold
		// can be worse than no fold at all.
		return left + right, leftIsString || rightIsString
	}
	if !leftIsString || !rightIsString {
		return "", false
	}
	return left + right, true
}

// PackageStringConstants reads every string constant one package declares, as
// the table StringExpr folds identifiers against.
//
// Its callers sit in two builds of the gates and in module tests, so it lives
// where all of them can reach it. Non-test sources only: a test's constant is
// not a value the package ships.
func PackageStringConstants(t testing.TB, dir string) map[string]string {
	t.Helper()
	sources, err := Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s for its string constants: %v", dir, err)
	}
	var files []*ast.File
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := ParseFile(source, 0)
		if err != nil {
			t.Fatalf("parsing %s for its string constants: %v", source, err)
		}
		files = append(files, file)
	}
	return StringConstantsOf(t, files)
}

// StringConstantsOf reads the package-level string constants of one package's
// files, one value per name. A name declared with two values, as a build-tag
// pair can, fails: one value per name cannot say which build the caller means.
func StringConstantsOf(t testing.TB, files []*ast.File) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, values := range StringConstantValues(t, files) {
		if len(values) > 1 {
			t.Fatalf("const %s is declared with %d values %q, and this reader keeps one per name; "+
				"read it with gatekit.StringConstantValues", name, len(values), values)
		}
		out[name] = values[0]
	}
	return out
}

// constSpec is one name of a const declaration and the expression it takes.
type constSpec struct {
	name  string
	value ast.Expr
}

// StringConstantValues reads the package-level string constants of one
// package's files, with every distinct value each name is declared with.
//
// A value is whatever StringExpr reads strictly, so `"a" + "b"`, `string("a")`
// and `("a")` count. A value naming another constant resolves in any
// declaration order, once every declaration of that name agrees on one value.
// `const a, b = "x", "y"` and a grouped spec that repeats the one above it read
// as the compiler reads them.
func StringConstantValues(t testing.TB, files []*ast.File) map[string][]string {
	t.Helper()
	pending := constSpecsOf(t, files)
	declared := map[string]int{}
	for _, spec := range pending {
		declared[spec.name]++
	}
	values := map[string][]string{}
	resolved := map[string]int{}
	for progress := true; progress; {
		progress = false
		known := settledConstants(values, resolved, declared)
		var unread []constSpec
		for _, spec := range pending {
			text, isString := StringExpr(spec.value, known, FoldStrict)
			if !isString {
				unread = append(unread, spec)
				continue
			}
			if !slices.Contains(values[spec.name], text) {
				values[spec.name] = append(values[spec.name], text)
			}
			resolved[spec.name]++
			progress = true
		}
		pending = unread
	}
	return values
}

// settledConstants is the table identifiers resolve against: the names whose
// every declaration has resolved, to one value.
func settledConstants(values map[string][]string, resolved, declared map[string]int) map[string]string {
	known := map[string]string{}
	for name, texts := range values {
		if len(texts) == 1 && resolved[name] == declared[name] {
			known[name] = texts[0]
		}
	}
	return known
}

// constSpecsOf lists every named const of files with its expression. A grouped
// spec with no values repeats the expressions of the one above it, as Go does.
func constSpecsOf(t testing.TB, files []*ast.File) []constSpec {
	t.Helper()
	var specs []constSpec
	for _, file := range files {
		for _, decl := range file.Decls {
			general, isGeneral := decl.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.CONST {
				continue
			}
			var previous []ast.Expr
			for _, spec := range general.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				if len(value.Values) > 0 {
					previous = value.Values
				}
				for i, name := range value.Names {
					if name.Name == "_" || i >= len(previous) {
						continue
					}
					refuseUndecodable(t, name.Name, previous[i])
					specs = append(specs, constSpec{name.Name, previous[i]})
				}
			}
		}
	}
	return specs
}

// refuseUndecodable fails on a string literal strconv cannot decode, rather
// than let its constant drop out of the table without a word.
func refuseUndecodable(t testing.TB, name string, value ast.Expr) {
	t.Helper()
	ast.Inspect(value, func(node ast.Node) bool {
		lit, isLit := node.(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}
		if _, err := strconv.Unquote(lit.Value); err != nil {
			t.Fatalf("const %s holds the string literal %s, which does not unquote: %v", name, lit.Value, err)
		}
		return false
	})
}
