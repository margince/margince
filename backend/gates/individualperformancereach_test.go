// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The call graph the individual-performance scope gate walks: from a handler
// or a tool to the functions that read the switch.

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// switchReachDirs are the packages whose code a surface of the switch runs
// through. A handler or tool outside them is reported as not found.
var switchReachDirs = []string{
	"internal/platform/auth",
	"internal/modules/identity",
	"internal/modules/reporting",
	"internal/modules/forecasting",
	"internal/modules/agents",
	"internal/modules/assurance",
	"internal/compose",
}

// switchReader names one function that reads the installation's switch.
type switchReader struct{ pkg, name string }

// switchReaders are the switch's readers: the value a request bound, the
// stored setting, and the rule that answers both for one seat.
var switchReaders = []switchReader{
	{"auth", "IndividualPerformanceOf"},
	{"auth", "MayMeasureSeat"},
	{"identity", "IndividualPerformanceTx"},
}

// switchGraph links each declared function, and each variable of func type,
// to every function or such variable its code names or is handed. A call
// through an interface names every declared method that could answer it.
type switchGraph struct {
	fset    *token.FileSet
	edges   map[types.Object]map[types.Object]bool
	decls   map[*types.Func]*ast.FuncDecl
	methods map[string][]*types.Func
	pkgOf   map[*types.Func]string
	infos   map[*types.Func]*types.Info
}

// loadSwitchGraph type-checks the packages under dirs, each after the ones it
// imports, so a call across them resolves to its real declaration.
func loadSwitchGraph(t *testing.T, dirs ...string) *switchGraph {
	t.Helper()
	fset := gatekit.SourceFileSet()
	byPackage := map[string][]*ast.File{}
	for _, dir := range dirs {
		_, files := parseGoFilesUnder(t, dir)
		for _, file := range files {
			pkgDir := filepath.ToSlash(filepath.Dir(fset.Position(file.Pos()).Filename))
			byPackage[modulePath+"/"+pkgDir] = append(byPackage[modulePath+"/"+pkgDir], file)
		}
	}
	return newSwitchGraph(fset, byPackage)
}

func newSwitchGraph(fset *token.FileSet, byPackage map[string][]*ast.File) *switchGraph {
	g := &switchGraph{
		fset: fset, edges: map[types.Object]map[types.Object]bool{}, decls: map[*types.Func]*ast.FuncDecl{},
		methods: map[string][]*types.Func{}, pkgOf: map[*types.Func]string{}, infos: map[*types.Func]*types.Info{},
	}
	checked := sharedImports{}
	infos := map[string]*types.Info{}
	for _, path := range importOrder(byPackage) {
		files := byPackage[path]
		info := &types.Info{
			Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
			Types: map[ast.Expr]types.TypeAndValue{},
		}
		conf := types.Config{Importer: checked, Error: func(error) {}}
		pkg, _ := conf.Check(path, fset, files, info)
		checked[path], infos[path] = pkg, info
		g.index(files, info)
	}
	for path, info := range infos {
		g.link(byPackage[path], info)
	}
	return g
}

// importOrder lists the packages so that each follows every package of the
// set it imports.
func importOrder(byPackage map[string][]*ast.File) []string {
	var order []string
	done := map[string]bool{}
	var visit func(path string)
	visit = func(path string) {
		if done[path] {
			return
		}
		done[path] = true
		for _, file := range byPackage[path] {
			for _, spec := range file.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if _, ours := byPackage[imported]; err == nil && ours {
					visit(imported)
				}
			}
		}
		order = append(order, path)
	}
	for _, path := range slices.Sorted(maps.Keys(byPackage)) {
		visit(path)
	}
	return order
}

func (g *switchGraph) index(files []*ast.File, info *types.Info) {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
				g.decls[obj], g.pkgOf[obj], g.infos[obj] = fn, file.Name.Name, info
				if fn.Recv != nil {
					g.methods[methodKey(obj)] = append(g.methods[methodKey(obj)], obj)
				}
			}
		}
	}
}

// link adds what each declaration names. It also adds every value handed to a
// variable of func type: by assignment, as a struct field, or as an argument.
func (g *switchGraph) link(files []*ast.File, info *types.Info) {
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
					g.addAll(obj, g.named(fn.Body, info))
				}
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				g.linkFlow(n, info)
				return true
			})
		}
	}
}

func (g *switchGraph) linkFlow(n ast.Node, info *types.Info) {
	switch node := n.(type) {
	case *ast.AssignStmt:
		for i, lhs := range node.Lhs {
			if len(node.Rhs) == len(node.Lhs) {
				g.handTo(assignedObject(lhs, info), node.Rhs[i], info)
			} else if len(node.Rhs) == 1 {
				g.handTo(assignedObject(lhs, info), node.Rhs[0], info)
			}
		}
	case *ast.ValueSpec:
		for i, name := range node.Names {
			if i < len(node.Values) {
				g.handTo(info.Defs[name], node.Values[i], info)
			}
		}
	case *ast.CompositeLit:
		g.linkFields(node, info)
	case *ast.CallExpr:
		sig, ok := info.TypeOf(node.Fun).(*types.Signature)
		if !ok {
			return
		}
		for i, arg := range node.Args {
			if params := sig.Params(); params.Len() > 0 {
				g.handTo(params.At(min(i, params.Len()-1)), arg, info)
			}
		}
	}
}

// linkFields hands each field value of a struct literal to its field.
func (g *switchGraph) linkFields(lit *ast.CompositeLit, info *types.Info) {
	st, _ := info.TypeOf(lit).(*types.Struct)
	if named, ok := info.TypeOf(lit).(*types.Named); ok {
		st, _ = named.Underlying().(*types.Struct)
	}
	for i, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok {
				g.handTo(info.Uses[key], kv.Value, info)
			}
		} else if st != nil && i < st.NumFields() {
			g.handTo(st.Field(i), elt, info)
		}
	}
}

func assignedObject(lhs ast.Expr, info *types.Info) types.Object {
	switch target := lhs.(type) {
	case *ast.Ident:
		if obj := info.Defs[target]; obj != nil {
			return obj
		}
		return info.Uses[target]
	case *ast.SelectorExpr:
		return info.Uses[target.Sel]
	}
	return nil
}

// handTo links a variable of func type to what value names.
func (g *switchGraph) handTo(target types.Object, value ast.Expr, info *types.Info) {
	if v, ok := target.(*types.Var); ok && isFuncTyped(v) {
		g.addAll(v, g.named(value, info))
	}
}

func isFuncTyped(v *types.Var) bool {
	_, ok := v.Type().Underlying().(*types.Signature)
	return ok
}

// named lists the functions and func-typed variables node names.
func (g *switchGraph) named(node ast.Node, info *types.Info) []types.Object {
	var found []types.Object
	ast.Inspect(node, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		switch obj := info.Uses[ident].(type) {
		case *types.Func:
			found = append(found, obj.Origin())
		case *types.Var:
			if isFuncTyped(obj) {
				found = append(found, obj.Origin())
			}
		}
		return true
	})
	return found
}

func (g *switchGraph) addAll(from types.Object, to []types.Object) {
	if from == nil || len(to) == 0 {
		return
	}
	if g.edges[from] == nil {
		g.edges[from] = map[types.Object]bool{}
	}
	for _, obj := range to {
		g.edges[from][obj] = true
	}
}

// next is what running obj may run next; an interface method runs any
// declared method that could answer it.
func (g *switchGraph) next(obj types.Object) []types.Object {
	out := slices.Collect(maps.Keys(g.edges[obj]))
	if fn, ok := obj.(*types.Func); ok && g.decls[fn] == nil {
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
			for _, m := range g.methods[methodKey(fn)] {
				out = append(out, m)
			}
		}
	}
	return out
}

// readers resolves the switch's readers to their declarations.
func (g *switchGraph) readers(named []switchReader) (map[types.Object]bool, error) {
	found := map[types.Object]bool{}
	for _, reader := range named {
		hit := false
		for fn := range g.decls {
			if g.pkgOf[fn] == reader.pkg && fn.Name() == reader.name {
				found[fn], hit = true, true
			}
		}
		if !hit {
			return nil, fmt.Errorf("found no %s.%s, the switch's reader; the scope gate has stopped reading it", reader.pkg, reader.name)
		}
	}
	return found, nil
}

// reaches reports whether running root may run one of targets.
func (g *switchGraph) reaches(root types.Object, targets map[types.Object]bool) bool {
	seen := map[types.Object]bool{root: true}
	queue := []types.Object{root}
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]
		if targets[obj] {
			return true
		}
		for _, next := range g.next(obj) {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

// httpHandlers are the hand-written methods that serve operationID: named for
// it, taking the response writer first. The generated stubs answer 501.
func (g *switchGraph) httpHandlers(operationID string) []*types.Func {
	name := handlerName(operationID)
	var found []*types.Func
	for fn, decl := range g.decls {
		generated := strings.HasSuffix(g.fset.Position(decl.Pos()).Filename, "_gen.go")
		if decl.Recv != nil && fn.Name() == name && !generated && writesAResponse(decl) {
			found = append(found, fn)
		}
	}
	return slices.SortedFunc(slices.Values(found), g.byPosition)
}

func writesAResponse(decl *ast.FuncDecl) bool {
	params := decl.Type.Params.List
	if len(params) == 0 {
		return false
	}
	sel, ok := params[0].Type.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "ResponseWriter"
}

// toolHandlers are the Handle methods of the tool types whose Spec names
// verb as its tool name.
func (g *switchGraph) toolHandlers(verb string) []*types.Func {
	specOf := map[string]bool{}
	for fn, decl := range g.decls {
		if fn.Name() == "Spec" && decl.Recv != nil && namesTool(decl.Body, g.infos[fn], verb) {
			specOf[g.pkgOf[fn]+"."+receiverTypeName(decl)] = true
		}
	}
	var found []*types.Func
	for fn, decl := range g.decls {
		if fn.Name() == "Handle" && decl.Recv != nil && specOf[g.pkgOf[fn]+"."+receiverTypeName(decl)] {
			found = append(found, fn)
		}
	}
	return slices.SortedFunc(slices.Values(found), g.byPosition)
}

// namesTool reports whether body sets a tool spec's Name to verb, spelled as a
// literal or a constant.
func namesTool(body *ast.BlockStmt, info *types.Info, verb string) bool {
	named := false
	ast.Inspect(body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return !named
		}
		key, isIdent := kv.Key.(*ast.Ident)
		value := info.Types[kv.Value].Value
		if isIdent && key.Name == "Name" && value != nil && value.Kind() == constant.String && constant.StringVal(value) == verb {
			named = true
		}
		return !named
	})
	return named
}

func (g *switchGraph) byPosition(a, b *types.Func) int {
	return strings.Compare(g.where(a), g.where(b))
}

func (g *switchGraph) where(fn *types.Func) string {
	return g.fset.Position(g.decls[fn].Pos()).String()
}
