// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// goGenerator is one package's golden tests run in the mode that rewrites
// their files: the -update… flags and UPDATE_… variables its tests read.
type goGenerator struct {
	Dir   string // the package directory, relative to the repository root
	Tests []string
	Flags []string
	Env   []string
	// Tags are the build tags a plain build lacks, such as integration for a
	// test that needs a database. This gate names such tests and runs none.
	Tags string
}

// sourceFile is one parsed test file and the tags its constraint names when a
// plain build leaves it out.
type sourceFile struct {
	file *ast.File
	tags string
}

// goGenerators finds every switch a test file declares and the tests that
// reach it, one generator per package directory and build kind.
func goGenerators(root string, tracked []string) ([]goGenerator, error) {
	byDir := map[string][]sourceFile{}
	fset := token.NewFileSet()
	for _, path := range tracked {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, path), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(path)
		byDir[dir] = append(byDir[dir], sourceFile{file: file, tags: buildTags(file)})
	}
	var generators []goGenerator
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		var plain, all, tagged []*ast.File
		var tags []string
		for _, f := range byDir[dir] {
			all = append(all, f.file)
			if f.tags == "" {
				plain = append(plain, f.file)
				continue
			}
			tagged = append(tagged, f.file)
			for tag := range strings.SplitSeq(f.tags, ",") {
				tags = appendNew(tags, tag)
			}
		}
		slices.Sort(tags)
		// A tagged build compiles the plain files too, so its tests may reach
		// a helper in either; only its own tests are its own.
		for _, g := range []goGenerator{
			packageGenerator(dir, plain, plain, ""),
			packageGenerator(dir, all, tagged, strings.Join(tags, ",")),
		} {
			if len(g.Tests) > 0 {
				generators = append(generators, g)
			}
		}
	}
	return generators, nil
}

// buildTags names the tags in the file's build constraint when a plain build
// leaves the file out. It is empty when a plain build compiles the file.
func buildTags(file *ast.File) string {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			expr, err := constraint.Parse(comment.Text)
			if err != nil || expr.Eval(func(string) bool { return false }) {
				continue
			}
			tags := tagsOf(expr, nil)
			slices.Sort(tags)
			return strings.Join(tags, ",")
		}
	}
	return ""
}

func tagsOf(expr constraint.Expr, tags []string) []string {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		return appendNew(tags, e.Tag)
	case *constraint.NotExpr:
		return tagsOf(e.X, tags)
	case *constraint.AndExpr:
		return tagsOf(e.Y, tagsOf(e.X, tags))
	case *constraint.OrExpr:
		return tagsOf(e.Y, tagsOf(e.X, tags))
	}
	return tags
}

// packageGenerator joins the switches declared in scope to the tests of
// testFiles that reach them.
func packageGenerator(dir string, scope, testFiles []*ast.File, tags string) goGenerator {
	flags := map[string]string{} // variable → flag name
	funcs := map[string]*ast.FuncDecl{}
	for _, file := range scope {
		collectFlags(file, flags)
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	var tests []string
	for _, file := range testFiles {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && isTest(fn) {
				tests = append(tests, fn.Name.Name)
			}
		}
	}
	reads := map[string][]string{} // function → switches it reads directly
	edges := map[string][]string{} // function → functions it names
	for name, fn := range funcs {
		reads[name], edges[name] = readsAndEdges(fn, flags, funcs)
	}
	g := goGenerator{Dir: dir, Tags: tags}
	slices.Sort(tests)
	for _, name := range tests {
		switches := reachedSwitches(name, reads, edges)
		if len(switches) == 0 {
			continue
		}
		g.Tests = append(g.Tests, name)
		for _, s := range switches {
			if strings.HasPrefix(s, "-") {
				g.Flags = appendNew(g.Flags, s)
			} else {
				g.Env = appendNew(g.Env, s+"=1")
			}
		}
	}
	slices.Sort(g.Flags)
	slices.Sort(g.Env)
	return g
}

// collectFlags records each package variable declared as flag.Bool("update…").
func collectFlags(file *ast.File, flags map[string]string) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			if name, ok := literalArg(value.Values[0], "flag", "Bool"); ok && strings.HasPrefix(name, "update") {
				flags[value.Names[0].Name] = "-" + name
			}
		}
	}
}

// readsAndEdges lists the switches a function reads and the package functions
// it names, called or passed as a value.
func readsAndEdges(fn *ast.FuncDecl, flags map[string]string, funcs map[string]*ast.FuncDecl) (reads, edges []string) {
	if fn.Body == nil {
		return nil, nil
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if name, ok := literalArg(n, "os", "Getenv"); ok && strings.HasPrefix(name, "UPDATE_") {
				reads = appendNew(reads, name)
			}
		case *ast.Ident:
			if flag, ok := flags[n.Name]; ok {
				reads = appendNew(reads, flag)
			}
			if _, ok := funcs[n.Name]; ok && n.Name != fn.Name.Name {
				edges = appendNew(edges, n.Name)
			}
		}
		return true
	})
	return reads, edges
}

// reachedSwitches walks the package's functions from one test.
func reachedSwitches(test string, reads, edges map[string][]string) []string {
	seen := map[string]bool{test: true}
	queue := []string{test}
	var switches []string
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, s := range reads[name] {
			switches = appendNew(switches, s)
		}
		for _, next := range edges[name] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return switches
}

func isTest(fn *ast.FuncDecl) bool {
	return strings.HasPrefix(fn.Name.Name, "Test") && fn.Type.Params != nil && len(fn.Type.Params.List) == 1
}

// literalArg matches pkg.Fn("literal", …) and answers the literal.
func literalArg(expr ast.Expr, pkg, fn string) (string, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != fn {
		return "", false
	}
	if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != pkg {
		return "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func appendNew(list []string, value string) []string {
	if slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}
