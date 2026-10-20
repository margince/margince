// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A save that changes nothing changes nothing.
//
// A versioned row moves its version on every UPDATE that reaches it. An edit
// that re-sends the stored values and still writes moves the version, the audit
// log and the event stream. It also makes other editors' If-Match stale.
// storekit.Patch decides whether an edit changed anything (Patch.Empty).
//
// The client update seam is Patch.ApplyGuarded and ApplyGuardedIn. They take
// the caller's If-Match, so the function is an edit a user or an agent sent.
// The census is derived from that seam: every function that reaches it asks
// Empty or Moved, or says here why its write always moves something.
//
// The gate cannot see an edit that writes with its own UPDATE statement; the
// HTTP integration test holds those routes. A helper that only applies is
// ratified below and names the caller that asks.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var clientUpdateSeams = map[string]bool{"ApplyGuarded": true, "ApplyGuardedIn": true}

var emptinessQuestions = map[string]bool{"Empty": true, "Moved": true}

// writesThatAlwaysMove: functions that reach the client seam without asking
// whether the patch is empty, keyed by `path:function`. Each states why the
// write it makes cannot be a repeat of the stored values.
var writesThatAlwaysMove = gatekit.Waive(map[string]string{
	"internal/modules/commissions/decide.go:decideTx":                           "a decision moves the entry from pending to approved or void, and a repeat of the decision is refused by the status guard before the patch is built",
	"internal/modules/activities/lifecycle.go:archiveActivityInTx":              "the archive stamps archived_at on a row the caller has just read as live, so the stamp is a value the row did not hold",
	"internal/modules/contacts/companyarchive.go:archiveCompanyTx":              "the archive stamps archived_at on a row the caller has just read as live, so the stamp is a value the row did not hold",
	"internal/modules/contacts/contactarchive.go:archiveContactRows":            "the archive stamps archived_at on a row the caller has just read as live, so the stamp is a value the row did not hold",
	"internal/modules/contacts/relationship.go:archiveRelationshipWithEvidence": "the archive stamps archived_at on a row the caller has just read as live, so the stamp is a value the row did not hold",
	"internal/modules/projects/project.go:ArchiveProject":                       "the archive stamps archived_at on a row the caller has just read as live, so the stamp is a value the row did not hold",
	"internal/modules/dealrooms/document_write.go:RemoveDocument":               "the archive stamps archived_at on a row the caller has just read as live, so the stamp is a value the row did not hold",
	"internal/modules/dealrooms/room_list.go:archiveRoomTx":                     "the archive moves the room's state to archived and stamps archived_at, and a room already archived is refused before the patch is built",
	"internal/modules/projects/advance.go:recordPhaseTransition":                "the advance names a phase different from the current one, which the transition table refuses to repeat",
	"internal/modules/contracts/contract_lifecycle.go:applyStatusTx":            "a status transition names a status different from the current one, which the transition table refuses to repeat",
	"internal/modules/deals/offer_lifecycle.go:SendOffer":                       "sending moves the offer from draft to sent and freezes its snapshots, and a sent offer is refused before the patch is built",
	"internal/modules/deals/offer_lifecycle.go:AcceptOffer":                     "accepting moves the offer from sent to accepted, and any other status is refused before the patch is built",
	"internal/modules/deals/offer_lifecycle.go:RejectOffer":                     "rejecting moves the offer from sent to rejected, and any other status is refused before the patch is built",
	"internal/modules/deals/dealwrite.go:applyDealPatchGuarded":                 "the helper only applies; the deal update that builds the patch asks whether it is empty before it calls it",
	"internal/modules/contracts/contract_write.go:applyContractUpdate":          "the helper only applies; UpdateContract asks whether the patch is empty before it calls it",
})

// storekitOwnsTheSeam: the files that define the seam or build on it for a
// whole kind of row, keyed by path. Their callers hold the question.
var storekitOwnsTheSeam = gatekit.Waive(map[string]string{
	"internal/platform/database/storekit/patch.go":     "defines the seam, so it has no patch of its own to ask about",
	"internal/platform/database/storekit/legalhold.go": "a hold flips one flag on the row a caller has already resolved, and a repeat of the flag is refused by the hold's own guard",
	"internal/platform/database/storekit/unarchive.go": "a restore clears archived_at on a row the caller has just read as archived, so the clear is a value the row did not hold",
})

func TestAnEditThatReachesTheClientSeamAsksWhetherItChangedAnything(t *testing.T) {
	t.Parallel()
	defer writesThatAlwaysMove.AssertAllMatched(t)
	defer storekitOwnsTheSeam.AssertAllMatched(t)

	files := gatekit.Scope{
		Roots:   []string{"internal/modules"},
		Subject: fileReachesTheClientUpdateSeam,
		Exempt:  storekitOwnsTheSeam,
	}.Files(t)

	reached := 0
	for _, parsed := range files {
		for _, decl := range parsed.File.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			reaches, asks := reachesSeamAndAsks(fn)
			if !reaches {
				continue
			}
			reached++
			key := parsed.Path + ":" + fn.Name.Name
			if asks {
				continue
			}
			if !writesThatAlwaysMove.Waived(t, key) {
				t.Errorf("%s: reaches Patch.ApplyGuarded without asking Patch.Empty or Patch.Moved.\n"+
					"\tA save of the values the row holds would move its version, write an audit row and "+
					"emit an event. Return the current row when the patch is empty (after "+
					"storekit.EnsureIfMatch, so a stale If-Match still answers 409), or ratify the "+
					"function in writesThatAlwaysMove with why its write always moves something.", key)
			}
		}
	}
	// A floor below the real number, so a scan that stopped finding the seam
	// fails instead of reading green over an empty census.
	if reached < 15 {
		t.Errorf("found %d function(s) reaching the client update seam, want at least 15: the walk has "+
			"stopped finding it", reached)
	}
}

func fileReachesTheClientUpdateSeam(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall && calledMethodIn(call, clientUpdateSeams) {
			found = true
		}
		return !found
	})
	return found
}

// reachesSeamAndAsks reads one function, closures included: the update body of
// most stores is the closure handed to the transaction runner.
func reachesSeamAndAsks(fn *ast.FuncDecl) (reaches, asks bool) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch {
		case calledMethodIn(call, clientUpdateSeams):
			reaches = true
		case calledMethodIn(call, emptinessQuestions):
			asks = true
		}
		return true
	})
	return reaches, asks
}

func calledMethodIn(call *ast.CallExpr, names map[string]bool) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	return isSelector && names[selector.Sel.Name]
}
