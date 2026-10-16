// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The codes the server can put in a Problem body's top-level `code`. They are
// read from every site that sets one, so no list of them is kept here.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const problemWriterDir = "internal/platform/httperr"

// bodyCodeCarriers are the httperr structs whose Code becomes the body's code:
// writeProblem encodes problem, Write fills it from Fault, and Classify fills
// Fault from DetailedError. Following those assignments needs type
// information, so they are named; the census refuses an httperr struct with a
// Code that neither list names.
var bodyCodeCarriers = map[string]bool{"problem": true, "Fault": true, "DetailedError": true}

// fieldCodeCarriers hold a per-field code, which the body carries in
// details.errors and never as its own code.
var fieldCodeCarriers = map[string]bool{"FieldError": true}

// faultMethodNames hand httperr a code it reads in moduleDeclaredFault. A local
// assigned from one is a relay there: the method's own return is the source.
var faultMethodNames = map[string]bool{"MessageFault": true, "FieldFault": true}

// goUnit is one non-test source file of the module, with the module-local
// packages it imports keyed by the qualifier it calls them by.
type goUnit struct {
	path    string
	dir     string
	file    *ast.File
	imports map[string]string
}

// codeFuncKey names a function or method by its package directory and name. A
// method is keyed by its name alone, because a call to it is matched by name.
type codeFuncKey struct {
	dir, name string
	method    bool
}

// codeCallSite is one call of a function, inside the function that makes it.
type codeCallSite struct {
	unit *goUnit
	in   *ast.FuncDecl
	call *ast.CallExpr
}

// codeAssign is one assignment statement, inside the function that makes it.
type codeAssign struct {
	unit *goUnit
	in   *ast.FuncDecl
	stmt *ast.AssignStmt
}

// fieldSet is one composite literal of a struct, inside the function that builds it.
type fieldSet struct {
	unit *goUnit
	in   *ast.FuncDecl
	lit  *ast.CompositeLit
}

type paramRef struct {
	fn    *ast.FuncDecl
	index int
}

type emittedCensus struct {
	units      []*goUnit
	consts     map[string]map[string]string
	structs    map[string]map[string]*ast.StructType
	calls      map[codeFuncKey][]codeCallSite
	asValue    map[codeFuncKey][]string
	composite  map[string][]fieldSet
	codes      map[string]string
	faults     []string
	followed   map[paramRef]bool
	fieldsRead map[string]bool
	funcs      map[codeFuncKey]*ast.FuncDecl
	owner      map[*ast.FuncDecl]*goUnit
	decls      map[string]map[string]constDecl
	assigns    []codeAssign
	t          *testing.T
}

// emittedProblemCodes reads the module's codes, keyed by code, valued by one
// file:line that sends it, and fails on a site whose code it cannot read.
func emittedProblemCodes(t *testing.T) map[string]string {
	t.Helper()
	census := newEmittedCensus(t, readGoUnits(t, "internal"))
	census.readSources()
	for _, fault := range census.faults {
		t.Error(fault)
	}
	return census.codes
}

// sharedStatusCodes answers, by status, the codes httperr's own writers send:
// the ones a mock of that status most likely means.
func sharedStatusCodes(t *testing.T) map[int][]string {
	t.Helper()
	census := newEmittedCensus(t, readGoUnits(t, problemWriterDir))
	statuses := httpStatusNames()
	byStatus := map[int][]string{}
	note := func(unit *goUnit, status, code ast.Expr) {
		number, known := statuses[selectedName(status)]
		text, folded := census.constant(code, unit, 0)
		if known && folded && !slices.Contains(byStatus[number], text) {
			byStatus[number] = append(byStatus[number], text)
		}
	}
	for name := range bodyCodeCarriers {
		shape := census.structs[problemWriterDir][name]
		for _, set := range census.composite[problemWriterDir+"."+name] {
			status, hasStatus := fieldValue(set.lit, shape, "Status")
			code, hasCode := fieldValue(set.lit, shape, "Code")
			if hasStatus && hasCode {
				note(set.unit, status, code)
			}
		}
	}
	for _, row := range census.tableRows() {
		note(row.unit, row.status, row.code)
	}
	for _, codes := range byStatus {
		slices.Sort(codes)
	}
	return byStatus
}

func selectedName(expr ast.Expr) string {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	return ""
}

// httpStatusNames maps net/http's status constant names to their numbers,
// spelled from the status text the way the constants are.
func httpStatusNames() map[string]int {
	names := map[string]int{}
	for code := 100; code < 600; code++ {
		if text := http.StatusText(code); text != "" {
			names["Status"+strings.NewReplacer(" ", "", "-", "", "'", "").Replace(text)] = code
		}
	}
	return names
}

func readGoUnits(t *testing.T, root string) []*goUnit {
	t.Helper()
	var units []*goUnit
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_gen.go") || slices.Contains(strings.Split(path, "/"), "testdata") {
			return nil
		}
		file, err := gatekit.ParseFile(path, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		units = append(units, &goUnit{
			path: filepath.ToSlash(path), dir: filepath.ToSlash(filepath.Dir(path)),
			file: file, imports: localImports(file),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("reading the module's sources: %v", err)
	}
	return units
}

func localImports(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, spec := range file.Imports {
		importPath := strings.Trim(spec.Path.Value, `"`)
		dir, local := strings.CutPrefix(importPath, modulePath+"/")
		if !local {
			continue
		}
		if qualifier, _ := gatekit.ImportedAs(file, importPath); qualifier != "" {
			imports[qualifier] = dir
		}
	}
	return imports
}

func newEmittedCensus(t *testing.T, units []*goUnit) *emittedCensus {
	t.Helper()
	census := &emittedCensus{
		t: t, units: units, consts: map[string]map[string]string{},
		structs: map[string]map[string]*ast.StructType{}, calls: map[codeFuncKey][]codeCallSite{},
		asValue: map[codeFuncKey][]string{}, composite: map[string][]fieldSet{},
		codes: map[string]string{}, followed: map[paramRef]bool{}, fieldsRead: map[string]bool{},
		funcs: map[codeFuncKey]*ast.FuncDecl{}, owner: map[*ast.FuncDecl]*goUnit{}, decls: map[string]map[string]constDecl{},
	}
	for _, unit := range units {
		census.index(unit)
	}
	return census
}

// index records the unit's struct declarations, its calls and the composite
// literals it builds, each with the function it sits in.
func (c *emittedCensus) index(unit *goUnit) {
	if c.structs[unit.dir] == nil {
		c.structs[unit.dir] = map[string]*ast.StructType{}
	}
	for _, decl := range unit.file.Decls {
		var in *ast.FuncDecl
		if fn, ok := decl.(*ast.FuncDecl); ok {
			in = fn
			c.owner[fn] = unit
			c.funcs[codeFuncKey{dir: unit.dir, name: fn.Name.Name, method: fn.Recv != nil}] = fn
		}
		c.indexStructs(unit, decl)
		called := map[ast.Node]bool{}
		ast.Inspect(decl, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				if key, ok := c.callee(unit, n.Fun); ok {
					called[n.Fun] = true
					c.calls[key] = append(c.calls[key], codeCallSite{unit: unit, in: in, call: n})
				}
			case *ast.AssignStmt:
				c.assigns = append(c.assigns, codeAssign{unit: unit, in: in, stmt: n})
			case *ast.CompositeLit:
				if name, ok := c.literalType(unit, n.Type); ok {
					c.composite[name] = append(c.composite[name], fieldSet{unit: unit, in: in, lit: n})
				}
				c.indexElided(unit, in, n)
			case *ast.Ident, *ast.SelectorExpr:
				if key, ok := c.callee(unit, n.(ast.Expr)); ok && !called[n] && !key.method && in != nil && n != in.Name {
					c.asValue[key] = append(c.asValue[key], c.at(unit, n))
				}
			}
			return true
		})
	}
}

func (c *emittedCensus) indexStructs(unit *goUnit, decl ast.Decl) {
	general, ok := decl.(*ast.GenDecl)
	if !ok || general.Tok != token.TYPE {
		return
	}
	for _, spec := range general.Specs {
		if typed, ok := spec.(*ast.TypeSpec); ok {
			if shape, ok := typed.Type.(*ast.StructType); ok {
				c.structs[unit.dir][typed.Name.Name] = shape
			}
		}
	}
}

// callee names the function a call reaches: a package-level name here or in
// an imported package, or a method by its name.
func (c *emittedCensus) callee(unit *goUnit, fun ast.Expr) (codeFuncKey, bool) {
	switch f := fun.(type) {
	case *ast.Ident:
		if f.Obj != nil && f.Obj.Kind != ast.Fun {
			return codeFuncKey{}, false
		}
		return codeFuncKey{dir: unit.dir, name: f.Name}, true
	case *ast.SelectorExpr:
		if pkg, ok := f.X.(*ast.Ident); ok && pkg.Obj == nil {
			if dir, imported := unit.imports[pkg.Name]; imported {
				return codeFuncKey{dir: dir, name: f.Sel.Name}, true
			}
		}
		return codeFuncKey{dir: unit.dir, name: f.Sel.Name, method: true}, true
	case *ast.IndexExpr:
		return c.callee(unit, f.X)
	case *ast.IndexListExpr:
		return c.callee(unit, f.X)
	}
	return codeFuncKey{}, false
}

// literalType names the struct a composite literal builds as "dir.Type", for a
// type declared in the unit's package or one it imports.
func (c *emittedCensus) literalType(unit *goUnit, expr ast.Expr) (string, bool) {
	switch typ := expr.(type) {
	case *ast.Ident:
		return unit.dir + "." + typ.Name, true
	case *ast.SelectorExpr:
		if pkg, ok := typ.X.(*ast.Ident); ok {
			if dir, imported := unit.imports[pkg.Name]; imported {
				return dir + "." + typ.Sel.Name, true
			}
		}
	}
	return "", false
}

func (c *emittedCensus) at(unit *goUnit, node ast.Node) string {
	return fmt.Sprintf("%s:%d", unit.path, gatekit.SourceFileSet().Position(node.Pos()).Line)
}

// indexElided records the elements of a slice or map literal that leave their
// struct type to the container, as `[]*T{{Code: "x"}}` does.
func (c *emittedCensus) indexElided(unit *goUnit, in *ast.FuncDecl, container *ast.CompositeLit) {
	var elem ast.Expr
	switch typ := container.Type.(type) {
	case *ast.ArrayType:
		elem = typ.Elt
	case *ast.MapType:
		elem = typ.Value
	default:
		return
	}
	if star, ok := elem.(*ast.StarExpr); ok {
		elem = star.X
	}
	name, ok := c.literalType(unit, elem)
	if !ok {
		return
	}
	for _, element := range container.Elts {
		if pair, ok := element.(*ast.KeyValueExpr); ok {
			element = pair.Value
		}
		if ref, ok := element.(*ast.UnaryExpr); ok {
			element = ref.X
		}
		if lit, ok := element.(*ast.CompositeLit); ok && lit.Type == nil {
			c.composite[name] = append(c.composite[name], fieldSet{unit: unit, in: in, lit: lit})
		}
	}
}

// readSources reads the code at every site that sets one: a body carrier's
// Code, the sentinel table's code column, and a MessageFault's first return.
func (c *emittedCensus) readSources() {
	c.holdCarrierLists()
	for name := range bodyCodeCarriers {
		shape := c.structs[problemWriterDir][name]
		for _, set := range c.composite[problemWriterDir+"."+name] {
			if value, ok := fieldValue(set.lit, shape, "Code"); ok {
				c.read(value, set.unit, set.in)
			}
		}
	}
	c.readSentinelTable()
	for _, assign := range c.assigns {
		if len(assign.stmt.Lhs) != len(assign.stmt.Rhs) {
			continue
		}
		for i, target := range assign.stmt.Lhs {
			sel, isSel := target.(*ast.SelectorExpr)
			if !isSel || sel.Sel.Name != "Code" {
				continue
			}
			if base, isIdent := sel.X.(*ast.Ident); isIdent && c.typedAsCarrier(base, assign.unit) {
				c.read(assign.stmt.Rhs[i], assign.unit, assign.in)
			}
		}
	}
	for _, unit := range c.units {
		for _, decl := range unit.file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Name.Name == "MessageFault" && fn.Body != nil {
				c.readFaultReturns(unit, fn)
			}
		}
	}
}

// holdCarrierLists refuses an httperr struct with a Code field that neither
// carrier list names, and a name in either list that httperr no longer declares.
func (c *emittedCensus) holdCarrierLists() {
	for name, shape := range c.structs[problemWriterDir] {
		if _, ok := fieldIndex(shape, "Code"); ok && !bodyCodeCarriers[name] && !fieldCodeCarriers[name] {
			c.faults = append(c.faults, fmt.Sprintf("%s declares %s with a Code field, and this census does not "+
				"know whether that code reaches the Problem body; name it in bodyCodeCarriers or fieldCodeCarriers",
				problemWriterDir, name))
		}
	}
	for name := range bodyCodeCarriers {
		if c.structs[problemWriterDir][name] == nil {
			c.faults = append(c.faults, fmt.Sprintf("bodyCodeCarriers names %s, which %s no longer declares", name, problemWriterDir))
		}
	}
}

// sentinelRow is one row of httperr's sentinel table.
type sentinelRow struct {
	unit         *goUnit
	status, code ast.Expr
}

// readSentinelTable reads the code column of httperr's sentinel mapping.
func (c *emittedCensus) readSentinelTable() {
	for _, row := range c.tableRows() {
		c.read(row.code, row.unit, nil)
	}
}

func (c *emittedCensus) tableRows() []sentinelRow {
	for _, unit := range c.units {
		if unit.dir != problemWriterDir {
			continue
		}
		for _, decl := range unit.file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if ok && len(value.Names) == 1 && value.Names[0].Name == sentinelMappingVar && len(value.Values) == 1 {
					return c.rowsOf(unit, value.Values[0])
				}
			}
		}
	}
	c.faults = append(c.faults, fmt.Sprintf("found no %s table in %s, so no sentinel's code was read", sentinelMappingVar, problemWriterDir))
	return nil
}

func (c *emittedCensus) rowsOf(unit *goUnit, table ast.Expr) []sentinelRow {
	var shape *ast.StructType
	lit, ok := table.(*ast.CompositeLit)
	if ok {
		if rows, isSlice := lit.Type.(*ast.ArrayType); isSlice {
			shape, _ = rows.Elt.(*ast.StructType)
		}
	}
	if shape == nil {
		c.faults = append(c.faults, c.at(unit, table)+": the sentinel table is not a slice of an inline struct, which is the shape this census reads")
		return nil
	}
	var rows []sentinelRow
	for _, row := range lit.Elts {
		entry, ok := row.(*ast.CompositeLit)
		code, hasCode := fieldValue(entry, shape, "code")
		status, hasStatus := fieldValue(entry, shape, "status")
		if !ok || !hasCode || !hasStatus {
			c.faults = append(c.faults, c.at(unit, row)+": a sentinel row whose code and status this census cannot find")
			continue
		}
		rows = append(rows, sentinelRow{unit: unit, status: status, code: code})
	}
	return rows
}

// readFaultReturns reads the code each return of a MessageFault method gives.
func (c *emittedCensus) readFaultReturns(unit *goUnit, fn *ast.FuncDecl) {
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if ret, ok := node.(*ast.ReturnStmt); ok {
			if len(ret.Results) != 2 {
				c.faults = append(c.faults, c.at(unit, ret)+": a MessageFault return that is not (code, message), so its code cannot be read")
				return true
			}
			c.read(ret.Results[0], unit, fn)
		}
		return true
	})
}

// fieldValue answers the expression a composite literal gives a field, keyed
// or by position.
func fieldValue(lit *ast.CompositeLit, shape *ast.StructType, field string) (ast.Expr, bool) {
	if lit == nil || len(lit.Elts) == 0 {
		return nil, false
	}
	if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); keyed {
		for _, element := range lit.Elts {
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				if key, ok := pair.Key.(*ast.Ident); ok && key.Name == field {
					return pair.Value, true
				}
			}
		}
		return nil, false
	}
	index, ok := fieldIndex(shape, field)
	if !ok || index >= len(lit.Elts) {
		return nil, false
	}
	return lit.Elts[index], true
}

func fieldIndex(shape *ast.StructType, field string) (int, bool) {
	if shape == nil {
		return 0, false
	}
	index := 0
	for _, declared := range shape.Fields.List {
		if len(declared.Names) == 0 {
			index++
			continue
		}
		for _, name := range declared.Names {
			if name.Name == field {
				return index, true
			}
			index++
		}
	}
	return 0, false
}

// read folds a code expression to the codes it can hold. It follows the code
// to the caller, literal or table that sets it, since the census keeps no list.
func (c *emittedCensus) read(expr ast.Expr, unit *goUnit, in *ast.FuncDecl) {
	if code, ok := c.constant(expr, unit, 0); ok {
		if _, seen := c.codes[code]; !seen {
			c.codes[code] = c.at(unit, expr)
		}
		return
	}
	switch e := expr.(type) {
	case *ast.ParenExpr:
		c.read(e.X, unit, in)
		return
	case *ast.Ident:
		if index, ok := codeParamIndex(in, e); ok {
			c.followCallers(unit, in, index, e)
			return
		}
	case *ast.SelectorExpr:
		if c.readSelected(e, unit, in) {
			return
		}
	case *ast.CallExpr:
		if name, ok := e.Fun.(*ast.Ident); ok && name.Name == "string" && len(e.Args) == 1 {
			c.read(e.Args[0], unit, in)
			return
		}
		if c.relaysACode(expr, unit) {
			return
		}
	}
	c.faults = append(c.faults, fmt.Sprintf("%s: a Problem code this census cannot read; pass a literal, a package "+
		"constant or a parameter whose callers pass one", c.at(unit, expr)))
}

// readSelected reads `x.field` for a receiver, a range variable over a table
// literal, or a carrier whose code was read where it was set.
func (c *emittedCensus) readSelected(sel *ast.SelectorExpr, unit *goUnit, in *ast.FuncDecl) bool {
	base, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if receiver, typeName, isMethod := receiverOf(in); isMethod && base.Name == receiver {
		c.readField(unit, typeName, sel.Sel.Name, sel)
		return true
	}
	if rows, shape, ranged := rangedTable(base); ranged {
		for _, row := range rows.Elts {
			entry, isRow := row.(*ast.CompositeLit)
			if value, set := fieldValue(entry, shape, sel.Sel.Name); isRow && set {
				c.read(value, unit, in)
			}
		}
		return true
	}
	return c.relaysACode(sel, unit)
}

// rangedTable answers the slice literal of inline structs ident ranges over.
func rangedTable(ident *ast.Ident) (*ast.CompositeLit, *ast.StructType, bool) {
	if ident.Obj == nil {
		return nil, nil, false
	}
	assign, ok := ident.Obj.Decl.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return nil, nil, false
	}
	over, isRange := assign.Rhs[0].(*ast.UnaryExpr)
	if !isRange || over.Op != token.RANGE {
		return nil, nil, false
	}
	source := over.X
	if named, isIdent := source.(*ast.Ident); isIdent && named.Obj != nil {
		if spec, isSpec := named.Obj.Decl.(*ast.ValueSpec); isSpec && len(spec.Values) == 1 {
			source = spec.Values[0]
		}
	}
	table, isLit := source.(*ast.CompositeLit)
	if !isLit {
		return nil, nil, false
	}
	rows, isSlice := table.Type.(*ast.ArrayType)
	if !isSlice {
		return nil, nil, false
	}
	shape, isStruct := rows.Elt.(*ast.StructType)
	return table, shape, isStruct
}

// relaysACode reports a code copied from one read elsewhere. That is a body
// carrier's Code, or a local a fault method returned, either maybe bounded.
func (c *emittedCensus) relaysACode(expr ast.Expr, unit *goUnit) bool {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		base, ok := e.X.(*ast.Ident)
		return ok && e.Sel.Name == "Code" && c.typedAsCarrier(base, unit)
	case *ast.CallExpr:
		return len(e.Args) == 1 && c.relaysACode(e.Args[0], unit)
	case *ast.Ident:
		call, ok := assignedCall(e)
		method, isMethod := call.Fun.(*ast.SelectorExpr)
		return ok && isMethod && faultMethodNames[method.Sel.Name]
	}
	return false
}

// typedAsCarrier reports a variable declared as a body carrier, received as
// one, or assigned from errors.AsType or a function that answers one.
func (c *emittedCensus) typedAsCarrier(ident *ast.Ident, unit *goUnit) bool {
	if ident.Obj == nil {
		return false
	}
	switch decl := ident.Obj.Decl.(type) {
	case *ast.ValueSpec:
		return decl.Type != nil && c.isCarrier(decl.Type, unit)
	case *ast.Field:
		return c.isCarrier(decl.Type, unit)
	}
	if built, ok := assignedValue(ident); ok {
		if ref, isRef := built.(*ast.UnaryExpr); isRef {
			built = ref.X
		}
		if lit, isLit := built.(*ast.CompositeLit); isLit {
			return c.isCarrier(lit.Type, unit)
		}
	}
	call, ok := assignedCall(ident)
	if !ok {
		return false
	}
	if generic, isGeneric := call.Fun.(*ast.IndexExpr); isGeneric {
		return c.isCarrier(generic.Index, unit)
	}
	key, known := c.callee(unit, call.Fun)
	fn := c.funcs[key]
	return known && fn != nil && fn.Type.Results != nil && len(fn.Type.Results.List) > 0 &&
		c.isCarrier(fn.Type.Results.List[0].Type, c.owner[fn])
}

// assignedValue answers the one expression a `:=` gave ident.
func assignedValue(ident *ast.Ident) (ast.Expr, bool) {
	if ident.Obj == nil {
		return nil, false
	}
	assign, ok := ident.Obj.Decl.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return nil, false
	}
	return assign.Rhs[0], true
}

func assignedCall(ident *ast.Ident) (*ast.CallExpr, bool) {
	value, ok := assignedValue(ident)
	call, isCall := value.(*ast.CallExpr)
	if !ok || !isCall {
		return &ast.CallExpr{}, false
	}
	return call, true
}

func (c *emittedCensus) isCarrier(typ ast.Expr, unit *goUnit) bool {
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	name, ok := c.literalType(unit, typ)
	dir, typeName, _ := strings.Cut(name, ".")
	return ok && dir == problemWriterDir && bodyCodeCarriers[typeName]
}

// constant folds expr to the one string it is, through the constants of its
// own package and the packages its file imports.
func (c *emittedCensus) constant(expr ast.Expr, unit *goUnit, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok && pkg.Obj == nil {
			if dir, imported := unit.imports[pkg.Name]; imported {
				return c.namedConstant(dir, e.Sel.Name, depth)
			}
		}
		return "", false
	case *ast.CallExpr:
		if name, ok := e.Fun.(*ast.Ident); ok && name.Name == "string" && len(e.Args) == 1 {
			return c.constant(e.Args[0], unit, depth+1)
		}
		return "", false
	case *ast.Ident:
		if e.Obj != nil && e.Obj.Kind != ast.Con {
			return "", false
		}
		return c.namedConstant(unit.dir, e.Name, depth)
	case *ast.BinaryExpr:
		left, leftOK := c.constant(e.X, unit, depth+1)
		right, rightOK := c.constant(e.Y, unit, depth+1)
		return left + right, e.Op == token.ADD && leftOK && rightOK
	}
	return gatekit.StringExpr(expr, c.packageConsts(unit.dir), gatekit.FoldStrict)
}

func (c *emittedCensus) namedConstant(dir, name string, depth int) (string, bool) {
	if text, known := c.packageConsts(dir)[name]; known {
		return text, true
	}
	declared, known := c.constDecls(dir)[name]
	if !known {
		return "", false
	}
	return c.constant(declared.value, declared.unit, depth+1)
}

func (c *emittedCensus) packageConsts(dir string) map[string]string {
	if c.consts[dir] == nil {
		c.consts[dir] = gatekit.PackageStringConstants(c.t, filepath.FromSlash(dir))
	}
	return c.consts[dir]
}

// codeParamIndex answers the position of the parameter ident names in fn.
func codeParamIndex(fn *ast.FuncDecl, ident *ast.Ident) (int, bool) {
	if fn == nil || ident.Obj == nil || ident.Obj.Kind != ast.Var {
		return 0, false
	}
	index := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if name.Obj == ident.Obj {
				return index, true
			}
			index++
		}
	}
	return 0, false
}

func receiverOf(fn *ast.FuncDecl) (receiver, typeName string, ok bool) {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return "", "", false
	}
	typ := fn.Recv.List[0].Type
	if star, isStar := typ.(*ast.StarExpr); isStar {
		typ = star.X
	}
	name, isIdent := typ.(*ast.Ident)
	if !isIdent {
		return "", "", false
	}
	return fn.Recv.List[0].Names[0].Name, name.Name, true
}

// followCallers reads the argument every call of fn passes at index.
func (c *emittedCensus) followCallers(unit *goUnit, fn *ast.FuncDecl, index int, at ast.Node) {
	ref := paramRef{fn: fn, index: index}
	if c.followed[ref] {
		return
	}
	c.followed[ref] = true
	key := codeFuncKey{dir: unit.dir, name: fn.Name.Name, method: fn.Recv != nil}
	for _, use := range c.asValue[key] {
		c.faults = append(c.faults, fmt.Sprintf("%s: %s is used as a value, so the codes it is passed cannot be read; call it directly", use, fn.Name.Name))
	}
	sites := c.calls[key]
	if len(sites) == 0 && fn.Recv != nil {
		c.faults = append(c.faults, fmt.Sprintf("%s: the code comes from a parameter of method %s, which nothing in its "+
			"package calls by name, so the codes it is passed cannot be read", c.at(unit, at), fn.Name.Name))
	}
	for _, site := range sites {
		if index < len(site.call.Args) {
			c.read(site.call.Args[index], site.unit, site.in)
		}
	}
}

// readField reads field at every composite literal of the receiver's type.
func (c *emittedCensus) readField(unit *goUnit, typeName, field string, at ast.Node) {
	if c.fieldsRead[unit.dir+"."+typeName+"."+field] {
		return
	}
	c.fieldsRead[unit.dir+"."+typeName+"."+field] = true
	sets := c.composite[unit.dir+"."+typeName]
	shape := c.structs[unit.dir][typeName]
	if values, typed := c.typedValues(unit, shape, field); typed {
		for _, value := range values {
			if _, seen := c.codes[value]; !seen {
				c.codes[value] = c.at(unit, at)
			}
		}
		return
	}
	read := 0
	for _, set := range sets {
		if value, ok := fieldValue(set.lit, shape, field); ok {
			read++
			c.read(value, set.unit, set.in)
		}
	}
	if read == 0 {
		c.faults = append(c.faults, fmt.Sprintf("%s: the code is %s.%s, and no literal of %s sets it, so the codes it "+
			"holds cannot be read", c.at(unit, at), typeName, field, typeName))
	}
}

// constDecl is one package-level constant: its declared type name, if any, and
// the expression it is given.
type constDecl struct {
	typeName string
	value    ast.Expr
	unit     *goUnit
}

// constDecls indexes the package's constants, generated files included: the
// contract's enums live there. A planted package's live only in the census.
func (c *emittedCensus) constDecls(dir string) map[string]constDecl {
	if decls, read := c.decls[dir]; read {
		return decls
	}
	decls := map[string]constDecl{}
	c.decls[dir] = decls
	sources, err := filepath.Glob(filepath.Join(filepath.FromSlash(dir), "*.go"))
	if err != nil {
		c.t.Fatalf("listing %s for its constants: %v", dir, err)
	}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(source, 0)
		if err != nil {
			c.t.Fatalf("parsing %s for its constants: %v", source, err)
		}
		indexConstDecls(&goUnit{path: filepath.ToSlash(source), dir: dir, file: file, imports: localImports(file)}, decls)
	}
	for _, unit := range c.units {
		if unit.dir == dir {
			indexConstDecls(unit, decls)
		}
	}
	return decls
}

func indexConstDecls(unit *goUnit, into map[string]constDecl) {
	for _, decl := range unit.file.Decls {
		if general, ok := decl.(*ast.GenDecl); ok && general.Tok == token.CONST {
			indexConstSpecs(general, unit, into)
		}
	}
}

func indexConstSpecs(general *ast.GenDecl, unit *goUnit, into map[string]constDecl) {
	for _, spec := range general.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Names) != len(value.Values) {
			continue
		}
		typeName := ""
		if named, isIdent := value.Type.(*ast.Ident); isIdent {
			typeName = named.Name
		}
		for i, name := range value.Names {
			into[name.Name] = constDecl{typeName: typeName, value: value.Values[i], unit: unit}
		}
	}
}

// typedValues answers every constant of a field's named string type: the set
// the field can hold, whoever fills it.
func (c *emittedCensus) typedValues(unit *goUnit, shape *ast.StructType, field string) ([]string, bool) {
	typ, ok := carrierFieldType(shape, field)
	if !ok {
		return nil, false
	}
	dir, typeName := unit.dir, ""
	switch named := typ.(type) {
	case *ast.Ident:
		typeName = named.Name
	case *ast.SelectorExpr:
		pkg, isIdent := named.X.(*ast.Ident)
		if !isIdent {
			return nil, false
		}
		imported, local := unit.imports[pkg.Name]
		if !local {
			return nil, false
		}
		dir, typeName = imported, named.Sel.Name
	}
	if typeName == "" || typeName == "string" {
		return nil, false
	}
	var values []string
	for name, declared := range c.constDecls(dir) {
		if declared.typeName != typeName {
			continue
		}
		text, folded := c.namedConstant(dir, name, 0)
		if !folded {
			c.faults = append(c.faults, fmt.Sprintf("%s: constant %s of %s cannot be read as a string", declared.unit.path, name, typeName))
			continue
		}
		values = append(values, text)
	}
	return values, len(values) > 0
}

func carrierFieldType(shape *ast.StructType, field string) (ast.Expr, bool) {
	if shape == nil {
		return nil, false
	}
	for _, declared := range shape.Fields.List {
		for _, name := range declared.Names {
			if name.Name == field {
				return declared.Type, true
			}
		}
	}
	return nil, false
}
