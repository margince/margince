// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// One function publishes an AI Router binding, and it is install.
//
// A binding carries the generation the result cache scopes its entries to. The
// stamp is what closes a rebind's window: a completion already in flight
// finishes on the binding it loaded and writes its answer AFTER the rebind
// cleared the cache, so the clear alone cannot keep the old binding's words out
// of the new binding's answers — and RouteInfo names the provider and model of
// whichever binding serves a hit. An operator reads that identity to tell "no
// AI provider is configured" apart from "the model answered badly".
//
// The field is unexported, so only package ai can publish one — and it does so
// from four places, one of them a test helper. A fifth that stored a binding
// directly would reuse the generation it replaced, every stale entry would stay
// readable, and nothing would fail: the cache would go on answering, just with
// the wrong attribution. That is the shape this gate exists for.
//
// WHAT IT CANNOT SEE. A publisher that reached the field through a local
// variable or a second atomic pointer. Both are outside the syntax this reads,
// and neither is a thing the package has ever done.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// bindingPublisher names the function a binding may be stored from; a store
// anywhere else is this gate's finding.
const bindingPublisher = "install"

// bindingPublisherScope claims binding publication lives in the ai module and
// nowhere else. Nothing is exempt: the field is unexported, so no file outside
// that package can reach it and an entry here would be proof the sweep is
// reading the wrong tree.
var bindingPublisherScope = gatekit.Scope{
	Roots:   []string{"internal/modules/ai"},
	Subject: publishesABinding,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestOnlyOneFunctionPublishesARouterBinding(t *testing.T) {
	t.Parallel()
	var offenders []string
	for _, file := range bindingPublisherScope.Files(t) {
		for _, fn := range publishersIn(file.File) {
			if fn != bindingPublisher {
				offenders = append(offenders, file.Path+": "+fn)
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("a Router binding is published outside %s:\n\t%s\n\n"+
			"Publish through it instead. It stamps the generation the result cache scopes its entries "+
			"to, and a binding stored without a fresh one leaves every answer the replaced binding "+
			"produced readable — served afterwards under the provider and model now bound",
			bindingPublisher, strings.Join(offenders, "\n\t"))
	}
}

// publishesABinding reports whether a file stores into the Router's binding
// pointer at all — the sweep's subject, asked the same inside the root and out.
func publishesABinding(_ string, file *ast.File) bool {
	return len(publishersIn(file)) > 0
}

// publishersIn names every function in a file that stores into a `bound`
// pointer, in declaration order. Repeated stores inside a function collapse to
// a single entry — the gate judges the function, not each statement.
func publishersIn(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if storesIntoBound(fn.Body) {
			names = append(names, fn.Name.Name)
		}
	}
	return names
}

// storesIntoBound reports whether a body calls Store on a selector named
// `bound` — `r.bound.Store(...)`, whatever the receiver is spelled.
func storesIntoBound(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		store, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || store.Sel.Name != "Store" {
			return true
		}
		target, ok := store.X.(*ast.SelectorExpr)
		if ok && target.Sel.Name == "bound" {
			found = true
			return false
		}
		return true
	})
	return found
}
