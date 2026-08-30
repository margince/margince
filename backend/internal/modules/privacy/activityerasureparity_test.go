// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Destroying an activity's text is not the erasure. Everything the text left
// behind is: the verbatim provider original, the vectors, the provenance of
// fields that are now gone, the transcript readings, the proposals quoting it,
// the attachments, the transmitted copy.
//
// This package has shipped the same defect twice by keeping that list in two
// places. Migration 0291 had to add counterparty_email to a guard written from
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

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
	"ErasePerson": "the Art. 17 cascade, whose purges are scoped to the SUBJECT rather than to one activity: raw_capture by every address and channel identity of theirs (purgeDerivedTraces, purgeChannelRawCapture) — which reaches originals an activity's own natural-key join cannot — plus redactApprovalsCitingActivities under ErasedSourceWithdrawal, purgeTranscriptReadings, the attachments and the embeddings. Per-activity purging here would be N statements for one subject and would still miss the channel lane",
})

// contentPurger is the one function that finishes an activity-content erasure.
//
// Held by: TestEveryContentDestroyerFinishesTheErasure
// (backend/internal/modules/privacy/activityerasureparity_test.go)
const contentPurger = "purgeContentDerivedFrom"

// destroysActivityBody reports whether a statement clears an activity's text.
// `body = NULL` on the activity relation is the act every arm here performs and
// the one that leaves the derived copies behind.
func destroysActivityBody(statement string) bool {
	low := strings.ToLower(statement)
	return strings.Contains(low, "update activity") && strings.Contains(low, "body = null")
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
			for _, statement := range gatekit.SQLStatementsOf(fn) {
				if destroysActivityBody(statement) {
					destroyers[fn.Name.Name] = true
				}
			}
			for _, called := range functionsCalledIn(fn) {
				if called == contentPurger {
					purgers[fn.Name.Name] = true
					continue
				}
				callers[called] = append(callers[called], fn.Name.Name)
			}
		}
	}
	return destroyers, purgers, callers
}

// functionsCalledIn names every function a body calls, by the identifier at the
// call — a package-local function by its own name, a method by its selector.
func functionsCalledIn(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch called := call.Fun.(type) {
		case *ast.Ident:
			out = append(out, called.Name)
		case *ast.SelectorExpr:
			out = append(out, called.Sel.Name)
		}
		return true
	})
	return out
}
