// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H2

package gates

import (
	"go/ast"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A conditional by-id write that declines to happen says so.
//
// `WHERE id = $1 AND status IN ('queued','running')` is safe from the lost
// update. An interleaved writer leaves the state the predicate names, so the
// UPDATE matches no row. That much updateguardcas_test.go asks for.
//
// This gate asks the other half: whether the caller is told. The two are
// different obligations. One is data integrity, the other truthful reporting.
//
// A function that neither reads `RowsAffected` nor scans a `RETURNING` row
// returns nil for a row it never touched. Its caller reports the act as done.
//
// What counts as reading the outcome is broad by intent. A gate insisting on
// one spelling pushes a correct fix toward a waiver. So any of these count: a
// `RowsAffected` comparison, a scanned `RETURNING` row, or a storekit helper
// that does one of those for the caller.
//
// Where a silent no-op is right, the write says so in declinedWriteIsSilent
// below rather than growing a check nobody wants.

// outcomeMarkers are the selectors that make a declined write visible. The row
// count off the command tag, a row scanned back out, or a storekit helper.
var outcomeMarkers = map[string]bool{
	"RowsAffected":         true,
	"QueryRow":             true,
	"CollectOneRow":        true,
	"CollectExactlyOneRow": true,
	"UpdateReturning":      true,
	"ClaimRow":             true,
}

// returningRow is the other spelling. The statement asks for the row back, so
// a miss arrives as pgx.ErrNoRows rather than as a silent nil.
var returningRow = regexp.MustCompile(`(?i)\bRETURNING\b`)

// declinedWriteIsSilent holds the conditional by-id writes whose caller is
// right not to look, each saying why. A reason describes the act, not the
// missing check.
var declinedWriteIsSilent = gatekit.Waive(map[string]string{
	"backend/internal/modules/activities/audienceretraction.go:RetractDerivedForActivityTx":      "clearing a capture label that may already be absent, under the activity's lock: the retraction is idempotent by design because it runs again whenever a narrowing arrives late",
	"backend/internal/modules/activities/retentionstampproject.go:StampCorrespondenceForProject": "stamping a retention class once (retention_class IS NULL): an activity already classed keeps the class it was given, because a second project touching the same message must not reclassify it",
	"backend/internal/modules/capture/registry_watch.go:RenewWatch":                              "renewing a provider watch against the generation that was read: a connection reconnected since has a watch of its own, and extending the dead generation's would hold a subscription nobody reads",
	"backend/internal/modules/capture/syncstate.go:recordSyncFailure":                            "escalating a connection to reauth_required from connected or error only: one already archived or awaiting reauth is in no state this escalation improves",
	"backend/internal/modules/consent/lift.go:liftAdmittedTx":                                    "lifting a suppression the function has already read FOR UPDATE under the subject's lock: the locked read established revoked_at IS NULL, so the write cannot find the row otherwise",
	"backend/internal/modules/contacts/fillretraction.go:clearFilledValue":                       "retracting a filled value only while the column still holds what the fill wrote: a value the user has since edited is theirs, and clearing it would delete a human's correction to undo a machine's guess",
	"backend/internal/modules/contacts/leadresponseclock.go:startLeadResponseClockTx":            "starting a lead's response clock once (routed_at IS NULL) under the lead's lock: a lead routed twice keeps the first instant, which is the clock the service level is measured against",
	"backend/internal/modules/capture/backfillpager.go:commitBackfillPage":                       "cancelling a run that is no longer queued or running, in the arm the page commit falls to when its own guarded write matched nothing: the function reads THAT tag and discards this one, because a run somebody else finished or cancelled needs no second cancellation",
	"backend/internal/modules/capture/backfillpager.go:failBackfill":                             "failing a backfill that is no longer queued or running: the run somebody else finished or cancelled is the newer fact, and recording an error over it would report a completed import as broken",
	"backend/internal/modules/capture/backfillprogress.go:flushBackfillProgress":                 "page counters for a run that has finished, or for a connection reconnected since (the generation predicate): stale progress landing on a live row would overwrite a newer import's tallies with a dead one's",
	"backend/internal/modules/capture/channelpoll.go:AdvanceChannelPollOffsetTx":                 "a monotonic advance (poll_offset < $3): a poller holding an older offset declining to move the cursor backwards is the predicate working, and the furthest reader has already set it",
	"backend/internal/modules/capture/pending.go:Defer":                                          "rescheduling a row whose claim this worker no longer holds: the claim predicate hands the row to whoever holds it now, and a second deferral would move an attempt the new owner is driving",
	"backend/internal/modules/capture/pendingsweeps.go:AgeOutReviewTx":                           "ageing out a review that is no longer unsure: somebody decided it between the scan and the write, and their decision outranks the window expiring",
	"backend/internal/modules/capture/syncstate.go:recordSyncSuccess":                            "clearing a connection's error status on a good sync, where the ordinary case is a connection that was never in error, so the common path is the no-op",
	"backend/internal/modules/capture/threadverdict.go:Defer":                                    "the thread twin of the counterparty deferral, declining for the same reason: the claim moved, so the row is the new holder's to reschedule",
	"backend/internal/modules/capture/threadverdictmerge.go:ThreadVerdictMergeTx":                "folding one thread key's verdicts into another's under FOR UPDATE: a row whose status changed before the lock is one the merge is right to leave, and the merge is re-run by the next message on either key",
	"backend/internal/modules/contacts/siteread.go:UpdateSiteReadDraft":                          "the draft half of the same progressive read, declining for the same reason: facts arriving after the read stopped belong to a reading nobody is waiting for",
	"backend/internal/modules/contacts/siteread.go:UpdateSiteReadProgress":                       "progress for a read that is no longer running: a cancelled or finished read must not have phase and page counts written back over its final state",
	"backend/internal/modules/webhooks/deliverystore.go:markVisibilityRevoked":                   "revoking a delivery already revoked: idempotent by construction, and the first revocation already carries the reason",
	"backend/internal/modules/webhooks/deliverystore.go:recordOutcome":                           "recording a delivery attempt against a subscription whose visibility was revoked mid-flight: the revocation is the newer fact, and marking it delivered would publish an outcome for a subscriber no longer entitled to it",
})

// declinedWriteOwesACheck holds the writes where a silent no-op loses
// something. It is kept apart from the waivers above because the two say
// opposite things. One is a declined write behaving correctly. The other is a
// declined write nobody hears.
//
// Each names the issue carrying the behaviour change. That is a separate
// review: growing an error where a caller expects nil changes what a worker
// does next.
var declinedWriteOwesACheck = gatekit.Waive(map[string]string{
	"backend/internal/modules/ai/voice_build_run.go:SetBuildStage":               "a stage advances only while the build is running, so a worker whose build was cancelled keeps spending model calls on stages nothing records (#7210)",
	"backend/internal/modules/capture/pending.go:Retire":                         "a verdict reached by a paid model call is written only while this worker still holds the claim, and a stolen claim discards the verdict with the caller told nothing (#7210)",
	"backend/internal/modules/capture/pendingreview.go:LinkProposal":             "the proposal row is created first and attached second, so a row resolved in between leaves a proposal nothing points at (#7210)",
	"backend/internal/modules/capture/pendingreview.go:ResolveReviewedAs":        "a human reviewer's disposition lands only while the row is still unsure, so a decision made twice silently drops one of them and the screen reports both as saved (#7210)",
	"backend/internal/modules/contacts/domaintriageresolve.go:bindTriageDossier": "the dossier binds only while company_id IS NULL, so a concurrent bind to a DIFFERENT company leaves the caller believing it owns a dossier that names somebody else (#7210)",
})

// countingHelpers names every function in one package whose body reads a
// write's outcome. A caller handing its statement to one of them is credited
// with the check that helper performs.
//
// Resolved by name across the package and the in-module packages it imports,
// which is where this delegation lives. comms.Store.update returns ErrTerminal
// on nought rows for a dozen callers. capture's pending writes do the same
// through their own helper.
//
// Asking the caller to count as well would be asking twice. Refusing to resolve
// the callee would turn a correct design into two dozen waivers, and a register
// that large hides the next real one.
func countingHelpers(t *testing.T, cache map[string]map[string]bool, dir string) map[string]bool {
	t.Helper()
	if known, ok := cache[dir]; ok {
		return known
	}
	known := map[string]bool{}
	for _, file := range parsePackageFiles(t, dir) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && outcomeMarkers[sel.Sel.Name] {
					known[fn.Name.Name] = true
				}
				return true
			})
		}
	}
	cache[dir] = known
	return known
}

// execCall says the call is an Exec, whose command tag is the only outcome it
// offers. A QueryRow or a Query carries its own miss as pgx.ErrNoRows.
func execCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Exec"
}

// conditionalArg says one of the call's arguments is a conditional by-id write.
// Written at the call rather than resolved from elsewhere, which is the only
// place a per-statement judgement can be made.
//
// Read through gatekit, which decodes escapes and flattens a `+` chain. The
// source text of a double-quoted statement carries its escapes, and a census
// matching that reports a clean tree over the shape it hunts.
func conditionalArg(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		for _, text := range gatekit.SQLStatementsOf(a) {
			if byIDUpdate.MatchString(text) && conditionalWrite(text) {
				return true
			}
		}
	}
	return false
}

// callsACountingHelper says the function passes its work to one of them.
func callsACountingHelper(fn *ast.FuncDecl, counting map[string]bool) bool {
	var found bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			found = found || counting[callee.Name]
		case *ast.SelectorExpr:
			found = found || counting[callee.Sel.Name]
		}
		return true
	})
	return found
}

func TestAConditionalByIDWriteReportsThatItDeclined(t *testing.T) {
	t.Parallel()
	defer declinedWriteIsSilent.AssertAllMatched(t)
	defer declinedWriteOwesACheck.AssertAllMatched(t)
	heldCache := map[string]map[string][]string{}
	countingCache := map[string]map[string]bool{}
	var unchecked []string
	var corpus int
	// The module root, one up from this package.
	if err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
			return err
		}
		path = filepath.ToSlash(path)
		file, parseErr := gatekit.ParseFile(path, 0)
		if parseErr != nil {
			return parseErr
		}
		var imported []map[string][]string
		for _, dir := range inModuleImportDirs(file) {
			imported = append(imported, heldStatements(t, heldCache, dir))
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// Judged per statement, where the statement sits at its own Exec.
			// recordOutcome in the webhook store writes a delivered row and a
			// failed one, reading the tag of only one. Judging the function
			// would hide the other, and a census reporting less than it sees
			// fails in the one direction no assertion catches.
			var discards bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
					return true
				}
				call, ok := assign.Rhs[0].(*ast.CallExpr)
				if !ok || !execCall(call) || !conditionalArg(call) {
					return true
				}
				if id, ok := assign.Lhs[0].(*ast.Ident); ok && id.Name == "_" {
					discards = true
				}
				return true
			})
			var conditional, reads bool
			for _, lit := range statementsJudged(fn, heldStatements(t, heldCache, filepath.Dir(path)), imported) {
				if byIDUpdate.MatchString(lit) && conditionalWrite(lit) {
					conditional = true
				}
				if returningRow.MatchString(lit) {
					reads = true
				}
			}
			if !conditional {
				continue
			}
			corpus++
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && outcomeMarkers[sel.Sel.Name] {
					reads = true
				}
				return true
			})
			if discards {
				reads = false
			}
			// A fresh set per file. countingHelpers hands back the map it
			// caches, so merging the imports into it would leave those names
			// in the package's cache entry.
			//
			// A later file in the same package would then be credited with a
			// helper from a package it does not import, matched by name alone.
			// The census would read short, in an order set by the walk.
			counting := map[string]bool{}
			for name := range countingHelpers(t, countingCache, filepath.Dir(path)) {
				counting[name] = true
			}
			for _, dir := range inModuleImportDirs(file) {
				for name := range countingHelpers(t, countingCache, dir) {
					counting[name] = true
				}
			}
			if !discards && (reads || callsACountingHelper(fn, counting)) {
				continue
			}
			subject := strings.TrimPrefix(path, "../") + ":" + fn.Name.Name
			if declinedWriteIsSilent.Waived(t, subject) || declinedWriteOwesACheck.Waived(t, subject) {
				continue
			}
			unchecked = append(unchecked, subject)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The floor measures the corpus, not the findings. A register emptying as
	// writes are fixed would otherwise read as a passing census over a tree the
	// walk had stopped seeing.
	// The debt is counted as well as waived, because a waived finding on its
	// own is invisible.
	//
	// Pinning it keeps the gate green while holding the debt where it is. A
	// sixth write owing a check fails here, and each one #7210 fixes lowers
	// this number.
	const writesOwingACheck = 5
	if got := len(declinedWriteOwesACheck.Subjects()); got != writesOwingACheck {
		t.Errorf("%d writes owe an outcome check, pinned at %d: a new one is debt this gate refuses, "+
			"and one fixed lowers the pin", got, writesOwingACheck)
	}

	const fewestConditionalWrites = 20
	if corpus < fewestConditionalWrites {
		t.Fatalf("the walk found %d conditional by-id writes, fewer than the %d known to exist, "+
			"so it is reading a smaller tree than it is reporting on", corpus, fewestConditionalWrites)
	}
	for _, subject := range unchecked {
		t.Errorf("%s writes by id under a state predicate and never reads the outcome.\n"+
			"  The write is safe from a lost update and silent about declining: it returns nil for a row\n"+
			"  it did not touch, and its caller reports the act as done. Read RowsAffected, or scan a\n"+
			"  RETURNING row, or name it in declinedWriteIsSilent with the reason a silent no-op is right.",
			subject)
	}
}
