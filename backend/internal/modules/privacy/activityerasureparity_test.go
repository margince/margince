// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Destroying an activity's text is not the erasure. Everything the text left
// behind is: the verbatim provider original, the vectors, the provenance of
// fields that are now gone, the transcript readings, the proposals quoting it,
// the attachments, the transmitted copy.
//
// This package has shipped the same defect twice by keeping that list in two
// places. A migration had to add counterparty_email to a guard written from
// the smaller of two content lists. Then the lift arm destroyed the body and
// left the raw_capture row standing, joined on the (source_system, source_id)
// pair the lift deliberately keeps — so an Art. 15 export served the verbatim
// original back, on a record whose statutory floor had expired, with no erasure
// request anywhere in the chain. The file carried the invariant in a comment
// the whole time: "a record must not be more thoroughly erased by the clock
// than by a controller's decision."
//
// So the list lives in ONE function, and this is what says nobody grew a second
// one. It reads this package's own source, because the property is structural:
// a second list reads correctly at every call site, and the half that drifts
// leaves originals behind while reporting success.
//
// WHAT THIS CENSUS IS, AND IS NOT. It is structural, not a flow analysis. A
// destroyer is accepted the moment its body contains a call to the purger — it
// does not check that the call runs for the SAME activity, that it precedes the
// body clear, or that it is not on a branch the destroying path never takes. So
// a function that nulled one activity's body and purged a different one's, or
// purged only inside an `if`, would pass.
//
// That limit is relied upon rather than enforced, and the reason it is
// acceptable HERE is that these functions are short, each acts on one id, and
// each is exercised end to end by an integration test that reads the raw
// capture back (erasure_restriction_integration_test.go). A destroyer that grew
// branches, a loop over several ids, or an id it did not receive would be past
// what this reader can judge — and the right response then is a test that reads
// the rows, not a cleverer parser here. A reader who finds one should say so
// rather than trusting a green run.
//
// It also does not resolve which TYPE a method call lands on. Two types
// declaring the same method name would share an entry; the package has no such
// pair today.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// finishedAnotherWay are the destroyers that complete the erasure without the
// shared helper, each with what they do instead.
//
// One entry, and it is a different SHAPE rather than a shortcut. The Art. 17
// cascade erases everything about a SUBJECT, so its purges are scoped to the
// person: raw_capture by every address of theirs and by every channel identity
// (purgeDerivedTraces, purgeChannelRawCapture), which reaches originals the
// activity's own (source_system, source_id) join cannot — a channel poller
// stores its original under the provider's redelivery key. Routing it through
// the per-activity helper would be N purges for one subject AND would miss
// exactly those.
//
// It does the rest too, on the same scope: redactApprovalsCitingActivities
// under ErasedSourceWithdrawal (erasure.go), purgeTranscriptReadings over the
// held ids (erasure_restrict.go), the attachments and the embeddings.
var finishedAnotherWay = gatekit.Waive(map[string]string{
	"method ErasePerson": "the Art. 17 cascade, whose purges are scoped to the SUBJECT rather than to one activity: raw_capture by every address and channel identity of theirs (purgeDerivedTraces, purgeChannelRawCapture) — which reaches originals an activity's own natural-key join cannot — plus redactApprovalsCitingActivities under ErasedSourceWithdrawal, purgeTranscriptReadings, the attachments and the embeddings. Per-activity purging here would be N statements for one subject and would still miss the channel lane",
})

// contentPurger is the one function that finishes an activity-content erasure.
//
// Held by: TestEveryContentDestroyerFinishesTheErasure
// (backend/internal/modules/privacy/activityerasureparity_test.go)
const contentPurger = "purgeContentDerivedFrom"

// updatesActivity and nullsBody read the two halves of the act, normalised.
//
// Substring matching on `update activity` and `body = null` was the first
// spelling of this and it was too literal to be a census: `UPDATE "activity"`,
// `UPDATE public.activity`, `UPDATE ONLY activity`, an alias after the target,
// or `body=NULL` with no spaces all destroy the same text and all read as a
// clean tree. Under-recognition is the one direction this file must not fail
// in — a second content-destroying list that spells its UPDATE differently is
// exactly the defect it exists to catch.
var (
	updatesActivity = regexp.MustCompile(`(?s)\bupdate\s+(?:only\s+)?(?:"?[a-z_][a-z_0-9]*"?\s*\.\s*)?"?activity"?(?:\s|$)`)
	nullsBody       = regexp.MustCompile(`(?s)\b(?:"?[a-z_][a-z_0-9]*"?\s*\.\s*)?"?body"?\s*=\s*null\b`)
)

// destroysActivityBody reports whether a statement clears an activity's text.
// `body = NULL` on the activity relation is the act every arm here performs and
// the one that leaves the derived copies behind.
func destroysActivityBody(statement string) bool {
	low := strings.ToLower(statement)
	return updatesActivity.MatchString(low) && nullsBody.MatchString(low)
}

// TestEveryContentDestroyerFinishesTheErasure is the census.
func TestEveryContentDestroyerFinishesTheErasure(t *testing.T) {
	t.Parallel()
	defer finishedAnotherWay.AssertAllMatched(t)
	destroyers, purgers, callers := readActivityErasureShape(t)

	if len(destroyers) < 2 {
		t.Fatalf("found %d function(s) destroying an activity's body and expects at least 2 — the sweep's erase action and the lift both do, so the reader has gone quiet rather than the tree having lost one: %v",
			len(destroyers), destroyers)
	}
	for name := range destroyers {
		if purgers[name] || finishedAnotherWay.Waived(t, name) {
			continue
		}
		// A destroyer that does not finish the job itself is allowed — the
		// shared lift statement is one, because the restriction lift and the
		// erasure have to be a single statement (the 0290 guard admits nothing
		// else). What is NOT allowed is a caller of it that forgets.
		reached := callers[name]
		if len(reached) == 0 {
			t.Errorf("%s destroys an activity's body, does not call %s, and nothing in this package calls it — so whatever runs it erases the text and leaves the provider original, the vectors and the proposals quoting it",
				name, contentPurger)
			continue
		}
		for _, caller := range reached {
			if !purgers[caller] && !finishedAnotherWay.Waived(t, caller) {
				t.Errorf("%s destroys an activity's body through %s and never calls %s — the parsed copy goes and the verbatim original stands, joined on the (source_system, source_id) pair the erasure keeps, for an Art. 15 export to serve back",
					caller, name, contentPurger)
			}
		}
	}
}

// readActivityErasureShape answers three things about this package: which
// functions destroy an activity's body, which call the shared purge, and who
// calls whom.
func readActivityErasureShape(t *testing.T) (destroyers, purgers map[string]bool, callers map[string][]string) {
	t.Helper()
	destroyers, purgers, callers = map[string]bool{}, map[string]bool{}, map[string][]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			declared := declarationKey(fn)
			for _, statement := range gatekit.SQLStatementsOf(fn) {
				if destroysActivityBody(statement) {
					destroyers[declared] = true
				}
			}
			for _, called := range functionsCalledIn(fn) {
				if called == methodKey+contentPurger {
					purgers[declared] = true
					continue
				}
				callers[called] = append(callers[called], declared)
			}
		}
	}
	return destroyers, purgers, callers
}

// A method and a package function may share a name — RetentionService's
// eraseActivityContent does today — so the two are kept apart. Folding them
// would credit a package function with every `obj.foo()` in the package, and
// then demand purger or waiver status from a function that never calls the
// destroyer at all.
//
// What this does NOT resolve is which TYPE a method call lands on: that needs
// type information this reader does not load. Two types declaring the same
// method name would share an entry. The header says so; the package has no such
// pair today, and the census would err toward reporting rather than toward
// silence if it grew one.
const (
	methodKey   = "method "
	functionKey = "func "
)

// declarationKey names a declaration the way a call to it is recorded.
func declarationKey(fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		return methodKey + fn.Name.Name
	}
	return functionKey + fn.Name.Name
}

// functionsCalledIn names every function a body calls, by the identifier at the
// call — a package-local function by its own name, a method by its selector,
// each tagged with which it is.
func functionsCalledIn(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch called := call.Fun.(type) {
		case *ast.Ident:
			out = append(out, functionKey+called.Name)
		case *ast.SelectorExpr:
			out = append(out, methodKey+called.Sel.Name)
		}
		return true
	})
	return out
}
