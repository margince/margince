// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A MailboxProof is set in exactly one place, and that place spends the token
// that earns it.
//
// The proof lets a marketing grant complete without a confirmation mail. What
// makes that defensible is not the constant — it is that the caller redeemed a
// single-use link delivered to the subject's own mailbox. Set the field anywhere
// else and the type becomes a string a caller can assert, which is the whole of
// the protection gone: a handler building a RecordInput from a request body
// could grant marketing consent for anybody.
//
// So this counts the assignments. One, in the confirm submit, on the far side of
// spendConfirmTokenTx.
//
// WHAT IT CANNOT SEE: a RecordInput built by copying another struct that already
// carries a proof. Nothing in this tree does that, and the shape of the type —
// no constructor, no setter — is what keeps it awkward.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// theMailboxProofWriter names the function this gate permits, and the file it
// lives in. Named rather than derived, because which site is permitted is a
// judgment about what that site does — it assigns on the far side of a spend —
// and not something a walk could work out.
const (
	theMailboxProofWriter     = "recordMarketingAnswerTx"
	theMailboxProofSpender    = "SubmitConfirmation"
	theMailboxProofWriterFile = "internal/modules/consent/confirmsubmit.go"
)

func TestOnlyTheConfirmSubmitClaimsAProvenMailbox(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return assignsMailboxProof(file) },
	}
	var elsewhere []string
	var permitted *ast.File
	for _, f := range scope.Files(t) {
		if f.Path == theMailboxProofWriterFile {
			permitted = f.File
			continue
		}
		elsewhere = append(elsewhere, f.Path)
	}
	if permitted == nil {
		t.Fatalf("%s assigns no MailboxProof — either the claim moved and this gate now certifies "+
			"nothing, or the field is unused and the type should go", theMailboxProofWriterFile)
	}
	if len(elsewhere) > 0 {
		t.Errorf("MailboxProof is set outside %s:\n\t%s\n\n"+
			"The proof is what lets a marketing grant complete with no confirmation mail, and it is "+
			"honest only where the caller has just spent the single-use link that earns it. A second "+
			"writer makes it a claim any caller can assert — including one building a RecordInput from "+
			"a request body, which would grant marketing consent for anybody.",
			theMailboxProofWriterFile, strings.Join(elsewhere, "\n\t"))
	}
	if !writerStillSpendsTheToken(t, permitted) {
		t.Errorf("%s no longer reaches %s from %s — the proof is only true because the link was "+
			"redeemed, so an assignment the spend does not reach is a claim with nothing behind it",
			theMailboxProofWriterFile, theMailboxProofWriter, theMailboxProofSpender)
	}
}

// assignsMailboxProof reports whether a file sets the field on a composite
// literal or by assignment. Both spellings, because a gate that reads only the
// literal form is defeated by two lines.
func assignsMailboxProof(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && key.Name == "MailboxProof" {
				found = true
			}
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "MailboxProof" {
					found = true
				}
			}
		}
		return !found
	})
	return found
}

// writerStillSpendsTheToken holds the other half: the assignment is earned
// BECAUSE the transaction carrying it spent the link first.
//
// It walks rather than greps, and the difference is the whole value. A
// substring search over the file passes on a leftover comment naming the spend,
// and passes again if the assignment moves into a third function that the spend
// never reaches — which is exactly the shape of the regression it exists to
// catch. So it names the two functions and asserts the edge between them: the
// assigning function is called by the spending one.
func writerStillSpendsTheToken(t *testing.T, file *ast.File) bool {
	t.Helper()
	assignedIn := functionAssigningMailboxProof(file)
	if assignedIn != theMailboxProofWriter {
		t.Errorf("MailboxProof is assigned in %q, but this gate permits %q — either move the "+
			"assignment back to the function that spends the link, or decide the permitted site "+
			"anew and say here why the new one has earned the claim",
			assignedIn, theMailboxProofWriter)
		return false
	}
	return callsBoth(file, theMailboxProofSpender, theMailboxProofWriter)
}

// functionAssigningMailboxProof names the function that sets the field, or ""
// when none does.
func functionAssigningMailboxProof(file *ast.File) string {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if assignsMailboxProof(&ast.File{Decls: []ast.Decl{fn}}) {
			return fn.Name.Name
		}
	}
	return ""
}

// callsBoth reports whether the named caller calls the named callee — the edge
// that makes the proof earned rather than asserted.
func callsBoth(file *ast.File, caller, callee string) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != caller || fn.Body == nil {
			continue
		}
		reaches := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == callee {
				reaches = true
			}
			return !reaches
		})
		return reaches
	}
	return false
}
