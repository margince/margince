// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// What the individual-performance path walk reads. That is each declaration
// of the switch's packages and each type converted to an interface. It is
// also each value stored in a variable or field of func type.

import (
	"fmt"
	"go/ast"
	"go/build"
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
	"internal/modules/commissions",
	"internal/modules/initiatives",
	"internal/compose",
}

// switchReader names one function by the import path of its package and its
// name, a method as Type.Method. answer places the result that carries the
// switch's answer. It is a bool a condition must read, or an error the caller
// must return or compare with nil. It may also be a value, such as the
// reader's own scope.
type switchReader struct {
	pkg, name string
	answer    int
}

const (
	authPath      = modulePath + "/internal/platform/auth"
	reportingPath = modulePath + "/internal/modules/reporting"
	composePath   = modulePath + "/internal/compose"
)

// walkConfig is what the walk takes as given about code it reads: the
// switch's readers and the outside functions whose behaviour no body here
// shows.
type walkConfig struct {
	readers, bare, dispatchers []switchReader
	// floors answer, per group of a figure, whether the floor withholds it:
	// true refuses, so only the branch where it is false is checked.
	floors []switchReader
	// synchronous are the outside callees that run a callback they are
	// handed before they return, spelled path.Func or path.Type.Method.
	synchronous []string
	// sentinelPackages are the outside packages whose Err names are errors
	// that are never nil.
	sentinelPackages []string
	// refusalConstructors are outside functions that always answer a new error.
	refusalConstructors []string
}

// switchWalkConfig is the real tree's. The readers are the value a request
// bound, the stored setting, and the rule that answers both for one seat.
// The reader's own seat counts too. MayMeasureSeat answers yes for it, whatever
// the switch says, so a path that builds it has nothing left to ask.
var switchWalkConfig = walkConfig{
	readers: []switchReader{
		{authPath, "IndividualPerformanceOf", 0},
		{authPath, "MayMeasureSeat", 0},
		{identityPath, "IndividualPerformanceTx", 0},
		{composePath, "ownScope", 0},
		// The weekly review's seat, read off the principal as ownScope reads it.
		// The weekly package sits below compose and cannot call ownScope.
		{composePath + "/weekly", "reviewUser", 0},
		// The grouping rule while the switch is off: nil only where no group
		// splits the answer by a colleague's seat.
		{composePath, "requireOwnGrouping", 0},
	},
	// thinGroups is the floor itself. measuresOwners is true where a spec
	// counts owners, so a false answer leaves no seat to floor.
	floors: []switchReader{{composePath, "thinGroups", 0}, {composePath, "measuresOwners", 0}},
	// Functions whose answer carries no figure keyed to a colleague. They are
	// not a check: a figure made after one still needs the switch.
	bare: []switchReader{
		{reportingPath, "Service.Catalog", 0},                              // the metric catalog: names and definitions
		{reportingPath, "Service.ListReports", 0},                          // saved report definitions by name
		{reportingPath, "Service.GetReport", 0},                            // one report definition: its selection, not its figures
		{modulePath + "/internal/modules/commissions", "Store.Summary", 0}, // owed to partners, never by seat
		{composePath, "restrictedQuestionAnswer", 0},                       // a restricted card's binding, with no answer
		{composePath + "/reportdoc", "Validate", 0},                        // the run ids a document cites, read from the document alone
		// valueFromAnswer reads one cell out of an answer it is handed, and
		// RenderReport hands it only answers ReadReportRun read through a check.
		{composePath, "valueFromAnswer", 0},
	},
	dispatchers: []switchReader{{modulePath + "/internal/modules/agents", "Registry.Invoke", 0}},
	synchronous: []string{
		databasePath + ".WithWorkspaceTx", databasePath + ".WithWorkspaceSnapshot",
		databasePath + ".DB.Tx", databasePath + ".DB.TxIsolated",
	},
	sentinelPackages:    []string{modulePath + "/internal/shared/apperrors", "github.com/jackc/pgx/v5"},
	refusalConstructors: []string{"fmt.Errorf", "errors.New", modulePath + "/internal/platform/httperr.Validation"},
}

// valueSite is one expression stored in a variable or field of func type,
// with the type information of the file it is written in.
type valueSite struct {
	expr ast.Expr
	info *types.Info
}

// invocation is one call of a declared function, for resolving what a
// parameter stored in a field was handed.
type invocation struct {
	call *ast.CallExpr
	info *types.Info
}

// paramSlot places a parameter in the signature of its function.
type paramSlot struct {
	fn    *types.Func
	index int
}

// switchGraph is the code the walk reads. converted holds each concrete type
// some expression turns into an interface value; stored holds what each
// func-typed variable or field is ever given. spelled holds the declared
// type of each variable whose type a stand-in import leaves invalid.
// reassigned holds each variable an assignment gives a new value.
type switchGraph struct {
	fset        *token.FileSet
	decls       map[*types.Func]*ast.FuncDecl
	pkgOf       map[*types.Func]string
	infos       map[*types.Func]*types.Info
	converted   map[types.Type]bool
	stored      map[*types.Var][]valueSite
	calls       map[*types.Func][]invocation
	paramOf     map[*types.Var]paramSlot
	writers     map[*types.Var]bool
	spelled     map[*types.Var]string
	inits       map[*types.Var]valueSite
	reassigned  map[*types.Var]bool
	outside     map[string]bool
	readerSet   map[*types.Func]int
	floorSet    map[*types.Func]int
	bareSet     map[*types.Func]bool
	dispatchSet map[*types.Func]bool
	config      walkConfig
	synchronous map[string]bool
	sentinels   map[*types.Var]bool
	refusing    map[*types.Func]bool
	caches      fieldCaches
}

// loadSwitchGraph type-checks the packages under dirs, each after the ones it
// imports, so a call across them resolves to its real declaration. A file
// the default build leaves out is not read.
func loadSwitchGraph(t *testing.T, config walkConfig, dirs ...string) *switchGraph {
	t.Helper()
	fset := gatekit.SourceFileSet()
	byPackage := map[string][]*ast.File{}
	for _, dir := range dirs {
		_, files := parseGoFilesUnder(t, dir)
		for _, file := range files {
			name := fset.Position(file.Pos()).Filename
			if built, err := build.Default.MatchFile(filepath.Dir(name), filepath.Base(name)); err != nil || !built {
				continue
			}
			pkgDir := filepath.ToSlash(filepath.Dir(name))
			byPackage[modulePath+"/"+pkgDir] = append(byPackage[modulePath+"/"+pkgDir], file)
		}
	}
	g, err := newSwitchGraph(fset, byPackage, config)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// newSwitchGraph reads byPackage and resolves config against it. It fails on
// a type error no stand-in import explains, since the walk would read that
// code wrong. It fails on a declared name it cannot find.
func newSwitchGraph(fset *token.FileSet, byPackage map[string][]*ast.File, config walkConfig) (*switchGraph, error) {
	g := &switchGraph{
		fset: fset, decls: map[*types.Func]*ast.FuncDecl{}, pkgOf: map[*types.Func]string{},
		infos: map[*types.Func]*types.Info{}, converted: map[types.Type]bool{},
		stored: map[*types.Var][]valueSite{}, writers: map[*types.Var]bool{},
		calls: map[*types.Func][]invocation{}, paramOf: map[*types.Var]paramSlot{},
		spelled: map[*types.Var]string{}, inits: map[*types.Var]valueSite{}, reassigned: map[*types.Var]bool{},
		outside: map[string]bool{}, config: config, sentinels: map[*types.Var]bool{}, refusing: map[*types.Func]bool{},
		caches: newFieldCaches(),
	}
	checked := switchImports{}
	var unexplained []string
	for _, path := range importOrder(byPackage) {
		files := byPackage[path]
		info := &types.Info{
			Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
			Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		var errs []types.Error
		conf := types.Config{Importer: checked, Error: func(err error) {
			if typeErr, ok := err.(types.Error); ok {
				errs = append(errs, typeErr)
			}
		}}
		pkg, err := conf.Check(path, fset, files, info)
		if pkg == nil {
			return nil, fmt.Errorf("type-checking %s: %w", path, err)
		}
		checked[path] = pkg
		unexplained = append(unexplained, unexplainedTypeErrors(errs, files, info, checked)...)
		for _, file := range files {
			g.index(file, info)
		}
		for _, file := range files {
			g.census(file, info)
			g.censusCaches(file, info)
		}
	}
	if len(unexplained) > 0 {
		return nil, fmt.Errorf("type-checking the walked packages found %d errors no stand-in import explains, "+
			"so the walk would read that code wrong. Fix the code, or teach the gate's importer:\n\t%s",
			len(unexplained), strings.Join(unexplained[:min(len(unexplained), 20)], "\n\t"))
	}
	return g, g.resolveConfig()
}

// resolveConfig resolves the declared readers and dispatchers to their
// declarations. Each synchronous callee must be one some package calls, so
// the list holds only what the tree uses.
func (g *switchGraph) resolveConfig() error {
	readers, err := g.resolveNamed(g.config.readers)
	if err != nil {
		return err
	}
	g.readerSet = readers
	if g.floorSet, err = g.resolveNamed(g.config.floors); err != nil {
		return err
	}
	bare, err := g.resolveNamed(g.config.bare)
	if err != nil {
		return err
	}
	g.bareSet = boolSet(bare)
	dispatch, err := g.resolveNamed(g.config.dispatchers)
	if err != nil {
		return err
	}
	g.dispatchSet = boolSet(dispatch)
	g.synchronous = map[string]bool{}
	for _, name := range g.config.synchronous {
		if !g.outside[name] {
			return fmt.Errorf("no walked package calls %s, which the scope gate declares synchronous; drop it", name)
		}
		g.synchronous[name] = true
	}
	return nil
}

func boolSet(named map[*types.Func]int) map[*types.Func]bool {
	out := make(map[*types.Func]bool, len(named))
	for fn := range named {
		out[fn] = true
	}
	return out
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

func (g *switchGraph) index(file *ast.File, info *types.Info) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
			g.decls[obj], g.pkgOf[obj], g.infos[obj] = fn, obj.Pkg().Path(), info
			if sig, ok := obj.Type().(*types.Signature); ok {
				for i := range sig.Params().Len() {
					g.paramOf[sig.Params().At(i)] = paramSlot{obj, i}
				}
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncType:
			g.indexWriters(node, info)
		case *ast.Field:
			g.spell(node.Names, node.Type, info)
		case *ast.ValueSpec:
			g.spell(node.Names, node.Type, info)
		}
		return true
	})
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			g.indexInits(gen, info)
		}
	}
}

// indexInits records the value each package variable is declared with.
func (g *switchGraph) indexInits(gen *ast.GenDecl, info *types.Info) {
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Values) != len(vs.Names) {
			continue
		}
		for i, name := range vs.Names {
			if v, ok := info.Defs[name].(*types.Var); ok {
				g.inits[v] = valueSite{vs.Values[i], info}
			}
		}
	}
}

// indexWriters records each parameter spelled as an http.ResponseWriter. The
// stand-in import gives it no type, so the spelling is what is read.
func (g *switchGraph) indexWriters(ft *ast.FuncType, info *types.Info) {
	for _, field := range ft.Params.List {
		sel, ok := field.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ResponseWriter" {
			continue
		}
		for _, name := range field.Names {
			if v, ok := info.Defs[name].(*types.Var); ok {
				g.writers[v] = true
			}
		}
	}
}

// census records each conversion of a concrete type to an interface in file.
// It also records each value handed to a variable or field of func type.
func (g *switchGraph) census(file *ast.File, info *types.Info) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			g.censusReassigned(node, info)
			g.spellFromCall(node, info)
			if len(node.Lhs) == len(node.Rhs) {
				for i, lhs := range node.Lhs {
					g.flow(info.TypeOf(lhs), node.Rhs[i], info)
					g.store(assignedObject(lhs, info), node.Rhs[i], info)
				}
			}
		case *ast.ValueSpec:
			for i, name := range node.Names {
				if i < len(node.Values) {
					g.flow(info.TypeOf(name), node.Values[i], info)
					g.store(info.Defs[name], node.Values[i], info)
				}
			}
		case *ast.CompositeLit:
			g.censusLiteral(node, info)
		case *ast.CallExpr:
			g.censusCall(node, info)
		case *ast.FuncDecl:
			g.censusReturns(node.Type, node.Body, info)
		case *ast.FuncLit:
			g.censusReturns(node.Type, node.Body, info)
		}
		return true
	})
}

func (g *switchGraph) censusLiteral(lit *ast.CompositeLit, info *types.Info) {
	litType := info.TypeOf(lit)
	if litType == nil {
		return
	}
	switch under := litType.Underlying().(type) {
	case *types.Struct:
		for i, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					g.flow(info.TypeOf(key), kv.Value, info)
					g.store(info.Uses[key], kv.Value, info)
				}
			} else if i < under.NumFields() {
				g.flow(under.Field(i).Type(), elt, info)
				g.store(under.Field(i), elt, info)
			}
		}
	case *types.Slice:
		g.censusElements(lit, under.Elem(), nil, info)
	case *types.Array:
		g.censusElements(lit, under.Elem(), nil, info)
	case *types.Map:
		g.censusElements(lit, under.Elem(), under.Key(), info)
	}
}

func (g *switchGraph) censusElements(lit *ast.CompositeLit, elem, key types.Type, info *types.Info) {
	for _, elt := range lit.Elts {
		value := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			value = kv.Value
			if key != nil {
				g.flow(key, kv.Key, info)
			}
		}
		g.flow(elem, value, info)
	}
}

// censusCall reads each argument as handed to its parameter, and an explicit
// conversion I(x) as a conversion.
func (g *switchGraph) censusCall(call *ast.CallExpr, info *types.Info) {
	if name := g.outsideCallee(call, info); name != "" {
		g.outside[name] = true
	}
	if fn := calledFunc(call, info); fn != nil {
		g.calls[fn] = append(g.calls[fn], invocation{call, info})
	}
	if tv, ok := info.Types[call.Fun]; ok && tv.IsType() && len(call.Args) == 1 {
		g.flow(tv.Type, call.Args[0], info)
		return
	}
	sig, ok := info.TypeOf(call.Fun).(*types.Signature)
	if !ok || sig.Params().Len() == 0 {
		return
	}
	last := sig.Params().Len() - 1
	for i, arg := range call.Args {
		paramType := sig.Params().At(min(i, last)).Type()
		if slice, ok := paramType.(*types.Slice); ok && sig.Variadic() && i >= last && !call.Ellipsis.IsValid() {
			paramType = slice.Elem()
		}
		g.flow(paramType, arg, info)
	}
}

// censusReturns reads each value a function returns as converted to its
// declared result type. A literal nested inside answers for its own results.
func (g *switchGraph) censusReturns(ft *ast.FuncType, body *ast.BlockStmt, info *types.Info) {
	if ft.Results == nil || body == nil {
		return
	}
	var results []types.Type
	for _, field := range ft.Results.List {
		for range max(1, len(field.Names)) {
			results = append(results, info.TypeOf(field.Type))
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if len(node.Results) == len(results) {
				for i, result := range node.Results {
					g.flow(results[i], result, info)
				}
			}
		}
		return true
	})
}

// flow records value's type when the place it goes to is an interface and the
// value is not one.
func (g *switchGraph) flow(target types.Type, value ast.Expr, info *types.Info) {
	source := info.TypeOf(value)
	if target == nil || source == nil || !types.IsInterface(target) || types.IsInterface(source) {
		return
	}
	if basic, ok := source.(*types.Basic); ok && (basic.Kind() == types.UntypedNil || basic.Kind() == types.Invalid) {
		return
	}
	g.converted[source] = true
}

// store records value as one a func-typed variable or field is given.
func (g *switchGraph) store(target types.Object, value ast.Expr, info *types.Info) {
	if v, ok := target.(*types.Var); ok && isFuncTyped(v) {
		g.stored[v.Origin()] = append(g.stored[v.Origin()], valueSite{value, info})
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

func isFuncTyped(v *types.Var) bool {
	_, ok := v.Type().Underlying().(*types.Signature)
	return ok
}

// implementations are the declared methods that answer a call of method on
// iface. There is one for each type the code converts to an interface that
// has the method. A type a stand-in leaves unresolved has no method, so it is
// not one. ok is false when there is none, or one has no body here.
func (g *switchGraph) implementations(iface *types.Interface, method *types.Func) (found []*types.Func, ok bool) {
	ok = true
	for _, concrete := range slices.SortedFunc(maps.Keys(g.converted), byTypeName) {
		if !types.Implements(concrete, iface) {
			continue
		}
		obj, _, _ := types.LookupFieldOrMethod(concrete, true, method.Pkg(), method.Name())
		fn, isFunc := obj.(*types.Func)
		if !isFunc {
			continue
		}
		if g.decls[fn.Origin()] == nil {
			ok = false
			continue
		}
		found = append(found, fn.Origin())
	}
	return found, ok && len(found) > 0
}

func byTypeName(a, b types.Type) int {
	return strings.Compare(a.String(), b.String())
}

func (g *switchGraph) resolveNamed(named []switchReader) (map[*types.Func]int, error) {
	found := map[*types.Func]int{}
	for _, reader := range named {
		hit := false
		for fn, decl := range g.decls {
			if g.pkgOf[fn] == reader.pkg && declLabel(decl) == reader.name {
				found[fn], hit = reader.answer, true
			}
		}
		if !hit {
			return nil, fmt.Errorf("found no %s.%s, which the scope gate names; it has stopped reading it", reader.pkg, reader.name)
		}
	}
	return found, nil
}

// declLabel names a function as switchReader spells it: Type.Method or Func.
func declLabel(decl *ast.FuncDecl) string {
	if recv := receiverTypeName(decl); recv != "" {
		return recv + "." + decl.Name.Name
	}
	return decl.Name.Name
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
