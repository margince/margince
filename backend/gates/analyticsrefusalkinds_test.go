// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The contract's refusal kinds are exactly the kinds the engine constructs.
//
// A client branches on `details.kind`. A kind the engine builds and the enum
// lacks reaches a client that cannot name it; a kind the enum carries and
// nothing builds is a branch the client writes for a case that never comes. The
// Go side declares two kinds nothing builds, so the set is read from the
// constructions themselves — literals and `.Kind =` assignments — and a
// construction whose kind it cannot tie to a named constant fails the gate.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheRefusalKindEnumIsTheKindsTheEngineConstructs(t *testing.T) {
	t.Parallel()
	kinds := refusalKindConstants(t)
	census := refusalCensus{kinds: map[string]bool{}}
	err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		census.read(fset, file, kinds)
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	if len(census.unread) > 0 {
		t.Fatalf("RefusalError constructions this census cannot read — name the kind by its constant, "+
			"on the literal or on a variable the same function built:\n  %s", strings.Join(census.unread, "\n  "))
	}
	if len(census.kinds) == 0 {
		t.Fatal("no RefusalError construction found — this census reads nothing and agrees with everything")
	}
	constructed := slices.Sorted(maps.Keys(census.kinds))
	if declared := crmYAMLEnum(t, "AnalyticsRefusalDetails", "kind"); !slices.Equal(declared, constructed) {
		t.Errorf("AnalyticsRefusalDetails.kind declares %v but the engine constructs %v — "+
			"a kind on one side only is one a client cannot name, or a branch for a case that never comes",
			declared, constructed)
	}
}

// Each construction shape the census must read, and each it must refuse to
// guess at, planted as source.
func TestTheRefusalCensusSeesEveryConstructionShape(t *testing.T) {
	t.Parallel()
	kinds := refusalKindConstants(t)
	for _, plant := range []struct {
		shape, body, wantKind string
		wantUnread            bool
	}{
		{"a literal", `return &RefusalError{Kind: RefusalPrivacy}`, "privacy", false},
		{"a qualified literal", `return &analyticsquery.RefusalError{Kind: analyticsquery.RefusalInvalid}`, "invalid", false},
		{"new, then assigned", `r := new(RefusalError); r.Kind = RefusalTooExpensive; return r`, "too_expensive", false},
		{"a zero value, then assigned", `var r RefusalError; r.Kind = RefusalAmbiguous; return &r`, "ambiguous", false},
		{"a literal, then reassigned", `r := &RefusalError{Kind: RefusalInvalid}; r.Kind = RefusalAmbiguous; return r`, "ambiguous", false},
		{"a kind from a variable", `k := RefusalPrivacy; return &RefusalError{Kind: k}`, "", true},
		{"a literal that never gets a kind", `return &RefusalError{Message: "m"}`, "", true},
		{"new that never gets a kind", `r := new(RefusalError); return r`, "", true},
		{"a kind set through a field", `h.refusal.Kind = RefusalPrivacy; return nil`, "", true},
	} {
		src := "package p\nfunc f() error {\n" + strings.ReplaceAll(plant.body, "; ", "\n") + "\n}\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "plant.go", src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: the plant does not parse: %v", plant.shape, err)
		}
		census := refusalCensus{kinds: map[string]bool{}}
		census.read(fset, file, kinds)
		if got := len(census.unread) > 0; got != plant.wantUnread {
			t.Errorf("%s: unread=%v, want %v (%v)", plant.shape, got, plant.wantUnread, census.unread)
		}
		if plant.wantKind != "" && !census.kinds[plant.wantKind] {
			t.Errorf("%s: the census read %v and missed %q", plant.shape, census.kinds, plant.wantKind)
		}
	}
}

// refusalKindConstants maps each RefusalKind constant's name to its wire value,
// read from the declarations so a kind added there is known here.
func refusalKindConstants(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, value := range gatekit.PackageStringConstants(t, "internal/compose/analyticsquery") {
		if strings.HasPrefix(name, "Refusal") {
			out[name] = value
		}
	}
	if len(out) == 0 {
		t.Fatal("no Refusal* constant read out of analyticsquery — the kinds moved or were renamed")
	}
	return out
}

// refusalCensus is the kinds the read sources construct, and every
// construction it could not tie to a named kind.
type refusalCensus struct {
	kinds  map[string]bool
	unread []string
}

// refusalBinding is a variable one function holds a RefusalError in.
type refusalBinding struct {
	constructed, kindSet bool
	at                   token.Pos
}

func (c *refusalCensus) read(fset *token.FileSet, file *ast.File, kinds map[string]string) {
	for _, decl := range file.Decls {
		bound := map[string]*refusalBinding{}
		consumed := map[ast.Node]bool{}
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			bindRefusalParams(fn.Type.Params, bound)
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			c.visit(fset, n, kinds, bound, consumed)
			return true
		})
		for _, b := range bound {
			if b.constructed && !b.kindSet {
				c.unread = append(c.unread, fset.Position(b.at).String()+": a RefusalError never given a kind")
			}
		}
	}
}

func (c *refusalCensus) visit(
	fset *token.FileSet, n ast.Node, kinds map[string]string,
	bound map[string]*refusalBinding, consumed map[ast.Node]bool,
) {
	switch node := n.(type) {
	case *ast.AssignStmt:
		for i, lhs := range node.Lhs {
			if i >= len(node.Rhs) {
				break
			}
			if target, isKind := lhs.(*ast.SelectorExpr); isKind && target.Sel.Name == "Kind" {
				c.kindAssignment(fset, target, node.Rhs[i], kinds, bound)
				continue
			}
			if name, isIdent := lhs.(*ast.Ident); isIdent {
				if construction, kindless := refusalConstruction(node.Rhs[i]); construction != nil {
					consumed[construction] = true
					bound[name.Name] = &refusalBinding{constructed: kindless, at: node.Pos()}
				}
			}
		}
	case *ast.ValueSpec:
		if node.Type != nil && isRefusalErrorType(node.Type) {
			for _, name := range node.Names {
				bound[name.Name] = &refusalBinding{constructed: true, at: node.Pos()}
			}
		} else if star, isPtr := node.Type.(*ast.StarExpr); isPtr && isRefusalErrorType(star.X) {
			for _, name := range node.Names {
				bound[name.Name] = &refusalBinding{at: node.Pos()}
			}
		}
	case *ast.CompositeLit:
		if !isRefusalErrorType(node.Type) {
			return
		}
		kind, named, readable := literalKind(node, kinds)
		switch {
		case named && readable:
			c.kinds[kind] = true
		case named:
			c.unread = append(c.unread, fset.Position(node.Pos()).String()+": a Kind that is not a named kind")
		case !consumed[node]:
			c.unread = append(c.unread, fset.Position(node.Pos()).String()+": a RefusalError literal with no kind")
		}
	case *ast.CallExpr:
		if isNewRefusal(node) && !consumed[node] {
			c.unread = append(c.unread, fset.Position(node.Pos()).String()+": new(RefusalError) held nowhere")
		}
	}
}

// kindAssignment reads `x.Kind = …`. A receiver this function holds a
// RefusalError in is counted; a named kind assigned to anything else is a
// construction the census cannot tie to its type, so it is reported.
func (c *refusalCensus) kindAssignment(
	fset *token.FileSet, target *ast.SelectorExpr, value ast.Expr,
	kinds map[string]string, bound map[string]*refusalBinding,
) {
	kind, known := kinds[exprName(value)]
	var binding *refusalBinding
	if receiver, isIdent := target.X.(*ast.Ident); isIdent {
		binding = bound[receiver.Name]
	}
	switch {
	case binding != nil && known:
		binding.kindSet = true
		c.kinds[kind] = true
	case binding != nil:
		c.unread = append(c.unread, fset.Position(target.Pos()).String()+": a Kind that is not a named kind")
	case known:
		c.unread = append(c.unread, fset.Position(target.Pos()).String()+": a kind set on a receiver not tied to a RefusalError")
	}
}

func bindRefusalParams(params *ast.FieldList, bound map[string]*refusalBinding) {
	if params == nil {
		return
	}
	for _, field := range params.List {
		star, isPtr := field.Type.(*ast.StarExpr)
		if !isRefusalErrorType(field.Type) && (!isPtr || !isRefusalErrorType(star.X)) {
			continue
		}
		for _, name := range field.Names {
			bound[name.Name] = &refusalBinding{at: name.Pos()}
		}
	}
}

// refusalConstruction reports the RefusalError a right-hand side builds, and
// whether it is built with no kind.
func refusalConstruction(expr ast.Expr) (ast.Node, bool) {
	if unary, isAddr := expr.(*ast.UnaryExpr); isAddr {
		expr = unary.X
	}
	if lit, isLit := expr.(*ast.CompositeLit); isLit && isRefusalErrorType(lit.Type) {
		_, named, _ := literalKind(lit, nil)
		return lit, !named
	}
	if call, isCall := expr.(*ast.CallExpr); isCall && isNewRefusal(call) {
		return call, true
	}
	return nil, false
}

func isNewRefusal(call *ast.CallExpr) bool {
	fn, isIdent := call.Fun.(*ast.Ident)
	return isIdent && fn.Name == "new" && len(call.Args) == 1 && isRefusalErrorType(call.Args[0])
}

func isRefusalErrorType(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name == "RefusalError"
	case *ast.SelectorExpr:
		return typ.Sel.Name == "RefusalError"
	}
	return false
}

// literalKind reads a literal's Kind: whether it names one, and whether that
// name is a known kind constant.
func literalKind(lit *ast.CompositeLit, kinds map[string]string) (kind string, named, readable bool) {
	for _, elt := range lit.Elts {
		field, isKeyed := elt.(*ast.KeyValueExpr)
		if !isKeyed {
			continue
		}
		if key, isIdent := field.Key.(*ast.Ident); !isIdent || key.Name != "Kind" {
			continue
		}
		kind, readable = kinds[exprName(field.Value)]
		return kind, true, readable
	}
	return "", false, false
}

// AnalyticsRefusal restates Problem's envelope because the breaking-change
// check cannot read an allOf; the restatement must name the same members.
func TestTheRefusalEnvelopeIsTheProblemEnvelope(t *testing.T) {
	t.Parallel()
	schemas := crmYAMLSchemas(t)
	refusal := slices.Sorted(maps.Keys(schemas["AnalyticsRefusal"].Properties))
	problem := slices.Sorted(maps.Keys(schemas["Problem"].Properties))
	if len(problem) == 0 || !slices.Equal(refusal, problem) {
		t.Errorf("AnalyticsRefusal has %v and Problem has %v — one writer renders both", refusal, problem)
	}
}
