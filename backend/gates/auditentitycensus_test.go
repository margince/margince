// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The record types Go code writes audit rows about, read on the type-checked
// module. The census starts at storekit's insertAuditRows, the one Go INSERT
// into audit_log. A function that hands its own parameter on as a writer's
// record type is itself a writer. So is an interface method a writer
// implements. Any other argument is read to its values, or returned as opaque
// for the caller to judge.

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	auditSeedPackage = modulePath + "/internal/platform/database/storekit"
	auditSeedFunc    = auditSeedPackage + ".insertAuditRows"
	auditSeedParam   = "entityType"
)

// auditEntitySite is one call to an audit writer, and what its record type is.
type auditEntitySite struct {
	pos    string // file:line under the repository root
	writer string // file:function of the code making the call
	value  string // the constant it passes
	opaque string // the argument as written, when it is no constant
}

// auditEntityCensus is what the census read.
type auditEntityCensus struct {
	sites []auditEntitySite
	// asValue are the places a writer is named but not called. Calls made
	// through such a value are not read.
	asValue []string
	// directInserts are SQL INSERTs into audit_log outside the seed.
	directInserts []string
	// writers holds the writers found, by full name.
	writers map[string]bool
	// seedInserts is whether the seed still holds the INSERT this census
	// starts from, with the parameter it reads.
	seedInserts bool
}

var auditEntityCensusOnce sync.Once

var auditEntityCensusValue *auditEntityCensus

func auditEntityCensusOf(t *testing.T) *auditEntityCensus {
	t.Helper()
	pkgs := typedModule(t)
	auditEntityCensusOnce.Do(func() { auditEntityCensusValue = readAuditEntityCensus(pkgs, auditSeedFunc) })
	return auditEntityCensusValue
}

// auditFuncUse is one place a function of this module is named.
type auditFuncUse struct {
	pkg  *typedPackage
	call *ast.CallExpr // nil when the name is not the function of a call
	decl *ast.FuncDecl // the declaration around it; nil at package level
	pos  token.Pos
}

// auditIndex is the module, indexed for the fixed point.
type auditIndex struct {
	pkgs    []typedPackage
	uses    map[string][]auditFuncUse
	decls   map[string]*ast.FuncDecl
	declPkg map[*ast.FuncDecl]*typedPackage
	// methods holds each interface method declared in the module by its
	// name, with the interface it belongs to.
	ifaceMethods map[string][]auditIfaceMethod
	// methods holds every method declared in the module by its name.
	methods map[string][]string
}

type auditIfaceMethod struct {
	key   string
	iface *types.Interface
}

// readAuditEntityCensus reads pkgs from seed, the function holding the INSERT.
func readAuditEntityCensus(pkgs []typedPackage, seed string) *auditEntityCensus {
	index := indexAuditModule(pkgs)
	census := &auditEntityCensus{writers: map[string]bool{}}
	var queue []auditDoor
	seen := map[auditDoor]bool{}
	push := func(d auditDoor) {
		if !seen[d] {
			seen[d] = true
			census.writers[d.key] = true
			queue = append(queue, d)
		}
	}
	if decl := index.decls[seed]; decl != nil {
		if at := auditParamIndex(index.declPkg[decl].info, decl, auditSeedParam); at >= 0 {
			push(auditDoor{seed, at})
		}
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, iface := range index.interfacesFor(next.key) {
			push(auditDoor{iface, next.param})
		}
		for _, use := range index.uses[next.key] {
			if use.call == nil {
				census.asValue = append(census.asValue, auditRepoPos(use.pos))
				continue
			}
			site := auditEntitySite{pos: auditRepoPos(use.pos), writer: auditWriterName(use)}
			if next.param >= len(use.call.Args) {
				site.opaque = types.ExprString(use.call)
				census.sites = append(census.sites, site)
				continue
			}
			arg := use.call.Args[next.param]
			site.pos = auditRepoPos(arg.Pos())
			read, ok := index.resolver(use).resolve(arg)
			if !ok {
				site.opaque = types.ExprString(arg)
				census.sites = append(census.sites, site)
				continue
			}
			for _, value := range read.values {
				site.value = value
				census.sites = append(census.sites, site)
			}
			for _, forwarded := range read.params {
				push(forwarded)
			}
		}
	}
	census.directInserts, census.seedInserts = directAuditInserts(index, seed)
	census.seedInserts = census.seedInserts && census.writers[seed]
	return census
}

func indexAuditModule(pkgs []typedPackage) *auditIndex {
	index := &auditIndex{
		pkgs: pkgs,
		uses: map[string][]auditFuncUse{}, decls: map[string]*ast.FuncDecl{},
		declPkg: map[*ast.FuncDecl]*typedPackage{}, ifaceMethods: map[string][]auditIfaceMethod{},
		methods: map[string][]string{},
	}
	for i := range pkgs {
		pkg := &pkgs[i]
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, _ := decl.(*ast.FuncDecl)
				if fn != nil {
					key := auditFuncKey(pkg.info, fn)
					index.decls[key] = fn
					index.declPkg[fn] = pkg
					if fn.Recv != nil {
						index.methods[fn.Name.Name] = append(index.methods[fn.Name.Name], key)
					}
				}
				index.indexUses(pkg, fn, decl)
			}
		}
	}
	return index
}

// indexUses records every name in decl that refers to a function or method of
// this module, and every interface method declared there.
func (x *auditIndex) indexUses(pkg *typedPackage, fn *ast.FuncDecl, decl ast.Decl) {
	var stack []ast.Node
	ast.Inspect(decl, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		switch node := n.(type) {
		case *ast.InterfaceType:
			x.indexInterface(pkg.info, node)
		case *ast.Ident:
			used, ok := pkg.info.Uses[node].(*types.Func)
			if !ok || used.Pkg() == nil || !strings.HasPrefix(used.Pkg().Path(), modulePath) {
				return true
			}
			key := used.Origin().FullName()
			x.uses[key] = append(x.uses[key], auditFuncUse{
				pkg: pkg, call: auditCallOf(stack), decl: fn, pos: node.Pos(),
			})
		}
		return true
	})
}

func (x *auditIndex) indexInterface(info *types.Info, node *ast.InterfaceType) {
	iface, ok := info.Types[node].Type.(*types.Interface)
	if !ok {
		return
	}
	for i := range iface.NumExplicitMethods() {
		method := iface.ExplicitMethod(i)
		x.ifaceMethods[method.Name()] = append(x.ifaceMethods[method.Name()], auditIfaceMethod{method.FullName(), iface})
	}
}

// interfacesFor lists the interface methods a call could reach the writer
// through. Each has the writer's name, and the writer's receiver has every
// method of its interface with the same signature.
func (x *auditIndex) interfacesFor(key string) []string {
	has := x.receiverMethods(key)
	if has == nil {
		return nil
	}
	var out []string
	for _, candidate := range x.ifaceMethods[x.decls[key].Name.Name] {
		if auditImplementsBySignature(has, candidate.iface) {
			out = append(out, candidate.key)
		}
	}
	return out
}

// implementations lists the module methods named name whose receiver
// implements iface.
func (x *auditIndex) implementations(name string, iface *types.Interface) []string {
	var out []string
	for _, key := range x.methods[name] {
		if has := x.receiverMethods(key); has != nil && auditImplementsBySignature(has, iface) {
			out = append(out, key)
		}
	}
	return out
}

// receiverMethods spells the method set of the pointer to the receiver of the
// method declared under key, or nil for a function.
func (x *auditIndex) receiverMethods(key string) map[string]string {
	decl := x.decls[key]
	if decl == nil || decl.Recv == nil {
		return nil
	}
	obj, ok := x.declPkg[decl].info.Defs[decl.Name].(*types.Func)
	if !ok {
		return nil
	}
	recv := obj.Signature().Recv().Type()
	if ptr, isPtr := recv.(*types.Pointer); isPtr {
		recv = ptr.Elem()
	}
	return auditMethodSignatures(types.NewPointer(recv))
}

// auditMethodSignatures spells each method by name and signature. Packages checked
// apart hold distinct type objects, so a match is read off the spelling.
func auditMethodSignatures(t types.Type) map[string]string {
	set := types.NewMethodSet(t)
	out := make(map[string]string, set.Len())
	for i := range set.Len() {
		fn := set.At(i).Obj()
		out[fn.Name()] = auditSignature(fn.Type())
	}
	return out
}

// auditSignature spells a method's parameter and result types without their
// names, which an interface and its implementation need not share.
func auditSignature(t types.Type) string {
	sig, ok := t.(*types.Signature)
	if !ok {
		return types.TypeString(t, nil)
	}
	spell := func(tuple *types.Tuple) string {
		parts := make([]string, tuple.Len())
		for i := range tuple.Len() {
			parts[i] = types.TypeString(tuple.At(i).Type(), nil)
		}
		return strings.Join(parts, ", ")
	}
	return "(" + spell(sig.Params()) + ") (" + spell(sig.Results()) + ") " + strconv.FormatBool(sig.Variadic())
}

func auditImplementsBySignature(has map[string]string, iface *types.Interface) bool {
	for i := range iface.NumMethods() {
		method := iface.Method(i)
		if has[method.Name()] != auditSignature(method.Type()) {
			return false
		}
	}
	return true
}

// auditCallOf is the call whose function the innermost name on stack is.
func auditCallOf(stack []ast.Node) *ast.CallExpr {
	at := len(stack) - 1
	child := stack[at]
	for at--; at >= 0; at-- {
		switch parent := stack[at].(type) {
		case *ast.SelectorExpr:
			if parent.Sel != child {
				return nil
			}
		case *ast.IndexExpr, *ast.IndexListExpr, *ast.ParenExpr:
		case *ast.CallExpr:
			if parent.Fun == child {
				return parent
			}
			return nil
		default:
			return nil
		}
		child = stack[at]
	}
	return nil
}

func auditParamIndex(info *types.Info, decl *ast.FuncDecl, name string) int {
	obj, ok := info.Defs[decl.Name].(*types.Func)
	if !ok {
		return -1
	}
	params := obj.Signature().Params()
	for i := range params.Len() {
		if params.At(i).Name() == name {
			return i
		}
	}
	return -1
}

func auditFuncKey(info *types.Info, decl *ast.FuncDecl) string {
	if obj, ok := info.Defs[decl.Name].(*types.Func); ok {
		return obj.Origin().FullName()
	}
	return decl.Name.Name
}

// auditWriterName is the file and function a use sits in, the key a declared
// vocabulary is listed under.
func auditWriterName(use auditFuncUse) string {
	file := strings.SplitN(auditRepoPos(use.pos), ":", 2)[0]
	if use.decl == nil {
		return file + ":"
	}
	return file + ":" + use.decl.Name.Name
}

// auditRepoPos spells pos as file:line under the repository root.
func auditRepoPos(pos token.Pos) string {
	at := gatekit.SourceFileSet().Position(pos)
	return "backend/" + filepath.ToSlash(at.Filename) + ":" + strconv.Itoa(at.Line)
}

// auditInsertOpen finds SQL that ends where its INSERT's table is spliced in.
var auditInsertOpen = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(?:"?public"?\.)?"?$`)

// auditInsertFormat finds a format string that names its INSERT's table
// through a verb.
var auditInsertFormat = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(?:"?public"?\.)?"?%`)

// directAuditInserts finds each INSERT into audit_log outside the seed. That
// is a constant holding one, SQL that splices or formats "audit_log" in as its
// table, or a pgx.Identifier naming the table. The type
// checker folds a concatenation of constants, so the outermost constant
// expression is the one read.
func directAuditInserts(index *auditIndex, seed string) (found []string, seedInserts bool) {
	for i := range index.pkgs {
		pkg := &index.pkgs[i]
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, _ := decl.(*ast.FuncDecl)
				r := &auditResolver{index: index, pkg: pkg, decl: fn, visiting: map[types.Object]bool{}}
				inserts := r.directInserts(decl)
				if fn != nil && auditFuncKey(pkg.info, fn) == seed {
					seedInserts = len(inserts) > 0
					continue
				}
				found = append(found, inserts...)
			}
		}
	}
	return found, seedInserts
}

func (r *auditResolver) directInserts(decl ast.Decl) []string {
	var found []string
	info := r.pkg.info
	ast.Inspect(decl, func(n ast.Node) bool {
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if value, isConst := auditConstString(info, expr); isConst {
			// A constant's name is read where its value is written, not at
			// each use.
			if _, named := ast.Unparen(expr).(*ast.Ident); named {
				return false
			}
			if _, named := ast.Unparen(expr).(*ast.SelectorExpr); named {
				return false
			}
			if auditInsertAnywhere.MatchString(value) {
				found = append(found, auditRepoPos(expr.Pos()))
			}
			return false
		}
		if r.splicesAuditLog(expr) {
			found = append(found, auditRepoPos(expr.Pos()))
		}
		return true
	})
	return found
}

// splicesAuditLog reports whether expr builds an INSERT, or a table name,
// from a value that can be "audit_log".
func (r *auditResolver) splicesAuditLog(expr ast.Expr) bool {
	info := r.pkg.info
	names := func(e ast.Expr) bool {
		read, _ := r.resolve(e)
		return slices.Contains(read.values, storekitTableAudit)
	}
	switch node := expr.(type) {
	case *ast.BinaryExpr:
		left, ok := auditConstString(info, node.X)
		return node.Op == token.ADD && ok && auditInsertOpen.MatchString(left) && names(node.Y)
	case *ast.CallExpr:
		formatted := false
		for _, arg := range node.Args {
			if value, ok := auditConstString(info, arg); ok && auditInsertFormat.MatchString(value) {
				formatted = true
			} else if formatted && names(arg) {
				return true
			}
		}
	case *ast.CompositeLit:
		if tv, ok := info.Types[node]; ok && types.TypeString(tv.Type, nil) == "github.com/jackc/pgx/v5.Identifier" {
			return slices.ContainsFunc(node.Elts, names)
		}
	}
	return false
}

// storekitTableAudit is the table the seed inserts into.
const storekitTableAudit = "audit_log"

func auditConstString(info *types.Info, expr ast.Expr) (string, bool) {
	tv, ok := info.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}
