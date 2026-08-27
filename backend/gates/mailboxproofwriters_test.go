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
	"os"
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
	theMailboxProofWriterFile = "internal/modules/consent/confirmsubmit.go"
)

func TestOnlyTheConfirmSubmitClaimsAProvenMailbox(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return assignsMailboxProof(file) },
	}
	var elsewhere []string
	for _, f := range scope.Files(t) {
		if f.Path != theMailboxProofWriterFile {
			elsewhere = append(elsewhere, f.Path)
		}
	}
	if len(elsewhere) > 0 {
		t.Errorf("MailboxProof is set outside %s:\n\t%s\n\n"+
			"The proof is what lets a marketing grant complete with no confirmation mail, and it is "+
			"honest only where the caller has just spent the single-use link that earns it. A second "+
			"writer makes it a claim any caller can assert — including one building a RecordInput from "+
			"a request body, which would grant marketing consent for anybody.",
			theMailboxProofWriterFile, strings.Join(elsewhere, "\n\t"))
	}
	if !writerStillSpendsTheToken(t) {
		t.Errorf("%s sets a MailboxProof without calling spendConfirmTokenTx's caller path — the proof "+
			"is only true because the link was redeemed, so an assignment that no longer follows the "+
			"spend is a claim with nothing behind it", theMailboxProofWriter)
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

// writerStillSpendsTheToken holds the other half: the one permitted writer is
// permitted BECAUSE its transaction spent the link first. If the submit stops
// spending, the assignment stops being earned — and this gate would otherwise
// go on passing, since the file name never changed.
func writerStillSpendsTheToken(t *testing.T) bool {
	t.Helper()
	source, err := os.ReadFile(theMailboxProofWriterFile)
	if err != nil {
		t.Fatalf("read the one permitted writer: %v", err)
	}
	return strings.Contains(string(source), "spendConfirmTokenTx")
}
