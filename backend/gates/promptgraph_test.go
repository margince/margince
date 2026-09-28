// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The graph the prompt census walks: what counts as minting a prompt, which
// parameter carries one into a conduit, and how a conduit's callers become
// sites. promptcertified_test.go holds the census and the cases that pin it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// promptPackages is what one file calls the two packages a prompt is minted
// from. A package the file does not import is the empty string, which no
// selector can spell, so a literal of it never matches.
type promptPackages struct {
	model    string
	decision string
}

// promptShape is one kind of prompt literal: the type that carries it and the
// field the prompt text sits in.
type promptShape struct {
	pkg, typeName, field string
	// unkeyedMints says an UNKEYED literal of the type counts as a site. The
	// order of a decision.Question's fields puts Instructions second, so an
	// unkeyed one with two or more values carries it; the rule is deliberately
	// wider than that — any unkeyed Question with room for instructions — so a
	// reordered struct over-refuses rather than going unseen.
	unkeyedMints bool
}

func (p promptPackages) shapes() []promptShape {
	return []promptShape{
		// A system prompt is what makes a model call a SITE: a request without
		// one is a continuation of somebody else's, and a system string on its
		// own is prompt text nobody has sent yet.
		{pkg: p.model, typeName: "Request", field: "System"},
		// A decision question's instructions are its prompt. The criteria are
		// prompt too, but a question with criteria and no instructions is
		// asked of nothing — the instructions are what a site writes.
		{pkg: p.decision, typeName: "Question", field: "Instructions", unkeyedMints: true},
	}
}

// mintedPrompt reports whether a node mints a prompt, and the expression that
// supplies the prompt text when the syntax names one. It sees three spellings:
// a typed literal (model.Request{System: …}), a literal whose type is elided
// inside a map, slice or array of that type (map[string]decision.Question{"k":
// {Instructions: …}}), and — for a decision question — an unkeyed one. The
// elided form is how a decision request spells its questions, so a census that
// read typed literals alone would see none of them.
func mintedPrompt(n ast.Node, pkgs promptPackages) (ast.Expr, bool) {
	lit, ok := n.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	for _, shape := range pkgs.shapes() {
		if shape.pkg == "" {
			continue
		}
		if namesType(lit.Type, shape) {
			if value, mints := promptFieldOf(lit, shape); mints {
				return value, true
			}
			continue
		}
		// elementType is approvalsummarycopy_test.go's, which walks elided
		// literals for the same reason.
		if !namesType(elementType(lit.Type), shape) {
			continue
		}
		for _, elt := range lit.Elts {
			if kv, isKV := elt.(*ast.KeyValueExpr); isKV {
				elt = kv.Value
			}
			if inner := elidedLiteral(elt); inner != nil {
				if value, mints := promptFieldOf(inner, shape); mints {
					return value, true
				}
			}
		}
	}
	return nil, false
}

// namesType reports whether a type expression is pkg.TypeName, through a
// pointer.
func namesType(expr ast.Expr, shape promptShape) bool {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != shape.typeName {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == shape.pkg
}

// elidedLiteral is an element written with its type left out, `{…}` or `&{…}`.
func elidedLiteral(expr ast.Expr) *ast.CompositeLit {
	if unary, isUnary := expr.(*ast.UnaryExpr); isUnary && unary.Op == token.AND {
		expr = unary.X
	}
	if lit, isLit := expr.(*ast.CompositeLit); isLit && lit.Type == nil {
		return lit
	}
	return nil
}

// promptFieldOf finds a literal's prompt field. An unkeyed literal mints when
// its shape says so, and names no expression: the census cannot tell which
// position is the prompt without the struct, so such a literal is a site and
// never a conduit.
func promptFieldOf(lit *ast.CompositeLit, shape promptShape) (ast.Expr, bool) {
	for _, elt := range lit.Elts {
		kv, isKV := elt.(*ast.KeyValueExpr)
		if !isKV {
			return nil, shape.unkeyedMints && len(lit.Elts) >= 2
		}
		if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == shape.field {
			return kv.Value, true
		}
	}
	return nil, false
}

// systemParameterIndex reports which of the enclosing function's parameters
// supplies a prompt, by position, given the expression the literal fills its
// prompt field with. Such a function is a conduit: companybrief.groundedRequest
// holds ONE literal and serves every caller that hands it a prompt builder, and
// choiceQuestion holds one decision question for every site that asks one — so
// counting literals would count one site where there are several, and a new
// caller would add no literal at all.
//
// The prompt must BE a parameter or be built by CALLING one. A parameter merely
// mentioned in the expression — a language code, a name — is data the site chose
// for itself, not a prompt handed in from outside.
func systemParameterIndex(prompt ast.Expr, fn *ast.FuncDecl) (int, bool) {
	if prompt == nil || fn.Type.Params == nil {
		return 0, false
	}
	at := map[string]int{}
	position := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			at[name.Name] = position
			position++
		}
	}
	switch value := prompt.(type) {
	case *ast.Ident:
		index, fromParam := at[value.Name]
		return index, fromParam
	case *ast.CallExpr:
		called, isIdent := value.Fun.(*ast.Ident)
		if !isIdent {
			return 0, false
		}
		index, fromParam := at[called.Name]
		return index, fromParam
	}
	return 0, false
}

// constructedType reads the type a constructor call yields, from the
// constructor's own declared result rather than from its name.
func constructedType(expr ast.Expr, dir string, imports map[string]string, returns map[funcKey]string) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return returns[funcKey{dir: dir, name: fn.Name}]
	case *ast.SelectorExpr:
		pkg, isIdent := fn.X.(*ast.Ident)
		if !isIdent {
			return ""
		}
		if target, known := imports[pkg.Name]; known {
			return returns[funcKey{dir: target, name: fn.Sel.Name}]
		}
	}
	return ""
}

// systemParameterIndex reports which of the enclosing function's parameters
// supplies a request literal's system prompt, by position. Such a function is a
// conduit: companybrief.groundedRequest holds ONE literal and serves every
// caller that hands it a prompt builder, so counting literals would count one
// site where there are several, and a new caller would add no literal at all.
//

// forwardedParameterPositions answers which argument positions of one call the
// caller fills with its own parameters — the shape that makes a wrapper stand
// in for whoever supplied the value.
func forwardedParameterPositions(n ast.Node, fn *ast.FuncDecl) map[int]bool {
	call, ok := n.(*ast.CallExpr)
	if !ok || fn.Type.Params == nil {
		return nil
	}
	params := map[string]bool{}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			params[name.Name] = true
		}
	}
	out := map[int]bool{}
	for position, arg := range call.Args {
		if ident, isIdent := arg.(*ast.Ident); isIdent && params[ident.Name] {
			out[position] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// propagateConduits makes every caller of a conduit a site in its own right,
// to a fixed point so a conduit reached through another still lands on the
// function that actually chose the prompt.
func propagateConduits(g *promptGraph) {
	callers := map[funcKey][]funcKey{}
	for from, tos := range g.calls {
		for _, to := range tos {
			callers[to] = append(callers[to], from)
		}
	}
	queue := make([]funcKey, 0, len(g.conduits))
	for c := range g.conduits {
		queue = append(queue, c)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, caller := range callers[cur] {
			if g.minting[caller] {
				continue
			}
			g.minting[caller] = true
			// A wrapper that FORWARDS its own parameter into a conduit is a
			// conduit too. Without this the walk stops at the wrapper and the
			// function that actually chose the prompt is never counted.
			if at, known := g.promptArg[cur]; known && g.forwards[caller][cur][at] {
				g.conduits[caller] = true
			}
			// A site is never its own certification, whether the literal is its
			// own or a conduit's. collectFunc refuses that for a direct minter;
			// a conduit's caller is a site by the same reasoning and has to be
			// refused by the same rule, or a builder written into a
			// certification file certifies itself through the conduit.
			delete(g.roots, caller)
			if g.conduits[caller] {
				queue = append(queue, caller)
			}
		}
	}
}

// graphOf parses one source string into a graph, for the cases above.
func graphOf(t *testing.T, src, name, dir string, isCert bool) promptGraph {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parsing the case source: %v", err)
	}
	g := promptGraph{
		minting:   map[funcKey]bool{},
		calls:     map[funcKey][]funcKey{},
		roots:     map[funcKey]bool{},
		conduits:  map[funcKey]bool{},
		returns:   map[funcKey]string{},
		promptArg: map[funcKey]int{},
		forwards:  map[funcKey]map[funcKey]map[int]bool{},
	}
	collectFile(&g, file, dir, isCert)
	return g
}
