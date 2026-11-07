// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

//go:build !integration

package gates

// Capture locks component identities before rows; later writers use the same
// coordinator without waiting while they own rows.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	captureEntry    = "internal/modules/capture/sink.go"
	captureJoin     = "internal/modules/capture/threadjoin.go"
	captureLockFile = "internal/modules/capture/capturebeforethewrite.go"
	lockTakerName   = "takeMergeLockFirst"
	captureEntryFn  = "Upsert"
	// Upsert takes the merge lock before landing. The landing step resolves
	// mailbox aliases before capture, which can lock existing activity rows.
	mailReadFile    = "internal/modules/capture/threadboundary.go"
	mailReadFn      = "readMailForTheSeatTx"
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

	entry := functionNamed(t, gatekit.ModuleRoot(t), captureEntry, captureEntryFn)
	lockAt := offsetOfCall(entry, lockTakerName)
	if lockAt < 0 {
		t.Fatalf("%s does not call %s — a capture that locks an activity row before the merge "+
			"lock deadlocks against a concurrent merge of the same thread", captureEntryFn, lockTakerName)
	}
	// The sighting is still the first thing that can reach an activity, through
	// two wrappers. Asserting the outer one alone would let the sighting move out
	// from under it unnoticed. So each link of the chain fails on its own.
	mailRead := functionNamed(t, gatekit.ModuleRoot(t), mailReadFile, mailReadFn)
	if offsetOfCall(mailRead, seatAddressFn) < 0 {
		t.Errorf("%s no longer calls %s, so the ordering below pins the mail read and not the "+
			"alias path it stands for", mailReadFn, seatAddressFn)
	}
	wrapper := functionNamed(t, gatekit.ModuleRoot(t), seatAddressFile, seatAddressFn)
	if offsetOfCall(wrapper, aliasSightingFn) < 0 {
		t.Errorf("%s no longer calls %s, so the ordering below pins the wrapper and not the "+
			"alias path it stands for", seatAddressFn, aliasSightingFn)
	}
	landingAt := offsetOfCall(entry, "landActivityTx")
	if landingAt < 0 || lockAt > landingAt {
		t.Fatal("Upsert must take the merge lock before landActivityTx can touch an activity")
	}
	landing := functionNamed(t, gatekit.ModuleRoot(t), "internal/modules/capture/sinkactivitylanding.go", "landActivityTx")
	readAt := offsetOfCall(landing, mailReadFn)
	captureAt := offsetOfCall(landing, "captureActivity")
	if readAt < 0 || captureAt < 0 || readAt > captureAt {
		t.Fatal("landActivityTx must resolve the seat's mailbox before capturing an activity")
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

func TestTheMergeLockHasOneSpelling(t *testing.T) {
	t.Parallel()
	tree := gatekit.ModuleRoot(t)
	for _, site := range []struct{ file, function, coordinator string }{
		{captureLockFile, lockTakerName, "WaitThreadKeysTx"},
		{captureJoin, "mergeLinkedThreads", "LockThreadKeysTx"},
	} {
		fn := functionNamed(t, tree, site.file, site.function)
		if offsetOfCall(fn, site.coordinator) < 0 {
			t.Errorf("%s must call the shared %s coordinator", site.function, site.coordinator)
		}
		for _, literal := range sqlStringsIn(t, tree+"/"+site.file, mustRead(t, tree+"/"+site.file)) {
			if strings.Contains(literal, "pg_advisory") {
				t.Errorf("%s spells a second lock protocol", site.file)
			}
		}
	}
	coordinator := "internal/platform/database/storekit/threadlocks.go"
	for _, function := range []string{"WaitThreadKeysTx", "LockThreadKeysTx"} {
		fn := functionNamed(t, tree, coordinator, function)
		if !callsName(fn, "threadStateTx") {
			t.Errorf("%s bypasses the shared component key ledger", function)
		}
		for _, name := range []string{"threadBarrier"} {
			if !bodyUses(fn, name) {
				t.Errorf("%s does not share %s", function, name)
			}
		}
	}
	if !bodyUses(functionNamed(t, tree, coordinator, "threadStateTx"), "threadComponent") {
		t.Error("thread coordinator lost its shared namespace")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := gatekit.ReadFile(path)
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
