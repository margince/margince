// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every send says WHY it is being sent.
//
// The engine resolves what a message is from the record, and a caller's claim
// is one of the four things it reads (authorizeresolve.go, arm 3). A send input
// built without one is not refused and does not fail: it reaches the engine
// with nothing said, the claim arm finds nothing to check, and the decision row
// records a message whose sender never stated an intent. That is invisible in
// review — the struct literal beside it looks the same, minus four fields
// nobody misses.
//
// So every construction of a send input either carries the claim or is ratified
// here with what its absence costs.
//
// A construction "carries the claim" when it either sets Context in the literal
// or is wrapped by the decoder that sets it — ApplyContext, ApplyChannelContext,
// or the sendContextFrom/applyTo pair the HTTP doors use. Matching only the
// literal would fail the doors that decode first and apply after, which is the
// correct shape and the majority of them.
//
// NOT asserted: that a claim is TRUE. A claim is a claim — the engine checks it
// against the record and records the disagreement. This gate is about a sender
// having said something at all.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sendInputTypes are the two send-input structs. Two because mail and channel
// are two implementations of one act, and the recurring defect in this area is
// a fix landing on one of them.
var sendInputTypes = map[string]bool{
	"SendEmailInput":   true,
	"SendMessageInput": true,
}

// claimWrappers are the decoders that place a claim on a send input built by
// somebody else. A construction wrapped in one of these states its claim even
// though the literal does not name the field.
var claimWrappers = map[string]bool{
	"ApplyContext":        true,
	"ApplyChannelContext": true,
	"applyTo":             true,
}

// claimedByTheirCallers are helpers whose every caller wraps the result in a
// claim decoder, so the bare literal inside them is claimed one call up.
//
// Named rather than inferred because the wrapping happens in ANOTHER FILE —
// sendInputFrom builds the literal in handlers_email.go and the applyTo that
// claims it lives in sendcontext.go — and a per-file walk cannot see across
// that. TestEveryHelperClaimedByItsCallersReallyIs holds the claim these entries
// make, so this list cannot quietly become an exemption.
var claimedByTheirCallers = map[string]bool{"sendInputFrom": true}

// contextlessSends ratifies each construction that states no claim, with what
// the omission costs.
var contextlessSends = gatekit.Waive(map[string]string{
	"internal/compose/heldrelease.go": "KNOWN GAP, not a settled exemption. An automation-drafted " +
		"message released by an approver reaches the engine with no claimed category, no evidence " +
		"and no operator reason, because automation.HeldDraftProposal carries none to pass — the " +
		"consent plan's PR 5 was to add them and never landed. The send is still authorized: the " +
		"engine resolves from the anchor thread, which is the strongest ground a reply can have, " +
		"so this is a missing STATEMENT rather than a missing check. Waived because adding the " +
		"fields reaches the approval payload and the edit-scope pin, which is its own change",
})

func TestEverySendStatesWhyItIsBeingSent(t *testing.T) {
	t.Parallel()

	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: buildsASendInput,
	}
	files := scope.Files(t)

	// Under-recognition is the one way this must not fail: a walk that stopped
	// finding the constructions — a renamed struct, a moved package — would
	// report PASS over a tree full of unexplained sends.
	if len(files) < 2 {
		t.Fatalf("found %d files building a send input, want at least the mail and channel doors: "+
			"the gate has stopped seeing its subject", len(files))
	}

	for _, f := range files {
		if statesAClaim(f.File) {
			continue
		}
		if contextlessSends.Waived(t, f.Path) {
			continue
		}
		t.Errorf("%s builds a send input that states no communication context: the message reaches "+
			"the engine with nothing said about why it is being sent, the claim arm finds nothing "+
			"to check, and the decision row records a sender who stated no intent. Set Context, or "+
			"ratify it in contextlessSends with what the omission costs", f.Path)
	}
	contextlessSends.AssertAllMatched(t)
}

// TestTheClaimDecoderStillSetsWhatThisGateLooksFor holds the assumption the
// census rests on.
//
// A file satisfies the gate by naming one of the claim setters. If the decoder
// stopped setting Context — renamed the field, moved the assignment — every
// wrapped door would keep naming ApplyContext and keep passing, while no claim
// reached the engine at all. This reads the decoder itself.
func TestTheClaimDecoderStillSetsWhatThisGateLooksFor(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	path := filepath.Join("internal", "modules", "activities", "sendcontext.go")
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the claim decoder: %v", err)
	}
	if !assignsTheContextField(file) {
		t.Error("sendcontext.go no longer assigns Context on a send input: every door that " +
			"satisfies this gate by naming its decoder now passes while claiming nothing")
	}
}

// TestEveryHelperClaimedByItsCallersReallyIs holds the claim that list makes.
//
// claimedByTheirCallers exempts a helper's bare literal on the promise that its
// callers wrap it. A promise nothing checks is an exemption: a new caller that
// forgot the wrapper would leave the helper exempt and the send unclaimed, and
// the census above would keep passing. So every call is found and every one
// must sit inside a claim decoder.
func TestEveryHelperClaimedByItsCallersReallyIs(t *testing.T) {
	t.Parallel()

	// The subject is a file that CALLS a vouched helper, not every file: the
	// scope sweeps its own negative space, and "every non-test file" would
	// report the whole tree as unseen rather than the callers this vouches for.
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: callsAVouchedHelper,
	}
	calls := 0
	for _, f := range scope.Files(t) {
		for helper := range claimedByTheirCallers {
			for _, unwrapped := range unwrappedCallsTo(f.File, helper) {
				calls++
				t.Errorf("%s calls %s outside a claim decoder: the census exempts that helper's "+
					"literal because its callers wrap it, so this send states no claim and "+
					"nothing reports it", f.Path, unwrapped)
			}
		}
		for helper := range claimedByTheirCallers {
			calls += wrappedCallCount(f.File, helper)
		}
	}
	// Under-recognition: an exemption for a helper nobody calls is a stale
	// entry, and one whose calls this walk cannot see is worse.
	if calls == 0 {
		t.Fatal("no calls to any helper in claimedByTheirCallers were found: the exemption is " +
			"stale, or this walk can no longer see the callers it vouches for")
	}
}

// callsAVouchedHelper reports whether a file calls one of the helpers whose
// literal the census exempts.
func callsAVouchedHelper(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if claimedByTheirCallers[calleeName(call)] {
			found = true
			return false
		}
		return true
	})
	return found
}

// unwrappedCallsTo names every call to helper that is NOT inside a claim
// decoder.
func unwrappedCallsTo(file *ast.File, helper string) []string {
	var bad []string
	var walk func(n ast.Node, wrapped bool)
	walk = func(n ast.Node, wrapped bool) {
		ast.Inspect(n, func(inner ast.Node) bool {
			if inner == nil || inner == n {
				return true
			}
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calleeName(call)
			if name == helper && !wrapped {
				bad = append(bad, helper)
			}
			walk(call, wrapped || claimWrappers[name])
			return false
		})
	}
	walk(file, false)
	return bad
}

// wrappedCallCount counts the calls that ARE wrapped, so the test can tell an
// empty tree from a clean one.
func wrappedCallCount(file *ast.File, helper string) int {
	n := 0
	var walk func(node ast.Node, wrapped bool)
	walk = func(node ast.Node, wrapped bool) {
		ast.Inspect(node, func(inner ast.Node) bool {
			if inner == nil || inner == node {
				return true
			}
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calleeName(call)
			if name == helper && wrapped {
				n++
			}
			walk(call, wrapped || claimWrappers[name])
			return false
		})
	}
	walk(file, false)
	return n
}

// buildsASendInput reports whether a file constructs one of the send inputs.
//
// A zero literal is not a construction. `return SendEmailInput{}, err` is an
// error path, and putting those in the corpus would demand a claim on the value
// nobody sends.
func buildsASendInput(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || len(lit.Elts) == 0 {
			return true
		}
		// The shared literalTypeName keeps a package qualifier, and the same
		// struct is `SendEmailInput{}` inside its package and
		// `activities.SendEmailInput{}` outside it. Both are constructions, so
		// the final segment is what is matched.
		name := literalTypeName(lit)
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if sendInputTypes[name] {
			found = true
			return false
		}
		return true
	})
	return found
}

// statesAClaim reports whether EVERY send-input construction in a file places a
// claim — either by naming Context in the literal, or by being wrapped in a
// decoder that sets it.
//
// PER CONSTRUCTION, not per file. An earlier draft asked only whether the file
// mentioned a decoder anywhere, and the mutation that matters survived it:
// deleting `Context:` from the channel door left the file still naming
// sendContextFrom a few lines above, so the gate passed while no claim reached
// the engine. A file with two constructions, one of them contextless, is the
// shape this has to catch.
func statesAClaim(file *ast.File) bool {
	every := true
	var walk func(n ast.Node, wrapped bool)
	walk = func(n ast.Node, wrapped bool) {
		ast.Inspect(n, func(inner ast.Node) bool {
			if inner == nil || inner == n {
				return true
			}
			switch v := inner.(type) {
			case *ast.CallExpr:
				// A construction inside a claim wrapper is claimed by it.
				walk(v, wrapped || claimWrappers[calleeName(v)])
				return false
			case *ast.CompositeLit:
				name := literalTypeName(v)
				if i := strings.LastIndex(name, "."); i >= 0 {
					name = name[i+1:]
				}
				if len(v.Elts) > 0 && sendInputTypes[name] && !wrapped && !namesTheContextField(v) {
					every = false
				}
				return true
			}
			return true
		})
	}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		if claimedByTheirCallers[fn.Name.Name] {
			continue
		}
		walk(fn.Body, false)
	}
	return every
}

// namesTheContextField reports whether a literal sets Context.
func namesTheContextField(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Context" {
			return true
		}
	}
	return false
}

// assignsTheContextField reports whether the decoder still writes Context onto
// a send input.
func assignsTheContextField(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if ok && sel.Sel.Name == "Context" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
