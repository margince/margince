// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

//go:build !integration

package gates

// A capture that may merge threads takes the merge lock before it touches an activity.
//
// Not every capture: a non-email record, and a message nothing stored links,
// take no lock at all, which is what keeps a workspace-wide lock off the common
// path. The ordering binds the ones that can reach a merge.
//
// mergeLinkedThreads takes one workspace-wide advisory lock and then updates
// every row of the threads it merges. A concurrent capture that has already
// written — and so locked — one of those rows, and then waits for the same
// advisory lock, closes a cycle; Postgres breaks it by aborting one transaction.
// Nothing is lost, because the aborted capture is retried, so the cost is paid in
// repeated retries on exactly the busy shared conversations two mailboxes both
// carry.
//
// Taking the coarse lock before any fine one removes the cycle, and that ordering
// is the kind of property an ordinary edit undoes without looking wrong: move the
// probe below the first write, or give the lock a second spelling, and the cycle
// is back with every test still green. Hence a gate rather than a comment.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

const (
	captureEntry    = "internal/modules/capture/sink.go"
	captureJoin     = "internal/modules/capture/threadjoin.go"
	captureLockFile = "internal/modules/capture/capturebeforethewrite.go"
	lockTakerName   = "takeMergeLockFirst"
	lockStatement   = "mergeLockStatement"
	captureEntryFn  = "Upsert"
	// The alias sighting sits one call down, behind the wrapper that settles what
	// the seat's own addresses decide. The gate reads both halves: the wrapper's
	// position inside Upsert, and the sighting's presence inside the wrapper.
	seatAddressFile = "internal/modules/capture/sentfromownaddress.go"
	seatAddressFn   = "readAgainstTheSeatsAddressesTx"
	aliasSightingFn = "noteAliasSightingTx"
)

// TestTheMergeLockIsTakenBeforeAnythingTouchesAnActivity holds the ordering where
// the transaction begins, not where the activity is written.
//
// captureActivity is not early enough, and the reason is worth keeping: the alias
// path runs before it in the same transaction, and adoption recomputes the audience
// of every message it adopts — which locks those activity rows. A transaction
// holding one of them before it asks for the merge lock is the cycle again.
//
// So the claim is about ORDER WITHIN Upsert: the lock is taken before
// noteAliasSightingTx, which is the first thing there that can reach an activity.
func TestTheMergeLockIsTakenBeforeAnythingTouchesAnActivity(t *testing.T) {
	t.Parallel()

	entry := functionNamed(t, moduleRoot(t), captureEntry, captureEntryFn)
	lockAt := offsetOfCall(entry, lockTakerName)
	if lockAt < 0 {
		t.Fatalf("%s does not call %s — a capture that locks an activity row before the merge "+
			"lock deadlocks against a concurrent merge of the same thread", captureEntryFn, lockTakerName)
	}
	// The sighting is still the first thing that can reach an activity; it is just
	// reached through the wrapper now. Asserting the wrapper alone would let the
	// sighting move out from under it without a word, so the chain is read as two
	// links and each one fails on its own.
	wrapper := functionNamed(t, moduleRoot(t), seatAddressFile, seatAddressFn)
	if offsetOfCall(wrapper, aliasSightingFn) < 0 {
		t.Errorf("%s no longer calls %s, so the ordering below pins the wrapper and not the "+
			"alias path it stands for", seatAddressFn, aliasSightingFn)
	}
	for _, reaches := range []string{seatAddressFn, "captureActivity"} {
		at := offsetOfCall(entry, reaches)
		if at < 0 {
			t.Errorf("%s no longer calls %s — this gate reads the order of the two, so a rename "+
				"leaves it holding nothing", captureEntryFn, reaches)
			continue
		}
		if lockAt > at {
			t.Errorf("%s takes the merge lock after %s, which can lock an activity row — the "+
				"coarse lock has to come first or the cycle is back", captureEntryFn, reaches)
		}
	}
}

// offsetOfCall returns the source position of the first call to name inside fn, or
// -1. A position rather than a statement index, because the calls this compares sit
// at different nesting depths — one inside a type switch — and an index among
// top-level statements cannot order those.
func offsetOfCall(fn *ast.FuncDecl, name string) int {
	invoked := invokedLiterals(fn)
	found := -1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if lit, isLit := n.(*ast.FuncLit); isLit && !invoked[lit] {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || found >= 0 {
			return found < 0
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == name {
			found = int(call.Pos())
		}
		return found < 0
	})
	return found
}

// TestTheMergeLockHasOneSpelling is the other half: two spellings are two locks,
// and the ordering would hold over a lock nobody else takes.
//
// It reads the two FUNCTIONS rather than counting the name in a file, because a
// text count is inflated by any comment that mentions it — this gate passed on
// exactly that coincidence once, while the file holding the early lock had moved
// out of what it read.
func TestTheMergeLockHasOneSpelling(t *testing.T) {
	t.Parallel()

	tree := moduleRoot(t)
	for _, site := range []struct{ file, function string }{
		{captureLockFile, lockTakerName},
		{captureJoin, "mergeLinkedThreads"},
	} {
		fn := functionNamed(t, tree, site.file, site.function)
		if !bodyUses(fn, lockStatement) {
			t.Errorf("%s does not take %s — a path that locks nothing, or locks a second "+
				"spelling of the same name, and the ordering means nothing", site.function, lockStatement)
		}
	}

	// And the SQL is spelled in exactly ONE string literal across these files.
	//
	// Read from the parsed literals, not the bytes: counting the text found a doc
	// comment that mentions the lock and called it a second spelling, and missed a
	// literal split across lines entirely. This gate passed once on that
	// coincidence while the file holding the early lock had moved out of what it
	// read, which is the shape worth not repeating.
	spellings := 0
	for _, file := range []string{captureEntry, captureLockFile, captureJoin} {
		for _, literal := range sqlStringsIn(t, tree+"/"+file, mustRead(t, tree+"/"+file)) {
			if strings.Contains(literal, "pg_advisory_xact_lock") {
				spellings++
			}
		}
	}
	if spellings != 1 {
		t.Errorf("pg_advisory_xact_lock is spelled in %d string literals across the capture path, "+
			"want 1 — two spellings of one lock name are two locks, and the ordering holds over "+
			"neither", spellings)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return body
}

// bodyUses reports whether a function body uses the named identifier.
func bodyUses(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
			found = true
		}
		return !found
	})
	return found
}

func probeFunction(file *ast.File) *ast.FuncDecl {
	for _, declared := range file.Decls {
		if fn, ok := declared.(*ast.FuncDecl); ok && fn.Body != nil {
			return fn
		}
	}
	return nil
}

// TestTheOrderingGateReadsCallOrder plants the shapes offsetOfCall has to tell
// apart, because that is the comparator the gate's assertion rests on now. The
// previous version of this test exercised a statement-index helper the gate had
// stopped using, so nothing held the thing that decides whether the gate fires.
//
// The nesting matters: the real calls sit at different depths — one inside a type
// switch — which is why the comparator reads source positions rather than indices
// among top-level statements, and why one case here is nested.
func TestTheOrderingGateReadsCallOrder(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name          string
		body          string
		lockIsEarlier bool
	}{
		{
			"the lock first",
			"func f() { s.takeMergeLockFirst(a); s.noteAliasSightingTx(b) }",
			true,
		},
		{
			"the lock after the alias path",
			"func f() { s.noteAliasSightingTx(b); s.takeMergeLockFirst(a) }",
			false,
		},
		{
			"both inside a switch, the lock first",
			"func f() { switch x { case 1: s.takeMergeLockFirst(a); s.noteAliasSightingTx(b) } }",
			true,
		},
		{
			"both inside a switch, the lock second",
			"func f() { switch x { case 1: s.noteAliasSightingTx(b); s.takeMergeLockFirst(a) } }",
			false,
		},
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go",
			"package probe\n"+c.body, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("%s: parsing the probe: %v", c.name, err)
		}
		fn := probeFunction(parsed)
		lock, alias := offsetOfCall(fn, lockTakerName), offsetOfCall(fn, "noteAliasSightingTx")
		if lock < 0 || alias < 0 {
			t.Fatalf("%s: the probe reads lock=%d alias=%d; both calls are present in it",
				c.name, lock, alias)
		}
		if earlier := lock < alias; earlier != c.lockIsEarlier {
			t.Errorf("%s: read the lock as earlier=%t, want %t", c.name, earlier, c.lockIsEarlier)
		}
	}
}

// And a call that is absent reads as absent rather than as position zero, which a
// comparison against another offset would otherwise treat as earliest.
func TestAnAbsentCallIsNotTheEarliestOne(t *testing.T) {
	t.Parallel()

	parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go",
		"package probe\nfunc f() { s.noteAliasSightingTx(b) }", parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the probe: %v", err)
	}
	if got := offsetOfCall(probeFunction(parsed), lockTakerName); got >= 0 {
		t.Errorf("a function that never takes the lock read as offset %d, want negative — a zero "+
			"would compare as earlier than every real call and the gate would pass", got)
	}
}

// invokedLiterals collects the function literals this body actually runs: the ones
// handed straight to a call, which is how the whole capture transaction reaches the
// store — s.db.Tx(ctx, func(tx pgx.Tx) error { ... }) — and the ones called in
// place. A literal nobody passes anywhere does not execute, so a call inside one
// says nothing about what the function does, and reading position cannot tell the
// two apart without this.
func invokedLiterals(fn *ast.FuncDecl) map[*ast.FuncLit]bool {
	invoked := map[*ast.FuncLit]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if lit, isLit := call.Fun.(*ast.FuncLit); isLit {
			invoked[lit] = true
		}
		for _, arg := range call.Args {
			if lit, isLit := arg.(*ast.FuncLit); isLit {
				invoked[lit] = true
			}
		}
		return true
	})
	return invoked
}
