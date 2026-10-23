// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// The subject lock is the first row a transaction takes.
//
// Art. 17 erasure is subject-first and always has been: anonymizeSubjectRows
// runs `UPDATE contact … archived_at` and then, in the same transaction, deletes
// the rows hanging off that subject — the consent tokens, the LinkedIn ghosts,
// the lead scores. A writer that touches one of those child rows and only then
// reaches for the subject has taken the two in the opposite order, and the two
// transactions deadlock on each other.
//
// It is not a hypothetical: the change that introduced auth.LockSubjectLive
// wrote exactly that inversion twice, in the DOI issuer and the LinkedIn
// confirm, and both were caught by human review rather than by anything here.
// Postgres kills one of the pair after deadlock_timeout, nothing in this tree
// retries a 40P01, and when the loser is the eraser an Art. 17 fulfilment fails
// on a path an ordinary seat can drive.
//
// So the ordering rule stops being a sentence in a doc comment. What the rule
// forbids is any row lock BEFORE the subject hold, and a write is a row lock:
// an INSERT, UPDATE or DELETE holds the rows it touches until commit, which is
// what the DOI supersession did.
//
// Position-based, and that is why this is its own gate rather than an arm of
// the reachability census next door. packageCallGraph records which identifiers
// a function names and which functions it calls; it carries no statement order,
// so it cannot tell "locks then writes" from "writes then locks". Within ONE
// function body the AST does carry order, and both real inversions were visible
// there — the DOI's supersession and the LinkedIn UPDATE each sat above the
// hold in the same function.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// subjectHolds are the spellings that take the subject row. A call to either,
// or to a same-package function that reaches one, discharges the obligation.
var subjectHolds = map[string]bool{
	"HoldWritableLive": true,
	"LockSubjectLive":  true,
}

// mutatingStatement matches SQL that takes a row lock: the three write verbs,
// and the explicit lock clauses.
//
// A plain SELECT is absent on purpose — it takes no row lock and orders nothing.
// The write verbs are present because they do: the inversion this gate exists
// for was an UPDATE of a child table, not an explicit FOR UPDATE.
// No trailing \b, and that is the whole of what makes it work. The first
// version ended the alternation with one, which put the boundary INSIDE the
// table name — `update\s+[a-z_]\b` matches a one-letter table and nothing else,
// so `UPDATE consent_doi_token` went unseen and this gate passed over the very
// deadlock it was written for. It was caught by re-introducing that deadlock and
// watching the gate stay green, which is the only way a short census reports
// itself.
var mutatingStatement = regexp.MustCompile(
	`(?is)\b(?:insert\s+into\s+[a-z_]|update\s+[a-z_]|delete\s+from\s+[a-z_]|for\s+update|for\s+no\s+key\s+update|for\s+share)`)

// locksBeforeTheSubject ratifies a function that takes a row before its subject.
//
// Empty. An entry here is a claim that the eraser cannot be on the other side of
// the rows this function takes first — which is a claim about privacy/erasure.go,
// so write down which rows and why the eraser never holds them.
var locksBeforeTheSubject = gatekit.Waive(map[string]string{
	"internal/modules/contacts:runLinkedInMatch": "the two writes this walk reads as one body are in SEPARATE transactions. runLinkedInMatch resolves ghost employers (matchGhostCompanies, which writes linkedin_connection.matched_company_id) in its own s.db.Tx that COMMITS, and only then opens the match transaction that holds contacts. The eraser does hold linkedin_connection — it deletes the subject's ghosts after locking the subject — but the company-resolution locks are released at the first commit, before any contact is held, so the two never sit on opposite sides of a cycle. Splitting is deliberate: matched_company_id must be resolved before the name tier reads it, and holding those connection locks into the contact-holding transaction is the one ordering the eraser deadlocks against.",
})

func TestTheSubjectLockIsTheFirstRowATransactionTakes(t *testing.T) {
	t.Parallel()
	defer locksBeforeTheSubject.AssertAllMatched(t)

	var findings []string
	judged := 0
	for _, dir := range moduleDirsWith(t, "WritableLive") {
		graph := packageCallGraph(t, dir)
		reachesHold := func(name string) bool {
			for spelling := range subjectHolds {
				if reaches(graph, name, spelling) {
					return true
				}
			}
			return false
		}
		fset := gatekit.SourceFileSet()
		files := parsePackageDir(t, fset, dir)
		// Which functions take a row lock at all — spelled themselves, or
		// through anything they call. A helper holding the UPDATE is what makes
		// its caller's ordering decisive, so the caller has to see it.
		locking := map[string]bool{}
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || fn.Body == nil {
					continue
				}
				if spellsAMutatingStatement(fn) {
					locking[scrubKey(receiverTypeName(fn), fn.Name.Name)] = true
				}
			}
		}
		reachesLock := func(name string) bool {
			if locking[name] {
				return true
			}
			for spelling := range locking {
				if reaches(graph, name, spelling) {
					return true
				}
			}
			return false
		}
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				// Keyed the way packageCallGraph keys it: a method is
				// "Store.IssueDoubleOptIn" there, and looking it up by the bare
				// name found nothing — which read as a tree with four
				// subject-holding functions in it rather than as a broken walk.
				// The floor below is what caught that.
				if !isFunc || fn.Body == nil {
					continue
				}
				recv := receiverTypeName(fn)
				if !reachesHold(scrubKey(recv, fn.Name.Name)) {
					continue
				}
				judged++
				hold, lock := holdAndFirstLock(fn, recv, receiverVarName(fn), reachesHold, reachesLock)
				if hold == token.NoPos || lock == token.NoPos || lock >= hold {
					continue
				}
				key := dir + ":" + scrubKey(recv, fn.Name.Name)
				if locksBeforeTheSubject.Waived(t, key) {
					continue
				}
				findings = append(findings, key+" takes a row at "+
					filepath.Base(fset.Position(lock).String())+
					" before holding its subject at "+
					filepath.Base(fset.Position(hold).String()))
			}
		}
	}

	// A census that judged nothing certifies nothing. The floor sits below the
	// real count so it catches a walk that stopped finding subject-holding
	// functions rather than a tree that changed.
	if judged < 10 {
		t.Fatalf("this gate judged %d function(s) that reach a subject hold and expects at least 10 — "+
			"the walk has stopped finding them rather than the tree having lost them", judged)
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("these functions take a row lock BEFORE holding the subject:\n    %s\n\n"+
			"Art. 17 erasure locks the subject first and then deletes the rows hanging off it "+
			"(privacy/erasure.go: anonymizeSubjectRows, then deleteConsentCapabilities and "+
			"scrubSubjectFromGraph). Taking them the other way round deadlocks against it, and when the "+
			"eraser loses the race an erasure fails. Move the hold above the first write — "+
			"auth.HoldWritableLive at the top of the transaction is the shape — or ratify the writer in "+
			"locksBeforeTheSubject with the rows it takes first and why the eraser never holds them.",
			strings.Join(findings, "\n    "))
	}
}

// holdAndFirstLock returns where this function holds its subject and where it
// first locks a row, either possibly NoPos. A call reaching the hold counts at
// the call. A call that merely writes does not count as a lock, which
// under-reports. A bound closure counts where it is called, and a deferred
// call at the end of the body, because that is when each runs.
func holdAndFirstLock(
	fn *ast.FuncDecl, recvType, recvVar string,
	reachesHold func(string) bool, reachesLock func(string) bool,
) (hold, lock token.Pos) {
	w := lockWalk{
		recvType: recvType, recvVar: recvVar, reachesHold: reachesHold, reachesLock: reachesLock,
		bound: boundClosures(fn.Body), end: fn.Body.End(),
	}
	return w.scan(fn.Body)
}

// lockWalk finds the hold and the first lock in one body.
type lockWalk struct {
	recvType, recvVar string
	reachesHold       func(string) bool
	reachesLock       func(string) bool
	bound             map[string]*ast.FuncLit
	end               token.Pos
}

func earliest(at *token.Pos, pos token.Pos) {
	if pos != token.NoPos && (*at == token.NoPos || pos < *at) {
		*at = pos
	}
}

func (w lockWalk) scan(body ast.Node) (hold, lock token.Pos) {
	hold, lock = token.NoPos, token.NoPos
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncLit:
			// A bound closure is judged where it is called, below.
			for _, lit := range w.bound {
				if lit == n {
					return false
				}
			}
		case *ast.DeferStmt:
			h, l := w.scan(n.Call)
			if h != token.NoPos {
				earliest(&hold, w.end)
			}
			if l != token.NoPos {
				earliest(&lock, w.end)
			}
			return false
		case *ast.CallExpr:
			w.call(n, &hold, &lock)
		case *ast.BasicLit:
			if spellsLock(n) {
				earliest(&lock, n.Pos())
			}
		}
		return true
	})
	return hold, lock
}

func (w lockWalk) call(n *ast.CallExpr, hold, lock *token.Pos) {
	switch fun := n.Fun.(type) {
	case *ast.SelectorExpr:
		if subjectHolds[fun.Sel.Name] {
			earliest(hold, n.Pos())
			return
		}
		// A method on this function's own receiver, keyed as the graph keys
		// it: s.applySignatureField reaching the hold discharges it here, at
		// the call. And a helper that takes a row LOCKS at its call site, so
		// moving one statement into a helper cannot hide the order it decides.
		if base, isIdent := fun.X.(*ast.Ident); isIdent && w.recvVar != "" && base.Name == w.recvVar {
			if w.reachesHold(scrubKey(w.recvType, fun.Sel.Name)) {
				earliest(hold, n.Pos())
			}
			if w.reachesLock(scrubKey(w.recvType, fun.Sel.Name)) {
				earliest(lock, n.Pos())
			}
		}
	case *ast.Ident:
		if lit, isBound := w.bound[fun.Name]; isBound {
			h, l := w.scan(lit.Body)
			if h != token.NoPos {
				earliest(hold, n.Pos())
			}
			if l != token.NoPos {
				earliest(lock, n.Pos())
			}
			return
		}
		if subjectHolds[fun.Name] || w.reachesHold(fun.Name) {
			earliest(hold, n.Pos())
		}
		if w.reachesLock(fun.Name) {
			earliest(lock, n.Pos())
		}
	}
}

// boundClosures are the closures a body names, `x := func…` or
// `var x = func…`, by that name.
func boundClosures(body *ast.BlockStmt) map[string]*ast.FuncLit {
	bound := map[string]*ast.FuncLit{}
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for i, rhs := range n.Rhs {
				lit, isLit := rhs.(*ast.FuncLit)
				if !isLit || i >= len(n.Lhs) {
					continue
				}
				if id, isIdent := n.Lhs[i].(*ast.Ident); isIdent {
					bound[id.Name] = lit
				}
			}
		case *ast.ValueSpec:
			for i, value := range n.Values {
				if lit, isLit := value.(*ast.FuncLit); isLit && i < len(n.Names) {
					bound[n.Names[i].Name] = lit
				}
			}
		}
		return true
	})
	return bound
}

// spellsLock reports a string literal holding SQL that takes a row lock.
func spellsLock(lit *ast.BasicLit) bool {
	if lit.Kind != token.STRING {
		return false
	}
	text, err := strconv.Unquote(lit.Value)
	return err == nil && mutatingStatement.MatchString(text)
}

// spellsAMutatingStatement reports whether a function contains SQL that takes a
// row lock. Its own literals only — the transitive question is the call graph's.
func spellsAMutatingStatement(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		lit, isLit := node.(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return !found
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return !found
		}
		if mutatingStatement.MatchString(text) {
			found = true
		}
		return !found
	})
	return found
}
