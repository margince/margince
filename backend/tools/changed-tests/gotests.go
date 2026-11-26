// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// lane is the test lane that runs a test file. The unit lane takes a file the
// plain build compiles. The integration lane takes one that only
// `-tags=integration` compiles. A file of an opt-in lane (e2e_llm, bench) is in
// neither.
type lane string

const (
	unitLane        lane = "go-unit"
	integrationLane lane = "go-integration"
)

// goTest is one top-level test function the diff reaches. direct says the diff
// changed the function itself, and not only a helper it calls.
type goTest struct {
	dir    string
	name   string
	lane   lane
	direct bool
}

// declaration is one top-level name of a package's test files: the lines it
// spans and the top-level names it refers to. A method counts as part of its
// receiver's type, so a change to it reaches every test that names the type.
type declaration struct {
	name        string
	file        string
	first, last int
	refers      map[string]bool
}

// testPackage is the test files of one directory, read as one namespace. The
// internal and the external test package share it. So a helper name that both
// declare selects the callers of both.
type testPackage struct {
	decls []declaration
	tests map[string]lane
}

// changedGoTests returns every test of the backend module whose function, or a
// test-file declaration it reaches, the diff changed. A change to TestMain
// reaches every test of its package.
func changedGoTests(root string, changes map[string]*fileChange) ([]goTest, error) {
	dirs := map[string]bool{}
	for file := range changes {
		if strings.HasPrefix(file, "backend/") && strings.HasSuffix(file, "_test.go") {
			dirs[path.Dir(file)] = true
		}
	}
	var found []goTest
	for _, dir := range sortedKeys(dirs) {
		pkg, err := readTestPackage(root, dir)
		if err != nil {
			return nil, err
		}
		found = append(found, pkg.reached(dir, changes)...)
	}
	return found, nil
}

func readTestPackage(root, dir string) (testPackage, error) {
	pkg := testPackage{tests: map[string]lane{}}
	files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*_test.go"))
	if err != nil {
		return pkg, err
	}
	fset := token.NewFileSet()
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return pkg, err
		}
		fileLane, runs := laneOf(parsed)
		repoPath := dir + "/" + filepath.Base(file)
		for _, decl := range parsed.Decls {
			pkg.decls = append(pkg.decls, declarationsOf(fset, repoPath, decl)...)
			if fn, ok := decl.(*ast.FuncDecl); ok && runs && isTest(fn) {
				pkg.tests[fn.Name.Name] = fileLane
			}
		}
	}
	return pkg, nil
}

// reached walks from the declarations the diff changed to every test that
// refers to one of them, directly or through other declarations.
func (pkg testPackage) reached(dir string, changes map[string]*fileChange) []goTest {
	referrers := map[string][]string{}
	changed := map[string]bool{}
	for _, decl := range pkg.decls {
		for name := range decl.refers {
			referrers[name] = append(referrers[name], decl.name)
		}
		if change := changes[decl.file]; change != nil && change.touches(decl.first, decl.last) {
			changed[decl.name] = true
		}
	}
	reach := map[string]bool{}
	queue := sortedKeys(changed)
	if changed["TestMain"] {
		queue = append(queue, sortedKeys(pkg.tests)...)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if reach[name] {
			continue
		}
		reach[name] = true
		queue = append(queue, referrers[name]...)
	}
	var found []goTest
	for _, name := range sortedKeys(pkg.tests) {
		if reach[name] {
			found = append(found, goTest{dir: dir, name: name, lane: pkg.tests[name], direct: changed[name]})
		}
	}
	return found
}

// laneOf reads a file's //go:build line. runs is false for a file neither the
// unit nor the integration lane compiles.
func laneOf(file *ast.File) (lane, bool) {
	var expr constraint.Expr
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			if parsed, err := constraint.Parse(comment.Text); err == nil && constraint.IsGoBuild(comment.Text) {
				expr = parsed
			}
		}
	}
	if expr == nil || expr.Eval(func(string) bool { return false }) {
		return unitLane, true
	}
	if expr.Eval(func(tag string) bool { return tag == "integration" }) {
		return integrationLane, true
	}
	return "", false
}

// isTest is the rule `go test` applies: Test, then nothing or a character that
// is not a lower-case letter, taking one *testing.T.
func isTest(fn *ast.FuncDecl) bool {
	name := fn.Name.Name
	if fn.Recv != nil || !strings.HasPrefix(name, "Test") || name == "TestMain" {
		return false
	}
	if rest := name[len("Test"):]; rest != "" {
		if r, _ := utf8.DecodeRuneInString(rest); unicode.IsLower(r) {
			return false
		}
	}
	params := fn.Type.Params.List
	if len(params) != 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

// declarationsOf splits one top-level declaration into its names. A span
// starts at the keyword or the name, so a changed doc comment reaches no test.
func declarationsOf(fset *token.FileSet, file string, decl ast.Decl) []declaration {
	span := func(node ast.Node, name string) declaration {
		return declaration{
			name: name, file: file, refers: refersTo(node),
			first: fset.Position(node.Pos()).Line, last: fset.Position(node.End()).Line,
		}
	}
	switch d := decl.(type) {
	case *ast.FuncDecl:
		name := d.Name.Name
		if d.Recv != nil && len(d.Recv.List) == 1 {
			name = receiverType(d.Recv.List[0].Type)
		}
		return []declaration{span(d, name)}
	case *ast.GenDecl:
		var out []declaration
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				out = append(out, span(s, s.Name.Name))
			case *ast.ValueSpec:
				for _, ident := range s.Names {
					out = append(out, span(s, ident.Name))
				}
			}
		}
		return out
	}
	return nil
}

// receiverType names the type of a method receiver: T, *T, T[P] or *T[P].
func receiverType(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

func refersTo(node ast.Node) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			names[ident.Name] = true
		}
		return true
	})
	return names
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
