// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A test that calls t.Parallel never assigns a package-level variable, its own
// package's or an imported one. Its siblings read that variable while it runs,
// so the swap reaches them. A helper the test calls counts as the test.
//
// It reads assignment statements, not lines. It does not see a write through a
// method, through reflection, or inside a function another package declares. An
// assignment inside sync.Once.Do, or in a function that takes a lock, is left
// alone: both are the guarded shapes.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// parallelGlobalWriters may assign a package-level variable while parallel.
var parallelGlobalWriters = gatekit.Waive(map[string]string{})

func TestNoParallelTestAssignsAPackageLevelVariable(t *testing.T) {
	t.Parallel()
	byDir := map[string]map[string]*ast.File{}
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if byDir[dir] == nil {
			byDir[dir] = map[string]*ast.File{}
		}
		byDir[dir][filepath.ToSlash(path)] = file
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	var offenders []string
	parallel := 0
	for _, files := range byDir {
		found, scanned := parallelGlobalWrites(files)
		parallel += scanned
		for _, hit := range found {
			if !parallelGlobalWriters.Waived(t, hit.test) {
				offenders = append(offenders, hit.test+" assigns "+hit.variable)
			}
		}
	}
	if parallel == 0 {
		t.Fatal("found no test that calls t.Parallel, so this gate's walk reads nothing it should")
	}
	if len(offenders) > 0 {
		t.Errorf("%d parallel test(s) assign a package-level variable their siblings read. Inject the value into what the test builds instead:\n\t%s",
			len(offenders), strings.Join(slices.Sorted(slices.Values(offenders)), "\n\t"))
	}
	parallelGlobalWriters.AssertAllMatched(t)
}

func TestTheParallelWriteGateSeesAHelperSwap(t *testing.T) {
	t.Parallel()
	src := `package fixture

import (
	"net/http"
	"sync"
	"testing"
)

var clock func() int
var once sync.Once
var cached int
var mu sync.Mutex

func swapClock() { clock = func() int { return 1 } }

func TestSwapsThroughAHelper(t *testing.T) {
	t.Parallel()
	swapClock()
}

func TestSwapsAnImportedVariable(t *testing.T) {
	t.Parallel()
	http.DefaultTransport = nil
}

func TestShadowsTheName(t *testing.T) {
	t.Parallel()
	clock := 1
	clock = 2
	_ = clock
}

func TestFillsAOnce(t *testing.T) {
	t.Parallel()
	once.Do(func() { cached = 1 })
}

func TestTakesALock(t *testing.T) {
	t.Parallel()
	mu.Lock()
	defer mu.Unlock()
	cached = 2
}

func TestRunsAlone(t *testing.T) {
	clock = nil
}
`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture_test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found, scanned := parallelGlobalWrites(map[string]*ast.File{"fixture_test.go": file})
	var got []string
	for _, hit := range found {
		got = append(got, hit.test+" "+hit.variable)
	}
	slices.Sort(got)
	want := []string{"fixture_test.go#TestSwapsAnImportedVariable http.DefaultTransport", "fixture_test.go#TestSwapsThroughAHelper clock"}
	if scanned != 5 || !slices.Equal(got, want) {
		t.Errorf("scanned %d parallel tests and found %v, want 5 and %v", scanned, got, want)
	}
}

// globalWrite is one parallel test and the variable it assigns.
type globalWrite struct{ test, variable string }

// parallelGlobalWrites finds the parallel tests among one directory's files
// that assign a package-level variable, and counts the parallel tests it read.
func parallelGlobalWrites(files map[string]*ast.File) ([]globalWrite, int) {
	vars := map[string]map[string]bool{}
	funcs := map[string]map[string]*ast.FuncDecl{}
	for _, f := range files {
		pkg := f.Name.Name
		if vars[pkg] == nil {
			vars[pkg], funcs[pkg] = map[string]bool{}, map[string]*ast.FuncDecl{}
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					for _, spec := range d.Specs {
						for _, n := range spec.(*ast.ValueSpec).Names {
							vars[pkg][n.Name] = n.Name != "_"
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					funcs[pkg][d.Name.Name] = d
				}
			}
		}
	}
	var found []globalWrite
	scanned := 0
	for _, path := range slices.Sorted(maps.Keys(files)) {
		f := files[path]
		w := writeScan{vars: vars[f.Name.Name], funcs: funcs[f.Name.Name], imports: importedNames(f), memo: map[string]string{}}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Body == nil || !callsParallel(fn.Body) {
				continue
			}
			scanned++
			if v := w.writes(fn); v != "" {
				found = append(found, globalWrite{test: path + "#" + fn.Name.Name, variable: v})
			}
		}
	}
	return found, scanned
}

func importedNames(f *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, im := range f.Imports {
		path := strings.Trim(im.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		names[name] = true
	}
	return names
}

// writeScan answers which package-level variable a function assigns, itself
// or through a function of its package it calls.
type writeScan struct {
	vars    map[string]bool
	funcs   map[string]*ast.FuncDecl
	imports map[string]bool
	memo    map[string]string
}

func (w writeScan) writes(fn *ast.FuncDecl) string {
	if v, seen := w.memo[fn.Name.Name]; seen {
		return v
	}
	w.memo[fn.Name.Name] = ""
	if takesALock(fn.Body) {
		return ""
	}
	locals := map[string]bool{}
	for _, field := range fn.Type.Params.List {
		for _, n := range field.Names {
			locals[n.Name] = true
		}
	}
	hit := ""
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if hit != "" || isOnceDo(n) {
			return false
		}
		switch node := n.(type) {
		case *ast.AssignStmt:
			hit = w.assigned(node, locals)
		case *ast.CallExpr:
			if id, ok := node.Fun.(*ast.Ident); ok && w.funcs[id.Name] != nil {
				hit = w.writes(w.funcs[id.Name])
			}
		}
		return hit == ""
	})
	w.memo[fn.Name.Name] = hit
	return hit
}

func (w writeScan) assigned(stmt *ast.AssignStmt, locals map[string]bool) string {
	for _, lhs := range stmt.Lhs {
		switch x := lhs.(type) {
		case *ast.Ident:
			if stmt.Tok == token.DEFINE {
				locals[x.Name] = true
			} else if w.vars[x.Name] && !locals[x.Name] {
				return x.Name
			}
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok && stmt.Tok == token.ASSIGN && w.imports[pkg.Name] && !locals[pkg.Name] && !w.vars[pkg.Name] {
				return pkg.Name + "." + x.Sel.Name
			}
		}
	}
	return ""
}

// isOnceDo reports a sync.Once fill, whose argument runs once per process.
func isOnceDo(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Do" && len(call.Args) == 1
}

func takesALock(body *ast.BlockStmt) bool {
	locked := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Lock" && len(call.Args) == 0 {
				locked = true
			}
		}
		return !locked
	})
	return locked
}
