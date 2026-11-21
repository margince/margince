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
	// unreadInterfaces are interface methods that share a writer's name and
	// signature but that no module type is read to reach it through.
	unreadInterfaces []string
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
	// shift is 1 for a method expression, whose first argument is the
	// receiver.
	shift int
}

// auditIndex is the module, indexed for the fixed point.
type auditIndex struct {
	pkgs    []typedPackage
	uses    map[string][]auditFuncUse
	decls   map[string]*ast.FuncDecl
	declPkg map[*ast.FuncDecl]*typedPackage
	// ifaceMethods holds each interface method declared in the module by
	// its name, with the interface it belongs to.
	ifaceMethods map[string][]auditIfaceMethod
	// named holds every named type the module declares that is no interface.
	named []*types.Named
	// implementers caches, per interface, the named types implementing it.
	implementers map[*types.Interface][]*types.Named
	// fieldIndex is built on the first field read.
	fieldIndex *auditFields
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
		reached, unread := index.interfacesFor(next.key)
		for _, iface := range reached {
			push(auditDoor{iface, next.param})
		}
		census.unreadInterfaces = append(census.unreadInterfaces, unread...)
		for _, use := range index.uses[next.key] {
			if use.call == nil {
				census.asValue = append(census.asValue, auditRepoPos(use.pos))
				continue
			}
			site := auditEntitySite{pos: auditRepoPos(use.pos), writer: auditWriterName(use)}
			at := next.param + use.shift
			if at >= len(use.call.Args) {
				site.opaque = types.ExprString(use.call)
				census.sites = append(census.sites, site)
				continue
			}
			arg := use.call.Args[at]
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
		implementers: map[*types.Interface][]*types.Named{},
	}
	for i := range pkgs {
		pkg := &pkgs[i]
		for _, file := range pkg.files {
			for _, decl := range file.Decls {
				fn, _ := decl.(*ast.FuncDecl)
				if fn != nil {
					index.decls[auditFuncKey(pkg.info, fn)] = fn
					index.declPkg[fn] = pkg
				}
				index.indexUses(pkg, fn, decl)
			}
		}
		index.indexNamed(pkg.info)
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
				pkg: pkg, call: auditCallOf(stack), decl: fn, pos: node.Pos(), shift: methodExprShift(pkg.info, stack),
			})
		}
		return true
	})
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
	file, _, _ := strings.Cut(auditRepoPos(use.pos), ":")
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

// storekitTableAudit is the table the seed inserts into.
const storekitTableAudit = "audit_log"

func auditConstString(info *types.Info, expr ast.Expr) (string, bool) {
	tv, ok := info.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}
