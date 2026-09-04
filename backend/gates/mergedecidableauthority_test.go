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
)

// decidableForbidden are the ways a second answer gets written. Each is a
// legitimate token elsewhere in the tree — the point is that the authority
// question is not answered by hand HERE.
var decidableForbidden = []string{
	"owner_id",
	"record_grant",
	"RowScope",
	"Unbounded",
	"writeAuthorityPredicate",
}

func TestTheMergeCardAsksTheSameAuthorityTheWriteAsks(t *testing.T) {
	t.Parallel()
	body := methodSource(t, decidableFile, decidableMethod)

	if !strings.Contains(body, "auth.WritableSubset") {
		t.Errorf("%s does not call auth.WritableSubset.\n\n"+
			"The card's offer and the disposition endpoint's refusal have to be two readings of\n"+
			"one authority. Reaching the answer any other way lets the button and the write drift,\n"+
			"and the reader is the one who finds out.", decidableMethod)
	}
	for _, forbidden := range decidableForbidden {
		if strings.Contains(body, forbidden) {
			t.Errorf("%s mentions %q, which is the authority rule being re-derived rather than asked.\n\n"+
				"auth.WritableSubset already answers visibility and write authority together. A second\n"+
				"spelling here is a second answer to one question, and the two will disagree.",
				decidableMethod, forbidden)
		}
	}
}

// methodSource returns the source text of one method, so a gate can assert what
// it does and does not reach for.
func methodSource(t *testing.T, file, method string) string {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != method || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Body.Pos()).Offset
		end := fset.Position(fn.Body.End()).Offset
		return readFile(t, file)[start:end]
	}
	t.Fatalf("%s has no method %s — a gate whose subject moved reports PASS on a smaller tree", file, method)
	return ""
}
