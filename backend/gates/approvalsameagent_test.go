// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// "Is this the agent that staged the proposal" has ONE spelling, and it is not
// passport equality.
//
// A connected agent's passport id does not survive its own token refresh: the
// rotation spends the presented token, retires the passport and mints a
// replacement under the same grant, so the agent comes back identical in every
// way that governs it and different in the one field three rules were comparing.
// The three disagreed about what that meant — the release refused nothing, while
// redemption and the poll refused the proposer its own proposal — which is the
// shape a reader cannot see from one call site.
//
// So the comparison lives in sameAgent and a fourth reader re-deriving it fails
// here rather than shipping a fourth answer. Approvals is the whole corpus
// because `row` is unexported: no other package can hold a staged proposal to
// compare against a caller in the first place.
//
// BOTH operands, which is what separates an identity comparison from a presence
// check. `p.PassportID != ids.Nil` and `a.PassportID == nil` ask whether there
// is a credential at all and stay legitimate anywhere; only a comparison of two
// PassportID-bearing expressions is claiming to answer who the agent is.
//
// Scope, deliberately: ==/!= is the shape the defect took and the shape this
// refuses. A map keyed by passport id, a slices.Contains over them, or a helper
// taking two of them would all re-derive the same identity and none of them
// fails here. Read this as a gate on the comparison, not as a general answer to
// "identity from a passport" — that one is still read by a human.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// mentionsPassportID reports whether an expression reaches a PassportID field
// anywhere inside it, so `*a.PassportID` and `ids.From[ids.PassportKind](p.PassportID)`
// both count as naming one.
func mentionsPassportID(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "PassportID" {
			found = true
		}
		return !found
	})
	return found
}

// comparesTwoPassports finds every ==/!= in a function whose BOTH sides name a
// passport — the comparison that is claiming to identify the agent.
func comparesTwoPassports(fn *ast.FuncDecl) []token.Pos {
	var at []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		cmp, ok := n.(*ast.BinaryExpr)
		if !ok || (cmp.Op != token.EQL && cmp.Op != token.NEQ) {
			return true
		}
		if mentionsPassportID(cmp.X) && mentionsPassportID(cmp.Y) {
			at = append(at, cmp.OpPos)
		}
		return true
	})
	return at
}

func TestTheProposersIdentityHasOneSpelling(t *testing.T) {
	t.Parallel()

	const root = "internal/modules/approvals"
	fset := token.NewFileSet()
	compared, permitted := 0, 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		path = filepath.ToSlash(path)
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			for _, pos := range comparesTwoPassports(fn) {
				compared++
				if fn.Name.Name == "sameAgent" {
					permitted++
					continue
				}
				t.Errorf("%s: %s compares one passport against another, which stops being the "+
					"agent's identity the moment its client refreshes — the rotation mints a new "+
					"passport under the same connection. Ask sameAgent instead.",
					fset.Position(pos), fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	// The floor is the PERMITTED instance, not merely some instance. A
	// prohibition is a census of offenders, and under-reading is the one
	// direction it must not fail in: a walk that resolved the wrong directory or
	// lost the files to a build tag finds nothing, reports PASS, and leaves no
	// failing assertion to notice. sameAgent holds the one comparison that is
	// supposed to be here, so seeing it is how this proves it reached the code —
	// and it makes the exemption self-checking, since exempting by function name
	// would otherwise quietly exempt nothing once that function is renamed.
	if permitted == 0 {
		t.Fatalf("this gate never reached sameAgent's own passport comparison (%d found in total), "+
			"so it has stopped reading the code it polices rather than finding it clean", compared)
	}
}
