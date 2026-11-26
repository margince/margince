// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The tools the switch's census reaches through the MCP registry. A router
// that runs a family's tool is one of the family's surfaces. So every tool it
// runs is walked: as an operation where the contract serves one, and as a
// tool by itself where none does.

import (
	"go/ast"
	"go/constant"
	"go/types"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const agentsPath = modulePath + "/internal/modules/agents"

// routersBesideTheFamilies run tools of the families beside tools of other
// surfaces, so the census takes from them only the families' own tools.
var routersBesideTheFamilies = gatekit.Waive(map[string]string{
	"readWorkspace": "describes the vocabulary of every surface, the report blocks among them, beside the roster and pipelines",
})

// routedOperations are the tools the families' routers run that no
// operation serves. A router is a family's when one of its targets is the
// tool of an operation in ops. A router that also runs an operation outside
// the census fails unless it is declared. Gaining such a tool cannot take a
// family's router out of the census. Reading no router at all fails too.
func routedOperations(t *testing.T, g *switchGraph, paths map[string]any, ops []scopedOperation) []scopedOperation {
	t.Helper()
	routers := g.routerTargets()
	if len(routers) == 0 {
		t.Fatalf("found no routeTargets in %s, so the census read no tool router", agentsPath)
	}
	held := map[string]bool{}
	for _, op := range ops {
		held[op.tool] = op.tool != ""
	}
	byVerb := map[string][]scopedOperation{}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		for _, op := range operationsAt(paths, path, "") {
			byVerb[op.tool] = append(byVerb[op.tool], op)
		}
	}
	var out []scopedOperation
	for _, router := range slices.Sorted(maps.Keys(routers)) {
		targets := routers[router]
		if !slices.ContainsFunc(targets, func(verb string) bool { return held[verb] }) {
			continue
		}
		outside := slices.DeleteFunc(slices.Clone(targets), func(verb string) bool { return held[verb] || len(byVerb[verb]) == 0 })
		if len(outside) > 0 && !routersBesideTheFamilies.Waived(t, router) {
			t.Errorf("the router %s runs a tool of the switch's families and also %v, which no family holds. "+
				"Place those operations under a family's path, or declare the router in routersBesideTheFamilies", router, outside)
		}
		for _, verb := range targets {
			if held[verb] || len(byVerb[verb]) > 0 {
				continue
			}
			held[verb] = true
			out = append(out, scopedOperation{family: "run by the router " + router, id: verb, tool: verb, toolOnly: true})
		}
	}
	return out
}

// routerTargets reads, for each router of the agents package, the tools its
// routeTargets method names: the values of the map it returns.
func (g *switchGraph) routerTargets() map[string][]string {
	routers := map[string][]string{}
	for fn, decl := range g.decls {
		if g.pkgOf[fn] != agentsPath || decl.Recv == nil || fn.Name() != "routeTargets" {
			continue
		}
		info := g.infos[fn]
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if ok && len(ret.Results) == 1 {
				routers[receiverTypeName(decl)] = append(routers[receiverTypeName(decl)], g.mapValues(ret.Results[0], info)...)
			}
			return !ok
		})
	}
	return routers
}

// mapValues are the string constants a map literal holds, or that the
// package variable it names was declared with.
func (g *switchGraph) mapValues(expr ast.Expr, info *types.Info) []string {
	if ident, ok := ast.Unparen(expr).(*ast.Ident); ok {
		if v, ok := info.Uses[ident].(*types.Var); ok && g.inits[v].expr != nil {
			return g.mapValues(g.inits[v].expr, g.inits[v].info)
		}
	}
	lit, ok := ast.Unparen(expr).(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if value := info.Types[kv.Value].Value; value != nil && value.Kind() == constant.String {
			out = append(out, constant.StringVal(value))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// placedOn names where an operation of the census was found, for a message.
func placedOn(op scopedOperation) string {
	if op.toolOnly {
		return "tool " + op.tool
	}
	return strings.ToUpper(op.method) + " " + op.path
}
