// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The two halves of automatic apply agree about which kinds it covers.
//
// A kind reaches an automatic apply through two independent places: the sweep's
// query, which decides what it comes looking for, and the applier's check,
// which decides what it admits. They are separate statements about one set, and
// a disagreement fails silently in whichever direction it went — the sweep
// wakes on rows it will always refuse, or it never looks for rows it would have
// applied and the feature simply does nothing.
//
// The admin-governed ladder adds a third: a kind may be governed and have no
// policy seam, in which case the applier correctly refuses it forever and
// nothing says the wiring was never done.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

const (
	autonomyFile      = "internal/modules/approvals/autonomy.go"
	autoApplyFile     = "internal/compose/autoapply.go"
	governedSeamsFile = "internal/compose/stageautopilotseam.go"
)

// Every admin-governed kind has a seam that can answer for it.
//
// governedKindPolicies panics at startup on a kind it cannot answer for, which
// is the right behaviour and the wrong place to find out: the panic happens in
// a deployed worker, not in CI. This finds it here.
func TestEveryAdminGovernedKindHasAPolicySeam(t *testing.T) {
	t.Parallel()
	governed := namedMapLiteralKeys(t, autonomyFile, "AdminGovernedAutoKinds")
	if len(governed) == 0 {
		t.Fatalf("found no AdminGovernedAutoKinds entries in %s: this census is "+
			"reading the wrong thing and would pass over any drift", autonomyFile)
	}
	seams := namedMapLiteralKeys(t, governedSeamsFile, "seams")
	if len(seams) == 0 {
		t.Fatalf("found no policy seams in %s: same problem, from the other side",
			governedSeamsFile)
	}
	// The seam map keys by a constant (deals.StageProgressionKind) while the
	// kind set keys by its string. Compared as the SUFFIX a constant name
	// shares with its value, so this notices a kind with no seam without
	// resolving identifiers across packages.
	for kind := range governed {
		if !someKeyNames(seams, kind) {
			t.Errorf("%q is in AdminGovernedAutoKinds but no seam in %s answers "+
				"for it: the applier refuses it forever, correctly, and nothing "+
				"in the tree says the wiring was never done", kind, governedSeamsFile)
		}
	}
}

// The sweep looks for exactly the kinds the applier will admit.
//
// Two functions, one set. The sweep's query calls AutoAppliableKinds; the
// applier tests AdminGovernedAutoKinds and AutoApplyKinds. If the query is ever
// pointed back at one ladder alone, the other ladder's kinds stop being swept
// and its feature goes quiet with every test still green.
func TestTheAutoApplySweepScansTheKindsTheApplierAdmits(t *testing.T) {
	t.Parallel()
	src := sourceOfGateSubject(t, autoApplyFile)
	if !strings.Contains(src, "approvals.AutoAppliableKinds()") {
		t.Errorf("%s no longer scans approvals.AutoAppliableKinds(): the sweep and "+
			"the applier read different sets, so one ladder's kinds are either "+
			"never looked for or never admitted", autoApplyFile)
	}
	// Both ladders must still be what the applier tests, or the union above is
	// wider than what it will accept and the sweep wakes on rows it refuses.
	for _, set := range []string{
		"approvals.AdminGovernedAutoKinds[", "approvals.AutoApplyKinds[",
	} {
		if !strings.Contains(src, set) {
			t.Errorf("%s no longer tests %s: a kind of that ladder would be swept "+
				"and then refused on every tick", autoApplyFile, set)
		}
	}
}

// sourceOfGateSubject reads a file this gate judges.
func sourceOfGateSubject(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(raw)
}

// namedMapLiteralKeys reads the keys of a named map literal, as written.
//
// A string key answers its value; a constant key answers its identifier name.
// Both are what the caller compares, and neither needs the type checker.
func namedMapLiteralKeys(t *testing.T, file, name string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	out := map[string]bool{}
	ast.Inspect(parsed, func(n ast.Node) bool {
		value, ok := n.(*ast.ValueSpec)
		if !ok || len(value.Names) != 1 || value.Names[0].Name != name {
			return true
		}
		for _, v := range value.Values {
			collectNamedMapKeys(v, out)
		}
		return true
	})
	// A short assignment (`seams := map[...]{...}`) is not a ValueSpec.
	ast.Inspect(parsed, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 {
			return true
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || ident.Name != name {
			return true
		}
		for _, v := range assign.Rhs {
			collectNamedMapKeys(v, out)
		}
		return true
	})
	return out
}

func collectNamedMapKeys(v ast.Expr, out map[string]bool) {
	lit, ok := v.(*ast.CompositeLit)
	if !ok {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch key := kv.Key.(type) {
		case *ast.BasicLit:
			if unquoted, err := strconv.Unquote(key.Value); err == nil {
				out[unquoted] = true
			}
		case *ast.Ident:
			out[key.Name] = true
		case *ast.SelectorExpr:
			out[key.Sel.Name] = true
		}
	}
}

// someKeyNames answers whether any key plausibly names this kind.
//
// A seam may key on the literal string or on the constant that holds it
// (deals.StageProgressionKind for "stage_progression"), so this compares the
// kind's words against the constant's, ignoring the underscores Go drops and
// the "Kind" suffix such constants carry. Comparing identifiers properly would
// need the type checker across two packages, which is a far heavier gate for
// an answer this shape already gives.
func someKeyNames(keys map[string]bool, kind string) bool {
	if keys[kind] {
		return true
	}
	squashed := strings.ReplaceAll(kind, "_", "")
	for key := range keys {
		trimmed := strings.TrimSuffix(key, "Kind")
		if strings.EqualFold(trimmed, squashed) || strings.EqualFold(key, squashed) {
			return true
		}
	}
	return false
}
