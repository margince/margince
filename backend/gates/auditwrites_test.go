// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The record types the audit log is written about, read from the code that
// writes it, starting at storekit's insertAuditRows. A function that hands its
// own parameter to a writer's entity argument is itself a writer. Each other
// call's entity argument is followed to the values it can hold. One this cannot
// follow is returned as opaque, for the caller to refuse.

import (
	"go/ast"
	"go/build/constraint"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// auditWriteSite is one call that writes an audit row, and what its entity argument
// can be.
type auditWriteSite struct {
	pos    string // file:line, relative to the repository root
	src    *auditSource
	fn     string // the enclosing function's name
	call   *ast.CallExpr
	types  []string
	opaque string // the argument as written, when nothing above reaches a value
	// asValue marks a writer named other than as a call's function, whose calls
	// through that value this census cannot see.
	asValue bool
}

type auditSource struct {
	path       string
	dir        string
	file       *ast.File
	imports    map[string]string // qualifier → package directory, for this module's packages
	qualifiers map[string]bool   // every import's qualifier
	generated  bool
}

type auditFuncKey struct {
	dir, name string
	method    bool
}

type auditCall struct {
	src      *auditSource
	fn       *ast.FuncDecl
	closures []*ast.FuncType
	call     *ast.CallExpr
}

type auditConst struct {
	src  *auditSource
	expr ast.Expr
}

type auditLiteralField struct {
	src  *auditSource
	fn   *ast.FuncDecl
	expr ast.Expr
}

type auditCensus struct {
	repo    string
	sources []*auditSource
	funcs   map[auditFuncKey]*ast.FuncDecl
	srcOf   map[*ast.FuncDecl]*auditSource
	consts  map[string]map[string]auditConst
	structs map[string][]string // "dir.Type" → its field names
	// fields holds writes to a field under "dir.Type.Field", and those whose
	// struct type is unnamed under the writing package's "dir.*.Field". A nil
	// value is an unreadable write.
	fields map[string][]auditLiteralField
	// importers is, per package directory, the directories that import it.
	importers map[string]map[string]bool
	// wholeWrites are the struct types some code fills without naming a field.
	wholeWrites map[string]bool
	calls       map[string][]auditCall
	writers     map[auditFuncKey]int
	sites       []auditWriteSite
}

const auditModulePrefix = "github.com/margince/margince/"

// auditSeedDir and auditSeed name the INSERT every audit row in Go goes through.
var auditSeedDir = filepath.ToSlash(filepath.Join("backend", "internal", "platform", "database", "storekit"))

const auditSeed = "insertAuditRows"

var (
	auditCensusOnce   sync.Once
	auditCensusRead   *auditCensus
	auditCensusFailed string
)

// auditWriteSites answers every Go call that writes an audit row, read once for
// the whole gate run.
func auditWriteSites(t *testing.T) []auditWriteSite {
	t.Helper()
	auditCensusOnce.Do(func() {
		c := &auditCensus{
			repo: filepath.Join(gatekit.ModuleRoot(t), ".."), funcs: map[auditFuncKey]*ast.FuncDecl{},
			srcOf: map[*ast.FuncDecl]*auditSource{}, consts: map[string]map[string]auditConst{},
			structs: map[string][]string{}, fields: map[string][]auditLiteralField{}, wholeWrites: map[string]bool{},
			importers: map[string]map[string]bool{},
			calls:     map[string][]auditCall{}, writers: map[auditFuncKey]int{},
		}
		auditCensusFailed = c.load()
		for _, src := range c.sources {
			c.indexBodies(src)
		}
		if auditCensusFailed == "" {
			auditCensusFailed = c.grow()
		}
		if auditCensusFailed == "" {
			c.writersUsedAsValues()
		}
		auditCensusRead = c
	})
	if auditCensusFailed != "" {
		t.Fatal(auditCensusFailed)
	}
	return auditCensusRead.sites
}

// auditPackageMapKeys reads the string keys of a package-level map literal.
func auditPackageMapKeys(t *testing.T, dir, name string) []string {
	t.Helper()
	auditWriteSites(t)
	constant, ok := auditCensusRead.consts[dir][name]
	lit, isLiteral := constant.expr.(*ast.CompositeLit)
	if !ok || !isLiteral {
		t.Fatalf("%s no longer declares %s as a map literal", dir, name)
	}
	var keys []string
	for _, elt := range lit.Elts {
		kv, isPair := elt.(*ast.KeyValueExpr)
		values, resolved := []string(nil), false
		if isPair {
			values, resolved = auditCensusRead.resolve(constant.src, nil, nil, kv.Key, 0)
		}
		if !resolved {
			t.Fatalf("%s.%s has a key this gate cannot read: %s", dir, name, types.ExprString(elt))
		}
		keys = append(keys, values...)
	}
	return keys
}

// load parses every Go file of the repository that the default build compiles.
func (c *auditCensus) load() string {
	err := gatekit.WalkDir(c.repo, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == "testdata" || name == "node_modules" || (strings.HasPrefix(name, ".") && path != c.repo) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		source, err := gatekit.ReadFile(path)
		if err != nil || !inDefaultBuild(source) {
			return err
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		c.index(path, file)
		return nil
	})
	if err != nil {
		return "reading the repository: " + err.Error()
	}
	return ""
}

// inDefaultBuild reports whether a plain build compiles the file. One behind a
// test tag, such as the integration harness, is test code as a _test file is.
func inDefaultBuild(source []byte) bool {
	for line := range strings.SplitSeq(string(source), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return true
		}
		if expr, err := constraint.Parse(line); err == nil && constraint.IsGoBuild(line) {
			return expr.Eval(func(tag string) bool {
				return tag == runtime.GOOS || tag == runtime.GOARCH || tag == "unix" || strings.HasPrefix(tag, "go1.")
			})
		}
	}
	return true
}

func (c *auditCensus) index(path string, file *ast.File) {
	rel, err := filepath.Rel(c.repo, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	src := &auditSource{
		path: rel, dir: filepath.ToSlash(filepath.Dir(rel)), file: file, imports: map[string]string{},
		qualifiers: map[string]bool{}, generated: strings.HasSuffix(rel, "_gen.go"),
	}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if qualifier, _ := gatekit.ImportedAs(file, imported); qualifier != "" {
			src.qualifiers[qualifier] = true
		}
		if !strings.HasPrefix(imported, auditModulePrefix) {
			continue
		}
		dir := strings.TrimPrefix(imported, auditModulePrefix)
		if qualifier, _ := gatekit.ImportedAs(file, imported); qualifier != "" {
			src.imports[qualifier] = dir
		}
		if c.importers[dir] == nil {
			c.importers[dir] = map[string]bool{}
		}
		c.importers[dir][src.dir] = true
	}
	consts := c.consts[src.dir]
	if consts == nil {
		consts = map[string]auditConst{}
		c.consts[src.dir] = consts
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch sp := spec.(type) {
				case *ast.ValueSpec:
					for i, name := range sp.Names {
						if len(sp.Values) == len(sp.Names) {
							consts[name.Name] = auditConst{src: src, expr: sp.Values[i]}
						}
					}
				case *ast.TypeSpec:
					if st, isStruct := sp.Type.(*ast.StructType); isStruct {
						c.structs[src.dir+"."+sp.Name.Name] = auditStructFields(st)
					}
				}
			}
		case *ast.FuncDecl:
			key := auditFuncKey{dir: src.dir, name: d.Name.Name, method: d.Recv != nil}
			c.funcs[key], c.srcOf[d] = d, src
		}
	}
	c.sources = append(c.sources, src)
}

func auditStructFields(st *ast.StructType) []string {
	var names []string
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		if len(field.Names) == 0 {
			names = append(names, types.ExprString(field.Type))
		}
	}
	return names
}

// indexBodies records every call by its callee's name, with the function and
// closures around it, and every keyed field of a composite literal.
func (c *auditCensus) indexBodies(src *auditSource) {
	var fn *ast.FuncDecl
	var closures []*ast.FuncType
	var stack []ast.Node
	ast.Inspect(src.file, func(n ast.Node) bool {
		if n == nil {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch top.(type) {
			case *ast.FuncLit:
				closures = closures[:len(closures)-1]
			case *ast.FuncDecl:
				fn = nil
			}
			return true
		}
		stack = append(stack, n)
		switch node := n.(type) {
		case *ast.FuncDecl:
			fn, closures = node, nil
		case *ast.FuncLit:
			closures = append(closures, node.Type)
		case *ast.CallExpr:
			if name := auditCalleeName(node); name != "" {
				c.calls[name] = append(c.calls[name], auditCall{src: src, fn: fn, closures: slices.Clone(closures), call: node})
			}
			c.indexFilledArgs(src, fn, node)
		case *ast.CompositeLit:
			c.indexLiteral(src, fn, node)
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					c.indexFieldWrite(src, fn, sel, auditAssignedValue(node.Rhs, len(node.Lhs), i))
				}
			}
		case *ast.UnaryExpr:
			c.indexAddressTaken(src, fn, node)
		case *ast.IndexExpr:
			if c.isFuncName(src, node.X) {
				c.markWholeWrite(src, fn, node.Index)
			}
		case *ast.IndexListExpr:
			for _, index := range node.Indices {
				if c.isFuncName(src, node.X) {
					c.markWholeWrite(src, fn, index)
				}
			}
		}
		return true
	})
}

// indexFieldWrite records an assignment to x.F under x's type, or under this
// package's untyped F when x's type is unnamed. A nil value is a write this
// cannot read.
func (c *auditCensus) indexFieldWrite(src *auditSource, fn *ast.FuncDecl, sel *ast.SelectorExpr, value ast.Expr) {
	key := src.dir + ".*." + sel.Sel.Name
	if x, ok := sel.X.(*ast.Ident); ok {
		if typeDir, typeName, typed := c.typeOfLocal(src, fn, x.Name); typed {
			key = typeDir + "." + typeName + "." + sel.Sel.Name
		}
	}
	c.fields[key] = append(c.fields[key], auditLiteralField{src: src, fn: fn, expr: value})
}

// indexAddressTaken reads `&x.F` as an unreadable write to F.
func (c *auditCensus) indexAddressTaken(src *auditSource, fn *ast.FuncDecl, node *ast.UnaryExpr) {
	if x, ok := node.X.(*ast.SelectorExpr); ok && node.Op == token.AND {
		c.indexFieldWrite(src, fn, x, nil)
	}
}

// indexFilledArgs reads `&x` handed to a call as a write to every field of x's
// type. A scan or a decode fills what it is handed.
func (c *auditCensus) indexFilledArgs(src *auditSource, fn *ast.FuncDecl, call *ast.CallExpr) {
	for _, arg := range call.Args {
		unary, ok := arg.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			continue
		}
		if x, isIdent := unary.X.(*ast.Ident); isIdent {
			if typ, in := c.localType(src, fn, x.Name); typ != nil {
				c.markWholeWrite(in, fn, typ)
			}
		}
	}
}

// markWholeWrite records a struct type a call fills without naming its fields,
// such as a type argument to a row scanner.
func (c *auditCensus) markWholeWrite(src *auditSource, _ *ast.FuncDecl, typ ast.Expr) {
	if typeDir, typeName, ok := c.typeOf(src, typ); ok {
		c.wholeWrites[typeDir+"."+typeName] = true
	}
}

// isFuncName reports whether expr names a function, so an index on it is an
// instantiation. That is a name an import qualifies, or a function this package declares.
func (c *auditCensus) isFuncName(src *auditSource, expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.SelectorExpr:
		q, ok := x.X.(*ast.Ident)
		if !ok {
			return false
		}
		return src.qualifiers[q.Name]
	case *ast.Ident:
		_, declared := c.funcs[auditFuncKey{dir: src.dir, name: x.Name}]
		return declared
	}
	return false
}

// auditCalleeName is calleeName with an explicit instantiation unwrapped.
func auditCalleeName(call *ast.CallExpr) string {
	return calleeName(&ast.CallExpr{Fun: unindexed(call.Fun)})
}

func unindexed(fun ast.Expr) ast.Expr {
	for {
		switch f := fun.(type) {
		case *ast.IndexExpr:
			fun = f.X
		case *ast.IndexListExpr:
			fun = f.X
		case *ast.ParenExpr:
			fun = f.X
		default:
			return fun
		}
	}
}

func (c *auditCensus) indexLiteral(src *auditSource, fn *ast.FuncDecl, lit *ast.CompositeLit) {
	c.indexFields(src, fn, lit.Type, lit)
	var elem ast.Expr
	switch t := lit.Type.(type) {
	case *ast.ArrayType:
		elem = t.Elt
	case *ast.MapType:
		elem = t.Value
	}
	for _, elt := range lit.Elts {
		if kv, isPair := elt.(*ast.KeyValueExpr); isPair {
			elt = kv.Value
		}
		if inner, elided := elt.(*ast.CompositeLit); elided && inner.Type == nil && elem != nil {
			c.indexFields(src, fn, elem, inner)
		}
	}
}

// indexFields records each keyed field of a struct literal of type typ. A
// positional literal writes every field unnamed.
func (c *auditCensus) indexFields(src *auditSource, fn *ast.FuncDecl, typ ast.Expr, lit *ast.CompositeLit) {
	typeDir, typeName, typed := c.typeOf(src, typ)
	if !typed {
		return
	}
	for _, elt := range lit.Elts {
		kv, keyed := elt.(*ast.KeyValueExpr)
		field, named := ast.Expr(nil), false
		if keyed {
			field, named = kv.Key, isIdentNamed(kv.Key, identName(kv.Key))
		}
		if !named {
			c.wholeWrites[typeDir+"."+typeName] = true
			return
		}
		key := typeDir + "." + typeName + "." + identName(field)
		c.fields[key] = append(c.fields[key], auditLiteralField{src: src, fn: fn, expr: kv.Value})
	}
}

func identName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// typeOf names a type expression as its package directory and type name.
func (c *auditCensus) typeOf(src *auditSource, expr ast.Expr) (string, string, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		return src.dir, t.Name, true
	case *ast.StarExpr:
		return c.typeOf(src, t.X)
	case *ast.IndexExpr:
		return c.typeOf(src, t.X)
	case *ast.IndexListExpr:
		return c.typeOf(src, t.X)
	case *ast.SelectorExpr:
		if q, ok := t.X.(*ast.Ident); ok {
			if dir, imported := src.imports[q.Name]; imported {
				return dir, t.Sel.Name, true
			}
		}
	}
	return "", "", false
}

// grow follows every writer's calls until no new writer appears.
func (c *auditCensus) grow() string {
	seed := auditFuncKey{dir: auditSeedDir, name: auditSeed}
	decl, ok := c.funcs[seed]
	if !ok {
		return "storekit no longer declares " + auditSeed + ", the INSERT this census grows its writers from"
	}
	index := auditParamIndex(decl, "entityType")
	if index < 0 {
		return auditSeed + " has no entityType parameter, so this census cannot tell which argument names the record type"
	}
	c.writers[seed] = index
	queue := []auditFuncKey{seed}
	seen := map[*ast.CallExpr]bool{}
	for len(queue) > 0 {
		writer := queue[0]
		queue = queue[1:]
		for _, at := range c.calls[writer.name] {
			if seen[at.call] || !c.reaches(at, writer) || c.writers[writer] >= len(at.call.Args) {
				continue
			}
			seen[at.call] = true
			if next, forwarded := c.visit(at, c.writers[writer]); forwarded {
				if _, known := c.writers[next.key]; !known {
					c.writers[next.key] = next.index
					queue = append(queue, next.key)
				}
			}
		}
	}
	return ""
}

type auditForward struct {
	key   auditFuncKey
	index int
}

// visit reads one writer call. A parameter handed straight on makes the
// enclosing function a writer; anything else is recorded as a site.
func (c *auditCensus) visit(at auditCall, index int) (auditForward, bool) {
	arg := at.call.Args[index]
	if p := paramBehind(at.fn, at.closures, arg, 0); p >= 0 {
		return auditForward{key: auditFuncKey{dir: at.src.dir, name: at.fn.Name.Name, method: at.fn.Recv != nil}, index: p}, true
	}
	site := auditWriteSite{
		pos: at.src.path + ":" + strconv.Itoa(gatekit.SourceFileSet().Position(at.call.Pos()).Line),
		src: at.src, call: at.call,
	}
	if at.fn != nil {
		site.fn = at.fn.Name.Name
	}
	values, ok := c.resolve(at.src, at.fn, at.closures, arg, 0)
	if ok {
		site.types = values
	} else {
		site.opaque = types.ExprString(arg)
	}
	c.sites = append(c.sites, site)
	return auditForward{}, false
}

// reaches reports whether this call names writer. A function is matched by its
// package. A method is matched by name in its own package, or anywhere if exported.
func (c *auditCensus) reaches(at auditCall, writer auditFuncKey) bool {
	return c.names(at.src, unindexed(at.call.Fun), writer)
}

// names reports whether the expression fun, in src, refers to writer.
func (c *auditCensus) names(src *auditSource, fun ast.Expr, writer auditFuncKey) bool {
	switch fun := fun.(type) {
	case *ast.Ident:
		return !writer.method && writer.dir == src.dir && fun.Name == writer.name
	case *ast.SelectorExpr:
		if fun.Sel.Name != writer.name {
			return false
		}
		if q, ok := fun.X.(*ast.Ident); ok {
			if dir, imported := src.imports[q.Name]; imported {
				return !writer.method && dir == writer.dir
			}
		}
		// A generated handler calls a store through an interface whose method
		// names a store's own, so its method calls name no writer.
		return writer.method && !src.generated && (writer.dir == src.dir || ast.IsExported(writer.name))
	}
	return false
}

// writersUsedAsValues records, as an opaque site, every writer this census
// sees named other than as the function a call makes.
func (c *auditCensus) writersUsedAsValues() {
	byName := map[string][]auditFuncKey{}
	for key := range c.writers {
		byName[key.name] = append(byName[key.name], key)
	}
	for _, src := range c.sources {
		called := map[ast.Expr]bool{}
		var fn *ast.FuncDecl
		ast.Inspect(src.file, func(n ast.Node) bool {
			var ref ast.Expr
			switch node := n.(type) {
			case *ast.FuncDecl:
				fn = node
				called[node.Name] = true
			case *ast.CallExpr:
				called[unindexed(node.Fun)] = true
			case *ast.SelectorExpr:
				called[node.Sel] = true
				ref = node
			case *ast.Ident:
				ref = node
			}
			if ref == nil || called[ref] {
				return true
			}
			for _, key := range byName[identName(auditSelectedName(ref))] {
				if c.names(src, ref, key) {
					c.sites = append(c.sites, auditWriteSite{
						pos: src.path + ":" + strconv.Itoa(gatekit.SourceFileSet().Position(ref.Pos()).Line),
						src: src, fn: funcName(fn), call: &ast.CallExpr{Fun: ref}, opaque: types.ExprString(ref), asValue: true,
					})
				}
			}
			return true
		})
	}
}

func auditSelectedName(expr ast.Expr) ast.Expr {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return sel.Sel
	}
	return expr
}

func funcName(fn *ast.FuncDecl) string {
	if fn == nil {
		return ""
	}
	return fn.Name.Name
}
