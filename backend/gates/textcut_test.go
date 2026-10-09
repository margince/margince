// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates_test

// Outside the waived files, no hand-written loop, rune slice or ToValidUTF8 cuts
// a string to a length; kernel/textcut does it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// textcutFloor is well under the module's hand-written Go file count, so a walk
// that stops early fails instead of reporting a clean tree.
const textcutFloor = 2000

// handCutWaivers are the files that may still cut a string by hand.
var handCutWaivers = gatekit.Waive(map[string]string{
	"internal/shared/kernel/textcut/textcut.go": "this is the helper every other cut calls; waiving it costs nothing, since the table test pins its answers.",
	"pkg/extension/files.go":                    "the published extension surface imports only the standard library, so its filename cut repeats the byte back-off and can drift from the helper.",
	"internal/compose/sitesnippet.go":           "splits one long sentence into consecutive pieces and keeps every rune, which a prefix cut cannot say; a change to how runes are counted has to be made there too.",
})

// A copy is matched by its statements, so it fails whatever its function is called.
// Three shapes count: a loop that backs off to a rune boundary,
// string(runes[:n]) over a []rune conversion, and strings.ToValidUTF8 over a
// byte slice. It cannot see a plain s[:n] that ignores runes, nor a []rune
// value passed in from another function.
func TestEveryStringCutGoesThroughTextcut(t *testing.T) {
	read := 0
	var offenders []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipNestedModule(path)
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		read++
		rel := filepath.ToSlash(path)
		for _, at := range handCuts(file) {
			if handCutWaivers.Waived(t, rel) {
				continue
			}
			offenders = append(offenders, rel+":"+strconv.Itoa(gatekit.SourceFileSet().Position(at).Line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the backend module: %v", err)
	}
	if read < textcutFloor {
		t.Fatalf("read only %d Go files and expects at least %d: the walk is broken, not the tree", read, textcutFloor)
	}
	if len(offenders) > 0 {
		t.Errorf("these cut a string to a length by hand; call textcut.Bytes, textcut.Runes or their Marked forms instead:\n  %s",
			strings.Join(offenders, "\n  "))
	}
	handCutWaivers.AssertAllMatched(t)
}

func TestARuneSliceNameBindsOnlyInItsOwnFunction(t *testing.T) {
	const source = `package p

func runes(s string) string {
	buf := []rune(s)
	return string(buf[:3])
}

func bytes(buf []byte) string {
	return string(buf[:3])
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", source, 0)
	if err != nil {
		t.Fatalf("parsing the planted source: %v", err)
	}
	if got := len(handCuts(file)); got != 1 {
		t.Errorf("found %d cuts, want 1: only the []rune cut counts, not the byte slice that shares its name", got)
	}
}

// skipNestedModule leaves out testdata and any directory that is its own Go
// module, since code there cannot import the helper.
func skipNestedModule(dir string) error {
	if dir == "." {
		return nil
	}
	if filepath.Base(dir) == "testdata" {
		return filepath.SkipDir
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return filepath.SkipDir
	}
	return nil
}

// handCuts returns the position of every hand-written cut in file.
func handCuts(file *ast.File) []token.Pos {
	var found []token.Pos
	for _, decl := range file.Decls {
		found = append(found, handCutsIn(decl)...)
	}
	return found
}

// handCutsIn reads rune-slice names from decl alone, so a same-named byte
// slice in another function is not taken for one.
func handCutsIn(decl ast.Decl) []token.Pos {
	runeSlices := runeSliceNames(decl)
	var found []token.Pos
	ast.Inspect(decl, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.ForStmt:
			if node.Cond != nil && callsUTF8Check(node.Cond) {
				found = append(found, node.Pos())
			}
		case *ast.CallExpr:
			if isRuneSliceToString(node, runeSlices) || isValidUTF8OfSlice(node) {
				found = append(found, node.Pos())
			}
		}
		return true
	})
	return found
}

// callsUTF8Check reports whether a loop condition asks utf8.RuneStart or
// utf8.ValidString, the two ways a hand-written cut backs off.
func callsUTF8Check(cond ast.Expr) bool {
	hit := false
	ast.Inspect(cond, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && isSelector(call.Fun, "utf8", "RuneStart", "ValidString") {
			hit = true
		}
		return !hit
	})
	return hit
}

// isRuneSliceToString matches string(x[:n]) where x is a []rune conversion
// or a name assigned from one.
func isRuneSliceToString(call *ast.CallExpr, runeSlices map[string]bool) bool {
	if fun, ok := call.Fun.(*ast.Ident); !ok || fun.Name != "string" || len(call.Args) != 1 {
		return false
	}
	slice, ok := call.Args[0].(*ast.SliceExpr)
	if !ok || slice.Low != nil || slice.High == nil {
		return false
	}
	if ident, ok := slice.X.(*ast.Ident); ok {
		return runeSlices[ident.Name]
	}
	inner, ok := slice.X.(*ast.CallExpr)
	return ok && isRuneConversion(inner)
}

// isValidUTF8OfSlice matches strings.ToValidUTF8(s[:n], ...), which drops the
// broken tail a byte cut leaves.
func isValidUTF8OfSlice(call *ast.CallExpr) bool {
	if !isSelector(call.Fun, "strings", "ToValidUTF8") || len(call.Args) == 0 {
		return false
	}
	slice, ok := call.Args[0].(*ast.SliceExpr)
	return ok && slice.Low == nil && slice.High != nil
}

// runeSliceNames returns the names in decl assigned from a []rune conversion.
func runeSliceNames(decl ast.Decl) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(decl, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			call, isCall := rhs.(*ast.CallExpr)
			ident, isIdent := assign.Lhs[i].(*ast.Ident)
			if isCall && isIdent && isRuneConversion(call) {
				names[ident.Name] = true
			}
		}
		return true
	})
	return names
}

// isRuneConversion matches []rune(x).
func isRuneConversion(call *ast.CallExpr) bool {
	array, ok := call.Fun.(*ast.ArrayType)
	if !ok || array.Len != nil {
		return false
	}
	elem, ok := array.Elt.(*ast.Ident)
	return ok && elem.Name == "rune"
}

// isSelector matches pkg.Name for any of names.
func isSelector(expr ast.Expr, pkg string, names ...string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != pkg {
		return false
	}
	return slices.Contains(names, sel.Sel.Name)
}
