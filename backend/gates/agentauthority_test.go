// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// An agent principal names the human whose authority it acts under, or says
// here why there is none.
//
// PD-002 is that a change names the PERSON behind it first, and the read path
// can only name a person the WRITE recorded. An audit row minted under
// actor_type='agent' with a NULL on_behalf_of therefore has nobody to name, and
// the audit screen renders it "No human authority recorded" — which is honest
// where it is TRUE and a silent gap where it is merely forgotten. From outside
// the two look identical.
//
// So the rule is not "always set it". Two shapes are legitimate and both are
// ratified below with their own reason. What this refuses is a THIRD arriving
// unremarked: a new caller that mints an agent, records no authority, and is
// discovered when somebody opens the audit log and finds a machine answerable
// to nobody.
//
// A CONSTRUCTION, not a comparison. Most mentions of principal.PrincipalAgent
// in this tree ask whether the current actor is one — `p.Type !=
// principal.PrincipalAgent` — and a census that counted those would report
// every gate and guard in the codebase as an unattributed writer. Only a
// literal that BUILDS a principal, or an assignment onto one, is a site that
// owes an answer.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// agentsWithNoHumanAuthority ratifies each site that mints an agent principal
// and records no human behind it, with the reason there is none to record.
var agentsWithNoHumanAuthority = gatekit.Waive(map[string]string{
	"internal/compose:extensionJobPrincipal": "a scheduled extension tick is work NOBODY requested, so there is no person to name and naming one would be the invention. Its own doc says why the type is still agent rather than system: auth.Require returns nil for PrincipalSystem before Permissions is consulted, so system would delete the second of the two locks on the governed core-write door, and auth.Unbounded would hand the tick RowScopeAll. The units that ship scheduled work do not act on this authority anyway — each lands its records through Runtime.Ingest, which resolves the member's live grants and discards what the tick carried",
})

// buildsAgentPrincipal reports whether this function CONSTRUCTS an agent
// principal: a `Type:` field set to principal.PrincipalAgent inside a composite
// literal, or an assignment of it onto an existing one.
//
// Both forms are in the tree and neither is reducible to the other — the
// scheduled-send path builds a human's principal first and promotes it, because
// the grants have to stay the human's ceiling.
func buildsAgentPrincipal(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Type" && isAgentConst(node.Value) {
				found = true
			}
		case *ast.AssignStmt:
			for _, rhs := range node.Rhs {
				if isAgentConst(rhs) {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// isAgentConst matches the qualified constant and nothing that merely ends in
// the same word: a local named PrincipalAgent would be a different thing.
func isAgentConst(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "PrincipalAgent" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "principal"
}

// namesTheAuthority reports whether the function mentions OnBehalfOf at all.
//
// MENTIONS, not "provably assigns a non-zero value". The scheduled-send path
// sets it only when the stored row carried one — a send scheduled before the
// column existed never recorded the authority and cannot be given one now — so
// a rule demanding an unconditional assignment would refuse a caller for
// handling the absence correctly. What this catches is a site that never
// considered the question at all.
func namesTheAuthority(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "OnBehalfOf" {
			found = true
		}
		return !found
	})
	return found
}

func TestEveryAgentPrincipalNamesItsHumanOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	defer agentsWithNoHumanAuthority.AssertAllMatched(t)

	fset := token.NewFileSet()
	built := 0
	for _, root := range []string{"internal/modules", "internal/compose", "internal/platform", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !buildsAgentPrincipal(fn) {
					continue
				}
				built++
				if namesTheAuthority(fn) {
					continue
				}
				key := filepath.ToSlash(filepath.Dir(path)) + ":" + fn.Name.Name
				if agentsWithNoHumanAuthority.Waived(t, key) {
					continue
				}
				t.Errorf("%s: %s mints an agent principal and never names OnBehalfOf — every audit "+
					"row it writes reads \"No human authority recorded\", which is indistinguishable "+
					"from a tick that genuinely had nobody behind it.\nSet it from whoever authorized "+
					"the work, or ratify this site in agentsWithNoHumanAuthority with the reason there "+
					"is no human to name.", path, fn.Name.Name)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// A census that judged nothing certifies nothing. Four functions build an
	// agent principal today — the passport's own identity, the automatic apply,
	// the scheduled send's promotion, and the extension tick — and a walk that
	// stopped recognising the construction would report a clean tree while
	// reading none of them.
	if built < 4 {
		t.Fatalf("this census found %d function(s) building an agent principal and expects at "+
			"least 4 — the walk has stopped recognising the construction rather than the tree "+
			"having lost them", built)
	}
}
