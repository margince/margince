// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// The subject lock is the FIRST row a transaction takes.
//
// Art. 17 erasure is subject-first and always has been: anonymizeSubjectRows
// runs `UPDATE person … archived_at` and then, in the same transaction, deletes
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
var locksBeforeTheSubject = gatekit.Waive(map[string]string{})

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
		fset := token.NewFileSet()
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

// holdAndFirstLock returns the position where this function holds its subject
// and the position of the first row it locks, either of which may be NoPos.
//
// A CALL to a function that reaches the hold counts as the hold, at the call's
// own position: that is the LinkedIn shape, where the write sat in the caller
// and the lock two frames down. A call to a function that merely writes is not
// counted as a lock, which is the direction that under-reports — stated rather
// than hidden, because resolving it needs the callee's statements at the
// caller's position and this walk has one file at a time.
func holdAndFirstLock(
	fn *ast.FuncDecl, recvType, recvVar string,
	reachesHold func(string) bool, reachesLock func(string) bool,
) (hold, lock token.Pos) {
	hold, lock = token.NoPos, token.NoPos
	mark := func(at *token.Pos, pos token.Pos) {
		if *at == token.NoPos || pos < *at {
			*at = pos
		}
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			switch fun := n.Fun.(type) {
			case *ast.SelectorExpr:
				if subjectHolds[fun.Sel.Name] {
					mark(&hold, n.Pos())
					break
				}
				// A method on this function's own receiver, keyed as the graph
				// keys it: s.applySignatureField reaching the hold discharges
				// it here, at the call.
				if base, isIdent := fun.X.(*ast.Ident); isIdent && recvVar != "" &&
					base.Name == recvVar {
					if reachesHold(scrubKey(recvType, fun.Sel.Name)) {
						mark(&hold, n.Pos())
					}
					// And a helper that takes a row LOCKS at its call site.
					// Without this the walk reads only the SQL a function
					// spells itself, so moving one statement into a helper
					// hides the ordering it decides — which is exactly how a
					// token-then-subject inversion reached this tree.
					if reachesLock(scrubKey(recvType, fun.Sel.Name)) {
						mark(&lock, n.Pos())
					}
				}
			case *ast.Ident:
				if subjectHolds[fun.Name] || reachesHold(fun.Name) {
					mark(&hold, n.Pos())
				}
				if reachesLock(fun.Name) {
					mark(&lock, n.Pos())
				}
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(n.Value)
			if err != nil {
				return true
			}
			if mutatingStatement.MatchString(text) {
				mark(&lock, n.Pos())
			}
		}
		return true
	})
	return hold, lock
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
