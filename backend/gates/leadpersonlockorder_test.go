// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// ONE lock order over the lead and the person it was promoted into.
//
// Two writers hold both rows. MergePerson locks the people (LockPair) and then
// repoints lead.promoted_person_id, so it goes person -> lead. DemoteLead used
// to go lead -> person. Opposite orders over the same pair is the whole of a
// deadlock: each transaction ends up holding what the other is waiting for,
// Postgres aborts one with 40P01, and the caller gets a 5xx where the losing
// side of a serialized race should get a clean refusal.
//
// The integration suite pins that ordering for the demote specifically, by
// holding a row open and asking what the parked writer holds. This gate asks
// the question the suite cannot: not "does THIS writer take them in order" but
// "does EVERY writer", including the one added next month whose race test
// nobody thought to write.
//
// What it holds is the ORDER, and only that. Whether a writer takes the
// person's lock AT ALL is held by the demote's race suite, which parks the
// writer on the lead and asks whether the person is already held — a question
// no source read can answer, because a lock not taken looks exactly like a
// lock the reader failed to find.
//
// It is transitive, and that is the point rather than a flourish: the promote
// path takes its lead lock in QualifyLead and its person lock three frames
// down in mergeLeadIntoPerson. A gate reading one function body at a time sees
// a lead lock here and a person lock there, finds no function holding both,
// and reports a clean tree over the exact shape it exists to catch.
//
// H2, not H3: the order is a convention two paths must share, and the harm is
// an aborted transaction rather than corrupted data. What makes it worth a
// gate is that the failure is invisible until two writers actually interleave
// in production, and by then the evidence is a 40P01 in a log.

import (
	"fmt"
	"go/ast"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The tables whose order is under judgement, and the order itself.
const (
	lockOrderPerson = "person"
	lockOrderLead   = "lead"
)

// lockOrderDoors are the storekit calls that take a row lock. Both spell the table
// as their third argument, which is what makes one predicate serve both.
var lockOrderDoors = map[string]bool{"LockRow": true, "LockPair": true}

// lockOrderRoots is where the two writers live. Scoped to the people module
// because lead and person are its rows: Scope proves that claim by sweeping
// everything outside it for the same sites.
var lockOrderRoots = []string{"internal/modules/people"}

// promoteTakesTheOldOrder ratifies the one path still going lead -> person.
//
// Promotion discovers WHICH person it is contending over: it reads the lead's
// emails, matches them, and merges into whatever live person that finds — so
// unlike the demote, which is told the person by lead.promoted_person_id, it
// cannot lock the person before it has read the lead under a lock. Reordering
// it means resolving the match unlocked and refusing the promotion when the
// match moves underneath, which turns a benign race into a user-visible 409.
// That is a product decision about what a racing promote answers, not a
// lock-order edit, so it is tracked as its own change rather than settled here
// as a side effect.
//
// Two entries because both entry points reach the locks: QualifyLead holds the
// body, and PromoteLead is the one-line wrapper callers use. Waiving only the
// body would leave the wrapper failing and read as a second, different defect.
var promoteTakesTheOldOrder = gatekit.Waive(map[string]string{
	"QualifyLead": "promotion resolves its person by matching the lead's emails, so it cannot name the " +
		"row to lock before it has read the lead — reordering it changes what a racing promote answers, " +
		"which is a product decision about what a racing promote returns, not a lock-order edit",
	"PromoteLead": "the one-line wrapper over QualifyLead, waived with it and for the same reason",
})

// TestOneLockOrderOverTheLeadAndItsPerson fails on a writer that reaches the
// lead's lock before the promoted person's.
func TestOneLockOrderOverTheLeadAndItsPerson(t *testing.T) {
	t.Parallel()
	files := gatekit.Scope{
		Roots:   lockOrderRoots,
		Subject: locksALeadOrItsPerson,
	}.Files(t)

	// Bodies are keyed by name alone, and a name with several bodies keeps all
	// of them. Resolving s.helper() to a receiver would need type information
	// this gate does not carry, so a call to an overloaded name is followed
	// into EVERY body that answers to it. That over-reports — a method taking
	// no lock inherits the locks of its namesake — and over-reporting is the
	// safe direction: it can turn a clean function into a finding somebody has
	// to look at, never an offender into a clean one.
	bodies := map[string][]*ast.FuncDecl{}
	var order []string
	for _, file := range files {
		for _, decl := range file.File.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			if _, seen := bodies[fn.Name.Name]; !seen {
				order = append(order, fn.Name.Name)
			}
			bodies[fn.Name.Name] = append(bodies[fn.Name.Name], fn)
		}
	}

	offenders := map[string]string{}
	locking := 0
	for _, name := range order {
		sequence := locksTakenBy(t, name, bodies, map[string]bool{})
		if len(sequence) == 0 {
			continue
		}
		locking++
		if lead, ok := leadBeforePerson(sequence); ok {
			offenders[name] = lead
		}
	}

	// A census that can fail short has already failed: a rename of the lock
	// door, or a scope that stopped reaching the module, empties the sequences
	// and every function reads clean.
	if locking < 6 {
		t.Fatalf("only %d function(s) in %s were found taking a row lock, and the module has more than "+
			"that: this gate is reading a tree it no longer understands rather than a tree that is clean",
			locking, lockOrderRoots[0])
	}

	names := make([]string, 0, len(offenders))
	for name := range offenders {
		if promoteTakesTheOldOrder.Waived(t, name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 0 {
		lines := make([]string, 0, len(names))
		for _, name := range names {
			lines = append(lines, fmt.Sprintf("  %s locks %s", name, offenders[name]))
		}
		t.Fatalf(`%d writer(s) take the lead's lock before the promoted person's:

%s

MergePerson locks the person first and then writes the lead. A writer taking
them the other way round closes a cycle with it: each holds what the other
waits for, and Postgres breaks the tie by aborting one with a deadlock, which
reaches the caller as a 5xx. Lock the person first — read whatever you need to
name it WITHOUT a lock, then re-read under both locks and refuse if it moved.`,
			len(names), strings.Join(lines, "\n"))
	}
	promoteTakesTheOldOrder.AssertAllMatched(t)
}

// locksALeadOrItsPerson is the sweep's subject: a file that locks EITHER of
// the two rows, not both.
//
// Either, because a file locking one of them today is exactly where the second
// lock is added tomorrow — and the sweep's job is to prove the roots cover the
// code that could acquire this obligation, not merely the code that already
// has. A module outside internal/modules/people that starts locking a person
// fails this sweep and has to say why it is not part of the order.
func locksALeadOrItsPerson(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		table, isLock := lockOrderTable(node)
		if isLock && (table == lockOrderPerson || table == lockOrderLead) {
			found = true
		}
		return !found
	})
	return found
}

// locksTakenBy returns the tables one function locks, in source order,
// following calls to functions declared in the same module.
//
// visited stops recursion on a cycle and, incidentally, on a second visit to
// the same callee within one walk. That under-reports a helper called twice,
// which cannot turn an offender into a clean function: the FIRST lock either
// path takes is the one the order is judged on, and it is still recorded.
func locksTakenBy(t *testing.T, name string, bodies map[string][]*ast.FuncDecl, visited map[string]bool) []string {
	t.Helper()
	if visited[name] {
		return nil
	}
	visited[name] = true
	var sequence []string
	for _, fn := range bodies[name] {
		sequence = append(sequence, locksTakenIn(t, name, fn, bodies, visited)...)
	}
	return sequence
}

// locksTakenIn is locksTakenBy over one body.
func locksTakenIn(t *testing.T, name string, fn *ast.FuncDecl, bodies map[string][]*ast.FuncDecl, visited map[string]bool) []string {
	t.Helper()
	var sequence []string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if table, isLock := lockOrderTable(node); isLock {
			if table == "" {
				t.Errorf("%s takes a row lock on a table this gate cannot read: the table is spelled as an "+
					"expression rather than a literal, so the order it takes cannot be judged", name)
				return true
			}
			sequence = append(sequence, table)
			return true
		}
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if callee := lockOrderCallee(call); callee != "" {
			sequence = append(sequence, locksTakenBy(t, callee, bodies, visited)...)
		}
		return true
	})
	return sequence
}

// lockOrderTable reports the table a storekit lock call names, and whether the
// node is such a call at all. An empty table with isLock true is a lock whose
// table is not a literal — reported by the caller rather than skipped.
func lockOrderTable(node ast.Node) (table string, isLock bool) {
	call, isCall := node.(*ast.CallExpr)
	if !isCall {
		return "", false
	}
	door, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || !lockOrderDoors[door.Sel.Name] {
		return "", false
	}
	// Every door in the map takes (ctx, tx, table, ...).
	if len(call.Args) < 3 {
		return "", false
	}
	literal, isLiteral := call.Args[2].(*ast.BasicLit)
	if !isLiteral {
		return "", true
	}
	unquoted, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", true
	}
	return unquoted, true
}

// lockOrderCallee is the name of a call to a function or method that may be
// declared in this module. A selector's own name is used because a method call
// on the store reads as s.helper — and a selector that resolves to some other
// package simply has no body here, which locksTakenBy answers with nothing.
func lockOrderCallee(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// leadBeforePerson reports whether a sequence reaches the lead's lock without
// the person's, and the table it found there. A sequence holding only one of
// the two is not an ordering and is not judged.
func leadBeforePerson(sequence []string) (string, bool) {
	for _, table := range sequence {
		switch table {
		case lockOrderPerson:
			return "", false
		case lockOrderLead:
			return lockOrderLead, lockOrderHasTable(sequence, lockOrderPerson)
		}
	}
	return "", false
}

func lockOrderHasTable(sequence []string, want string) bool {
	for _, table := range sequence {
		if table == want {
			return true
		}
	}
	return false
}
