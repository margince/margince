// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"go/ast"
	"slices"
	"strings"
	"testing"
)

// A contract leaves this package through ONE reader, and it is the masked one.
//
// `readContract` answers what the row IS: the write paths take their pre-image,
// their anchor and their audit before-image from it, and a masked pre-image
// would authorize a patch against a company the caller was merely not shown.
// `readContractForCaller` is the same read with the reader's own sight applied
// to what the row points at, and it is the one whose answer reaches the wire.
//
// The distinction lives in two function names, and a name is not an obligation.
// A sixth store method added next year reaches for the shorter name — it is the
// one that reads like "read the contract" — and ships an unmasked
// company_id to a caller admitted through the deal. That is #1876's defect
// arriving through the door #1983 closed, so it is held here rather than
// explained in a comment nobody greps.
func TestTheUnmaskedContractReadIsForWritesOnly(t *testing.T) {
	t.Parallel()
	callers := functionsWhere(t, func(_ *ast.File, fn *ast.FuncDecl) bool {
		return callsFunction(fn, unmaskedRead)
	})
	// A census that can fail short has already failed: a rename would leave
	// this walking an empty set and reporting PASS over every read at once.
	if len(callers) == 0 {
		t.Fatalf("nothing in this package calls %s — it was renamed or removed, and this gate "+
			"is now judging nothing", unmaskedRead)
	}
	for _, caller := range callers {
		if slices.Contains(unmaskedReadCallers, caller) {
			continue
		}
		t.Errorf("%s calls %s, which does not withhold the references its reader may not open.\n\n"+
			"A contract is admitted by its deal OR its company, and that disjunction is about "+
			"admission — a reader admitted through the deal may still not open the company. If this "+
			"answer reaches a caller, go through %s. If it is a write's pre-image, its anchor or its "+
			"audit image, add it to unmaskedReadCallers with what it does with the row.",
			caller, unmaskedRead, maskedRead)
	}
}

const (
	unmaskedRead = "readContract"
	maskedRead   = "readContractForCaller"
)

// unmaskedReadCallers are the two functions entitled to the unmasked row, each
// a sentence somebody wrote rather than an omission nobody noticed.
var unmaskedReadCallers = []string{
	// The write path's pre-image: it needs the anchor to authorize against and
	// the before image to audit, neither of which is a disclosure to anybody.
	"writableContract",
	// The masked read itself, which is this rule's one sanctioned wrapper.
	maskedRead,
}

// callsFunction answers whether the function calls the named one directly, by
// its bare name.
//
// Bare because both names are this package's own: a selector spelling
// (`x.readContract`) would be a method on some other type and a different
// function entirely, and matching it would report a stranger.
func callsFunction(fn *ast.FuncDecl, name string) bool {
	if fn.Body == nil || fn.Name.Name == name {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// Every reference the contract projection carries is masked, or the mask is a
// subset of the projection and the next column added to the select list travels
// unbounded.
//
// The projection is the census's own subject rather than a list restated here:
// a fourth reference added to contractColumns fails this until it is either
// masked or named as something other than a reference.
func TestEveryReferenceInTheProjectionIsMasked(t *testing.T) {
	t.Parallel()
	masked := map[string]bool{}
	for _, reference := range maskedReferences {
		masked[reference.field] = true
	}
	if len(masked) == 0 {
		t.Fatal("maskedReferences is empty, so this gate compares the projection against nothing")
	}
	var unmaskedColumns []string
	for _, column := range strings.Split(contractColumns, ",") {
		column = strings.TrimSpace(strings.ReplaceAll(column, "\n", ""))
		column = strings.TrimSpace(strings.Trim(column, "\t"))
		if !strings.HasSuffix(column, "_id") || masked[column] || slices.Contains(notAReference, column) {
			continue
		}
		unmaskedColumns = append(unmaskedColumns, column)
	}
	if len(unmaskedColumns) > 0 {
		t.Errorf("the contract projection carries %s, which name a record and are not withheld.\n\n"+
			"Handing back the id of a row the reader's own read would refuse makes the contract an "+
			"existence oracle over it. Add each to maskedReferences with the table that answers for "+
			"it, or to notAReference with why it points at nothing row-scoped.",
			strings.Join(unmaskedColumns, ", "))
	}
}

// notAReference are the projection's `_id` columns that do not name a
// row-scoped record, so nothing about the reader bounds them.
var notAReference = []string{
	// The row's own identity, which the caller was already admitted to.
	"id",
	// The successor in the renewal chain: another CONTRACT, admitted by the
	// same clause this row was, so a reader holding this one holds that one.
	"superseded_by_id",
}
