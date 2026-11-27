// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A scan of the Go tree for the places a vocabulary is written. A site is one
// struct literal key or one call argument. It resolves when the scan can read
// its string. That is a literal, a constant of the same package, string() of
// either, or a constant of an imported package. An unreadable site is returned
// as unresolved, so a caller can fail on it or waive it by name.

import (
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// stringScan says where a vocabulary is written.
type stringScan struct {
	// literals are the struct types that carry it, qualified by package name
	// (approvals.StageInput), so a type of the same bare name elsewhere is not read.
	literals map[string]bool
	keys     map[string]bool
	// calls maps a callee name to the position of the argument that holds the value.
	calls map[string]int
	// stageCalls also reads a target type constant passed to any call that stages.
	stageCalls bool
}

// scannedSite is one place a value is written.
type scannedSite struct {
	file, expr string
	values     []string
}

func (s scannedSite) key() string { return s.file + ": " + s.expr }

type scannedTree struct {
	sources []scannedSource
	byDir   map[string]map[string]map[string]bool
	global  map[string]map[string]bool
}

type scannedSource struct {
	file    *ast.File
	path    string
	imports map[string]string
}

// constConversion is a constant declared as string(pkg.Const): its value is
// that of the named constant, which can sit in a package read later.
type constConversion struct {
	dir, name, from string
}

func readScannedTree(t *testing.T) scannedTree {
	t.Helper()
	tree := scannedTree{byDir: map[string]map[string]map[string]bool{}, global: map[string]map[string]bool{}}
	var pending []constConversion
	packages := map[string][]*ast.File{}
	err := gatekit.WalkDir("internal", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") ||
			strings.Contains(p, "internal/contracts/") {
			return err
		}
		file, err := gatekit.ParseFile(p, 0)
		if err != nil {
			return err
		}
		dir := path.Dir(p)
		packages[dir] = append(packages[dir], file)
		pending = append(pending, constConversions(dir, file)...)
		tree.sources = append(tree.sources, scannedSource{file, p, scanImportNames(file)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for dir, files := range packages {
		tree.byDir[dir] = map[string]map[string]bool{}
		for name, values := range gatekit.StringConstantValues(t, files) {
			tree.byDir[dir][name] = map[string]bool{}
			if tree.global[name] == nil {
				tree.global[name] = map[string]bool{}
			}
			for _, value := range values {
				tree.byDir[dir][name][value] = true
				tree.global[name][value] = true
			}
		}
	}
	for _, c := range pending {
		if values := tree.global[c.from]; len(values) == 1 {
			tree.byDir[c.dir][c.name] = values
			tree.global[c.name] = values
		}
	}
	return tree
}

// scanSites returns every site a scan reads, resolved or not.
func scanSites(t *testing.T, scan stringScan) []scannedSite {
	t.Helper()
	tree := readScannedTree(t)
	var sites []scannedSite
	for _, src := range tree.sources {
		dir := path.Dir(src.path)
		local := tree.byDir[dir]
		site := func(expr ast.Expr) {
			sites = append(sites, scannedSite{src.path, types.ExprString(expr), resolveStringValue(expr, local, tree.global, src.imports)})
		}
		ast.Inspect(src.file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if scan.stageCalls {
					for _, value := range stagedCallArguments(node, local) {
						sites = append(sites, scannedSite{src.path, "stage call argument", []string{value}})
					}
				}
				if at, ok := scan.calls[scanCalleeName(node.Fun)]; ok && at < len(node.Args) {
					site(node.Args[at])
				}
			case *ast.CompositeLit:
				if !scan.literals[qualifiedTypeName(node.Type, dir, src.imports)] {
					return true
				}
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, _ := kv.Key.(*ast.Ident); key != nil && scan.keys[key.Name] {
						site(kv.Value)
					}
				}
			}
			return true
		})
	}
	return sites
}

// scanValues splits sites into the distinct values they hold, with the file that
// writes each, and the sites that hold none the scan could read.
func scanValues(sites []scannedSite) (resolved map[string]string, unresolved []scannedSite) {
	resolved = map[string]string{}
	for _, s := range sites {
		if len(s.values) == 0 {
			unresolved = append(unresolved, s)
		}
		for _, value := range s.values {
			resolved[value] = s.file
		}
	}
	return resolved, unresolved
}

func scanCalleeName(fun ast.Expr) string {
	switch fn := fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// qualifiedTypeName is package.Type for a literal's type. A qualified name takes
// its import's package. A bare name takes the package of the file's directory.
func qualifiedTypeName(expr ast.Expr, dir string, imports map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return path.Base(dir) + "." + e.Name
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			if base, imported := imports[pkg.Name]; imported {
				return base + "." + e.Sel.Name
			}
		}
	}
	return ""
}

func stagedCallArguments(call *ast.CallExpr, local map[string]map[string]bool) []string {
	if !strings.Contains(strings.ToLower(scanCalleeName(call.Fun)), "stage") {
		return nil
	}
	var values []string
	for _, arg := range call.Args {
		id, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		name := strings.ToLower(id.Name)
		if strings.HasSuffix(name, "targettype") || strings.HasSuffix(name, "recordtype") {
			values = append(values, slices.Sorted(maps.Keys(local[id.Name]))...)
		}
	}
	return values
}

func constConversions(dir string, file *ast.File) []constConversion {
	var out []constConversion
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				call, ok := vs.Values[i].(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					continue
				}
				if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "string" {
					continue
				}
				if arg, ok := call.Args[0].(*ast.SelectorExpr); ok {
					out = append(out, constConversion{dir, name.Name, arg.Sel.Name})
				}
			}
		}
	}
	return out
}

// scanImportNames maps each name a file can qualify an identifier with to the
// package name it stands for.
func scanImportNames(file *ast.File) map[string]string {
	names := map[string]string{}
	for _, spec := range file.Imports {
		base := path.Base(strings.Trim(spec.Path.Value, `"`))
		if spec.Name != nil {
			names[spec.Name.Name] = base
			continue
		}
		names[base] = base
	}
	return names
}

// resolveStringValue reads an Ident from its own package. It reads a selector
// from the constant of that name, only when its qualifier is an import and one
// package declares it.
func resolveStringValue(expr ast.Expr, local, global map[string]map[string]bool, imports map[string]string) []string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if value, ok := gatekit.StringExpr(e, nil, gatekit.FoldStrict); ok {
			return []string{value}
		}
	case *ast.Ident:
		return slices.Sorted(maps.Keys(local[e.Name]))
	case *ast.SelectorExpr:
		if pkg, qualified := e.X.(*ast.Ident); !qualified || imports[pkg.Name] == "" {
			return nil
		}
		if values := global[e.Sel.Name]; len(values) == 1 {
			return slices.Sorted(maps.Keys(values))
		}
	case *ast.CallExpr:
		if fn, ok := e.Fun.(*ast.Ident); ok && fn.Name == "string" && len(e.Args) == 1 {
			return resolveStringValue(e.Args[0], local, global, imports)
		}
	}
	return nil
}
