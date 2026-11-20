// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The record types the audit log is written about, read from the code that
// writes it, starting at storekit's insertAuditRows. A function that hands its
// own parameter to a writer's entity argument is itself a writer. Each other
// call's entity argument is followed to the values it can hold. One this cannot
// follow is returned as opaque, for the caller to refuse.

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"path/filepath"
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
	entity ast.Expr
	at     auditCall
	writer auditFuncKey // the writer the call reaches
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
	recv      string // a method's receiver type
}

// auditFuncKeyOf is the key fn is declared under.
func auditFuncKeyOf(src *auditSource, fn *ast.FuncDecl) auditFuncKey {
	key := auditFuncKey{dir: src.dir, name: fn.Name.Name, method: fn.Recv != nil}
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		if _, typeName, ok := (&auditCensus{}).typeOf(src, fn.Recv.List[0].Type); ok {
			key.recv = typeName
		}
	}
	return key
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
	typeNames   map[string]bool       // "dir.Type" for every declared type
	fieldTypes  map[string]auditConst // "dir.Type.field" → the field's declared type
	// mutated are the package-level names some use could change, hand on
	// or assign again, by "dir.name".
	mutated map[string]bool
	// pkgValues are the package-level consts and vars, by "dir.name".
	pkgValues map[string]bool
	methods   map[string][]auditFuncKey
	// writers holds each writer and every parameter position its entity
	// arguments arrive at.
	writers map[auditFuncKey]map[int]bool
	sites   []auditWriteSite
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

// repoAuditCensus is the census of this repository, read once for the whole
// gate run.
func repoAuditCensus(t *testing.T) *auditCensus {
	t.Helper()
	auditCensusOnce.Do(func() {
		auditCensusRead, auditCensusFailed = newAuditCensus(filepath.Join(gatekit.ModuleRoot(t), ".."))
	})
	if auditCensusFailed != "" {
		t.Fatal(auditCensusFailed)
	}
	return auditCensusRead
}

// auditWriteSites answers every Go call in this repository that writes an
// audit row.
func auditWriteSites(t *testing.T) []auditWriteSite {
	t.Helper()
	return repoAuditCensus(t).sites
}

// newAuditCensus reads the Go code under repo and grows its writers from
// storekit's seed. A census that could not be read says why.
func newAuditCensus(repo string) (*auditCensus, string) {
	c := &auditCensus{
		repo: repo, funcs: map[auditFuncKey]*ast.FuncDecl{},
		srcOf: map[*ast.FuncDecl]*auditSource{}, consts: map[string]map[string]auditConst{},
		structs: map[string][]string{}, fields: map[string][]auditLiteralField{}, wholeWrites: map[string]bool{},
		importers: map[string]map[string]bool{},
		calls:     map[string][]auditCall{}, writers: map[auditFuncKey]map[int]bool{},
		typeNames: map[string]bool{}, methods: map[string][]auditFuncKey{}, fieldTypes: map[string]auditConst{},
		mutated: map[string]bool{}, pkgValues: map[string]bool{},
	}
	if failed := c.load(); failed != "" {
		return c, failed
	}
	for _, src := range c.sources {
		c.indexBodies(src)
	}
	if failed := c.grow(); failed != "" {
		return c, failed
	}
	c.writersUsedAsValues()
	return c, ""
}

// packageMapKeys reads the string keys of a package-level map literal.
func (c *auditCensus) packageMapKeys(t *testing.T, dir, name string) []string {
	t.Helper()
	constant, ok := c.consts[dir][name]
	lit, isLiteral := constant.expr.(*ast.CompositeLit)
	if !ok || !isLiteral {
		t.Fatalf("%s no longer declares %s as a map literal", dir, name)
	}
	var keys []string
	for _, elt := range lit.Elts {
		kv, isPair := elt.(*ast.KeyValueExpr)
		values, resolved := []string(nil), false
		if isPair {
			values, resolved = c.resolve(constant.src, nil, nil, kv.Key, 0)
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
		if built, err := inDefaultBuild(path); err != nil || !built {
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

// inDefaultBuild reports whether a plain build for this platform compiles the
// file, by its name and its build constraint. One behind a test tag, such as the
// integration harness, is test code as a _test file is.
func inDefaultBuild(path string) (bool, error) {
	ctx := build.Default
	ctx.OpenFile = func(path string) (io.ReadCloser, error) {
		source, err := gatekit.ReadFile(path)
		return io.NopCloser(bytes.NewReader(source)), err
	}
	return ctx.MatchFile(filepath.Dir(path), filepath.Base(path))
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
						c.pkgValues[src.dir+"."+name.Name] = true
						if len(sp.Values) == len(sp.Names) {
							consts[name.Name] = auditConst{src: src, expr: sp.Values[i]}
						}
					}
				case *ast.TypeSpec:
					c.typeNames[src.dir+"."+sp.Name.Name] = true
					if st, isStruct := sp.Type.(*ast.StructType); isStruct {
						c.structs[src.dir+"."+sp.Name.Name] = auditStructFields(st)
						for _, field := range st.Fields.List {
							for _, name := range field.Names {
								c.fieldTypes[src.dir+"."+sp.Name.Name+"."+name.Name] = auditConst{src: src, expr: field.Type}
							}
						}
					}
				}
			}
		case *ast.FuncDecl:
			key := auditFuncKeyOf(src, d)
			c.funcs[key], c.srcOf[d] = d, src
			if key.method {
				c.methods[src.dir+"."+key.name] = append(c.methods[src.dir+"."+key.name], key)
			}
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
				switch target := lhs.(type) {
				case *ast.SelectorExpr:
					c.indexFieldWrite(src, fn, target, auditAssignedValue(node.Rhs, len(node.Lhs), i))
				case *ast.Ident:
					if node.Tok == token.ASSIGN && fn != nil {
						c.mutated[src.dir+"."+target.Name] = true
					}
				}
			}
		case *ast.UnaryExpr:
			c.indexAddressTaken(src, fn, node)
		case *ast.Ident:
			c.indexEscape(src.dir+"."+node.Name, node, stack[:len(stack)-1])
		case *ast.SelectorExpr:
			if q, isIdent := node.X.(*ast.Ident); isIdent {
				if dir, imported := src.imports[q.Name]; imported {
					c.indexEscape(dir+"."+node.Sel.Name, node, stack[:len(stack)-1])
				}
			}
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

// indexEscape marks a package-level name mutated when a use of it could change
// the collection it holds or hand it on.
func (c *auditCensus) indexEscape(key string, use ast.Node, stack []ast.Node) {
	if collectionEscapes(use, stack) {
		c.mutated[key] = true
	}
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
	queue := []auditForward{{key: seed, index: index}}
	c.writers[seed] = map[int]bool{index: true}
	type visited struct {
		call  *ast.CallExpr
		index int
	}
	seen := map[visited]bool{}
	for len(queue) > 0 {
		writer := queue[0]
		queue = queue[1:]
		for _, at := range c.calls[writer.key.name] {
			matched, shift := c.match(at.src, at.fn, unindexed(at.call.Fun), writer.key)
			arg := writer.index + shift
			if !matched || arg >= len(at.call.Args) || seen[visited{at.call, arg}] {
				continue
			}
			seen[visited{at.call, arg}] = true
			next, forwarded := c.visit(at, arg, writer.key)
			if !forwarded || c.writers[next.key][next.index] {
				continue
			}
			if c.writers[next.key] == nil {
				c.writers[next.key] = map[int]bool{}
			}
			c.writers[next.key][next.index] = true
			queue = append(queue, next)
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
func (c *auditCensus) visit(at auditCall, index int, writer auditFuncKey) (auditForward, bool) {
	arg := at.call.Args[index]
	if p := c.paramBehind(at.src, at.fn, at.closures, arg, 0); p >= 0 {
		return auditForward{key: auditFuncKeyOf(at.src, at.fn), index: p}, true
	}
	site := auditWriteSite{
		pos: at.src.path + ":" + strconv.Itoa(gatekit.SourceFileSet().Position(at.call.Pos()).Line),
		src: at.src, call: at.call, entity: arg, at: at, writer: writer,
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

// match reports whether fun, in src and fn, refers to writer, and how far a
// call through it shifts the writer's arguments. A method expression takes its
// receiver first. A function is matched by its package. A method is matched by
// its receiver's struct type where this can name it. Otherwise it is matched by
// name in its own package, or anywhere if exported.
func (c *auditCensus) match(src *auditSource, fn *ast.FuncDecl, fun ast.Expr, writer auditFuncKey) (bool, int) {
	switch fun := fun.(type) {
	case *ast.Ident:
		return !writer.method && writer.dir == src.dir && fun.Name == writer.name, 0
	case *ast.SelectorExpr:
		if fun.Sel.Name != writer.name {
			return false, 0
		}
		if typeDir, typeName, ok := c.methodExpressionType(src, fn, fun.X); ok {
			return writer.method && writer.dir == typeDir && writer.recv == typeName, 1
		}
		if q, ok := fun.X.(*ast.Ident); ok {
			if dir, imported := src.imports[q.Name]; imported {
				return !writer.method && dir == writer.dir, 0
			}
		}
		typeDir, typeName, typed := c.exprType(src, fn, fun.X, 0)
		if _, isStruct := c.structs[typeDir+"."+typeName]; typed && isStruct {
			return writer.method && writer.dir == typeDir && writer.recv == typeName, 0
		}
		// A generated handler calls a store through an interface whose method
		// names a store's own, so its method calls name no writer.
		return writer.method && !src.generated && (writer.dir == src.dir || ast.IsExported(writer.name)), 0
	}
	return false, 0
}

// exprType names the type of a local name, or of a field reached through one.
func (c *auditCensus) exprType(src *auditSource, fn *ast.FuncDecl, expr ast.Expr, depth int) (string, string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		return c.typeOfLocal(src, fn, x.Name)
	case *ast.SelectorExpr:
		typeDir, typeName, ok := c.exprType(src, fn, x.X, depth+1)
		field, declared := c.fieldTypes[typeDir+"."+typeName+"."+x.Sel.Name]
		if !ok || !declared || depth > auditResolveDepth {
			return "", "", false
		}
		return c.typeOf(field.src, field.expr)
	}
	return "", "", false
}

// methodExpressionType names the type a method expression such as Store.Write
// or (*pkg.Store).Write selects on.
func (c *auditCensus) methodExpressionType(src *auditSource, fn *ast.FuncDecl, x ast.Expr) (string, string, bool) {
	if paren, ok := x.(*ast.ParenExpr); ok {
		x = paren.X
	}
	if star, ok := x.(*ast.StarExpr); ok {
		x = star.X
	}
	switch t := x.(type) {
	case *ast.Ident:
		if typ, _ := c.localType(src, fn, t.Name); typ == nil && c.typeNames[src.dir+"."+t.Name] {
			return src.dir, t.Name, true
		}
	case *ast.SelectorExpr:
		if q, ok := t.X.(*ast.Ident); ok {
			if dir, imported := src.imports[q.Name]; imported && c.typeNames[dir+"."+t.Sel.Name] {
				return dir, t.Sel.Name, true
			}
		}
	}
	return "", "", false
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
				if matched, _ := c.match(src, fn, ref, key); matched {
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
