// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A detached write says what bounds it.
//
// context.WithoutCancel is how work that must OUTLIVE its request escapes the
// caller's cancellation — an import commit that would otherwise leave a run
// half-written, a failure mark that exists precisely for the request that timed
// out, an idempotency key that must become free again. Every one of those is a
// good reason, and every one of them is written down at the call site.
//
// What none of them said is what bounds the work AFTERWARDS. Detaching removes
// the caller's DEADLINE along with their cancellation, so a detached write with
// nothing of its own runs until the process does: a connection held, a
// goroutine pinned, and on the two unauthenticated edges in this tree, one of
// each per request that reached them.
//
// The tree already had the answer and had not written it as a rule. Seven named
// constants spell it out one site at a time — CleanupTimeout, traceWriteTimeout,
// terminalWriteTimeout, detachedWriteTimeout, failureRecordTimeout,
// writeBackLocalTimeout, logoWriteBackTimeout — and twenty call sites wrap the
// detach in a WithTimeout. Seven did not, and a reader could not tell those from
// the ones that had thought about it and concluded the work was bounded
// elsewhere. That is what this fixes: not "every detach is bounded", but "every
// detach SAYS", so the two are distinguishable.
//
// Deliberately syntactic. It reads the expression, not the program: whether a
// budget is the right length is a judgement the constant's own comment carries,
// and a gate that tried to decide it would be wrong about every site at once.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// boundedElsewhere names the detaches that carry no deadline on the line, and
// says what bounds each instead.
//
// The waiver is the POINT rather than an escape from it. Both shapes below are
// legitimate and neither is spellable as a WithTimeout at the detach: one hands
// the context to a function that bounds it, and one is a process lifetime that
// must not be bounded at all. What the gate refuses is the third shape — a
// detach that is neither, with nobody having said which it meant to be.
var boundedElsewhere = gatekit.Waive(map[string]string{
	"internal/modules/contacts/handlers_companylogo.go": "handed straight to writeBackTrimmedLogo, which opens by wrapping it in logoWriteBackTimeout. Bounding it here as well would put two budgets on one write and leave a reader guessing which one governs",
	"cmd/api/mcpapps.go":                "a process-lifetime loop, not a write: WithCancel over the detached context, cancelled when the server stops. A deadline here would retire the refresh loop mid-run on a schedule nothing chose",
	"internal/modules/ai/modelfacts.go": "handed to the catalog's ask, and the only ask — the OpenRouter model-list read — opens by wrapping it in openRouterModelsTimeout. The detach exists so one caller giving up does not fail the others joined on the same read",
	"cmd/api/relay.go":                  "the same shape for the outbox relay, and for the same reason — the cancel is the shutdown signal, and the loop is meant to run as long as the process does",
})

// TestEveryDetachedContextSaysWhatBoundsIt is the census.
func TestEveryDetachedContextSaysWhatBoundsIt(t *testing.T) {
	t.Parallel()
	read := 0
	waivedDetaches := map[string]int{}
	for _, root := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			for range unboundedDetaches(file) {
				read++
				if boundedElsewhere.Waived(t, path) {
					waivedDetaches[path]++
					continue
				}
				t.Errorf("%s: detaches a context from its caller without a deadline of its own, so the "+
					"work it carries runs until the process does. Wrap it — "+
					"context.WithTimeout(context.WithoutCancel(ctx), <a named budget>) — or, if something "+
					"else already bounds it, say so in boundedElsewhere so the next reader can tell this "+
					"apart from a detach that simply forgot", path)
			}
			read += boundedIn(file)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	boundedElsewhere.AssertAllMatched(t)
	// A waiver names a FILE, so a second detach added to a waived file would
	// inherit a reason written about the first one — which is the quiet half of
	// this gate failing. Each reason below is about one call, so each file is
	// allowed exactly one.
	for path, count := range waivedDetaches {
		if count > 1 {
			t.Errorf("%s carries %d unbounded detaches and one waiver, so all but one of them are covered "+
				"by a reason written about a different call. Bound the new ones, or split the waiver by "+
				"moving the calls apart", path, count)
		}
	}
	// Under-recognition is the one failure this may not have: a reader that
	// matched nothing would report a clean tree in exactly the same words as a
	// clean tree does. This tree detaches in dozens of places.
	if read < 20 {
		t.Fatalf("this census read %d detached context(s) and this tree carries far more: the reader has "+
			"stopped matching rather than the tree having changed, and PASS cannot tell the two apart", read)
	}
}

// unboundedDetaches counts the context.WithoutCancel calls a file makes that are
// NOT the first argument of a WithTimeout or WithDeadline beside them.
//
// The bounded shape is one expression — WithTimeout(WithoutCancel(ctx), budget)
// — so the whole question is whether the detach sits in that position. Reading
// it as one expression is also what keeps the gate honest about its own reach:
// a detach bound to a variable and wrapped three lines later is reported, and
// the fix is either to fold it into one expression or to say in the waiver what
// bounds it. Either way the next reader can see the answer without tracing.
func unboundedDetaches(file *ast.File) int {
	bounded := map[*ast.CallExpr]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall || len(call.Args) == 0 {
			return true
		}
		if !namesContextFunc(call.Fun, "WithTimeout") && !namesContextFunc(call.Fun, "WithDeadline") {
			return true
		}
		if inner, isCall := call.Args[0].(*ast.CallExpr); isCall && namesContextFunc(inner.Fun, "WithoutCancel") {
			bounded[inner] = true
		}
		return true
	})
	loose := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if isCall && namesContextFunc(call.Fun, "WithoutCancel") && !bounded[call] {
			loose++
		}
		return true
	})
	return loose
}

// boundedIn counts the detaches a file bounds on the spot, which is what the
// census below counts as having READ. Without them the floor would measure only
// the failures, and a tree that bounded every detach would read as one this
// gate could no longer see.
func boundedIn(file *ast.File) int {
	n := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || len(call.Args) == 0 {
			return true
		}
		if !namesContextFunc(call.Fun, "WithTimeout") && !namesContextFunc(call.Fun, "WithDeadline") {
			return true
		}
		if inner, isCall := call.Args[0].(*ast.CallExpr); isCall && namesContextFunc(inner.Fun, "WithoutCancel") {
			n++
		}
		return true
	})
	return n
}

// namesContextFunc reports whether an expression calls context.<name>, dot
// imports included.
//
// The import is not checked. `context` is the standard library's and nothing in
// this tree shadows it; a package that did would be the finding, not a false
// positive this should quietly swallow.
func namesContextFunc(fun ast.Expr, name string) bool {
	switch node := fun.(type) {
	case *ast.SelectorExpr:
		pkg, isIdent := node.X.(*ast.Ident)
		return isIdent && pkg.Name == "context" && node.Sel.Name == name
	case *ast.Ident:
		return node.Name == name
	default:
		return false
	}
}
