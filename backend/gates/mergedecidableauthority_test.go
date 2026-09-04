// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Who may settle a duplicate pair has ONE answer, and the card must ask the
// same thing the write asks.
//
// Two surfaces decide it. The disposition endpoint refuses a caller who cannot
// write both records (ensurePairWritable → auth.EnsureWritable), and the
// Worklist card decides whether to offer the verbs at all
// (Store.DecidableForMerge). They are separate code paths because one runs at
// the write and the other while rendering a feed, and nothing structural stops
// the second from growing its own idea of authority.
//
// If it does, the failure is silent and lands on the reader: a card that offers
// a verb the endpoint refuses is the button that told a rep to try again
// forever, and a card that withholds one the endpoint would accept hides work
// from the person whose job it is.
//
// So DecidableForMerge must reach its answer through auth.WritableSubset — the
// same visibility-and-write-authority pair EnsureWritable asks, asked set-wise.
// Re-deriving it here from owner_id, record_grant or a role's row scope is what
// this gate refuses, whatever the re-derivation happens to conclude today.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const (
	decidableFile   = "internal/modules/people/mergeface.go"
	decidableMethod = "DecidableForMerge"
	decidableAsks   = "auth.WritableSubset"
)

// decidableForbidden are the ways a second answer gets written: the columns the
// authority rule lives in, and the auth internals that render it. Each is
// legitimate elsewhere in the tree — the point is that this method does not
// answer the question itself.
//
// Matched against CALLS and SQL STRINGS rather than the method's text, so a
// sentence in a comment explaining why the rule is not re-derived here does not
// read as the re-derivation it is describing.
var decidableForbidden = []string{
	"owner_id",
	"record_grant",
	"RowScope",
	"Unbounded",
	"writeAuthorityPredicate",
	"VisibleSubset",
}

func TestTheMergeCardAsksTheSameAuthorityTheWriteAsks(t *testing.T) {
	t.Parallel()
	body := methodBody(t, decidableFile, decidableMethod)

	// A CALL, not a mention. Asserting on the method's text would be satisfied
	// by a comment naming the helper while the code beneath it did something
	// else entirely — which is the one failure a gate over authority must not
	// have.
	if !callsQualified(body, decidableAsks) {
		t.Errorf("%s does not CALL %s.\n\n"+
			"The card's offer and the disposition endpoint's refusal have to be two readings of\n"+
			"one authority. Reaching the answer any other way lets the button and the write drift,\n"+
			"and the reader is the one who finds out.", decidableMethod, decidableAsks)
	}
	for _, forbidden := range decidableForbidden {
		if where := reachesFor(body, forbidden); where != "" {
			t.Errorf("%s %s %q, which is the authority rule being re-derived rather than asked.\n\n"+
				"auth.WritableSubset already answers visibility and write authority together. A second\n"+
				"spelling here is a second answer to one question, and the two will disagree.",
				decidableMethod, where, forbidden)
		}
	}
}

// callsQualified reports whether the body calls the named function, spelled
// `pkg.Name`.
func callsQualified(body *ast.BlockStmt, want string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if qualifiedCallee(call.Fun) == want {
			found = true
		}
		return !found
	})
	return found
}

// reachesFor reports how the body reaches for a forbidden name — as a call or
// identifier, or inside a SQL string — and returns "" when it does not.
func reachesFor(body *ast.BlockStmt, forbidden string) string {
	var how string
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if strings.Contains(qualifiedCallee(node.Fun), forbidden) {
				how = "calls"
			}
		case *ast.SelectorExpr:
			if strings.Contains(node.Sel.Name, forbidden) {
				how = "reads"
			}
		case *ast.Ident:
			if node.Name == forbidden {
				how = "names"
			}
		case *ast.BasicLit:
			// A column name only ever appears here inside SQL, which is the
			// re-derivation this gate is really about.
			if node.Kind == token.STRING && strings.Contains(node.Value, forbidden) {
				how = "queries"
			}
		}
		return how == ""
	})
	return how
}

// qualifiedCallee is calleeName's package-qualified sibling: this gate has to
// tell auth.WritableSubset from any other WritableSubset, and the bare name
// cannot.
func qualifiedCallee(fun ast.Expr) string {
	switch node := fun.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		if pkg, ok := node.X.(*ast.Ident); ok {
			return pkg.Name + "." + node.Sel.Name
		}
		return node.Sel.Name
	default:
		return ""
	}
}

// methodBody returns one method's body, so a gate can assert what the code
// does rather than what its comments say.
func methodBody(t *testing.T, file, method string) *ast.BlockStmt {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == method && fn.Body != nil {
			return fn.Body
		}
	}
	t.Fatalf("%s has no method %s — a gate whose subject moved reports PASS on a smaller tree", file, method)
	return nil
}
