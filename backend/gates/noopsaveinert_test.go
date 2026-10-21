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
	"go/parser"
	"go/token"
	"sort"
	"strconv"
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
	"internal/modules/deals/product.go:ArchiveProductAt":                        "the archive stamps archived_at after reading the row as live; a repeated archive cannot reach the patch",
	"internal/modules/deals/offerarchive.go:ArchiveOfferAt":                     "the archive stamps archived_at after reading the row as live; a repeated archive cannot reach the patch",
	"internal/modules/deals/offer_template.go:ArchiveOfferTemplateAt":           "an already archived template returns after its version check; only a live template reaches the archive stamp",
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

// reachesSeamAndAsks reads one function, closures included, because the update
// body of most stores is the closure handed to the transaction runner. A patch
// counts as asked only when the same variable was asked Empty or Moved earlier in
// the source than the call that applies it. An unrelated method of that name, or
// a question asked after the write, does not satisfy the census.
func reachesSeamAndAsks(fn *ast.FuncDecl) (reaches, asks bool) {
	asked := map[string]token.Pos{}
	var seams []*ast.CallExpr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch {
		case calledMethodIn(call, clientUpdateSeams):
			seams = append(seams, call)
		case calledMethodIn(call, emptinessQuestions):
			if name := patchVariable(call); name != "" {
				if first, seen := asked[name]; !seen || call.Pos() < first {
					asked[name] = call.Pos()
				}
			}
		}
		return true
	})
	if len(seams) == 0 {
		return false, false
	}
	for _, seam := range seams {
		first, seen := asked[patchVariable(seam)]
		if !seen || first > seam.Pos() {
			return true, false
		}
	}
	return true, true
}

// patchVariable is the variable a method is called on, or empty when the
// receiver is not a plain variable.
func patchVariable(call *ast.CallExpr) string {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return ""
	}
	ident, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return ""
	}
	return ident.Name
}

func calledMethodIn(call *ast.CallExpr, names map[string]bool) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	return isSelector && names[selector.Sel.Name]
}

const noOpProbeFile = "internal/compose/integration/nosavechanges_http_integration_test.go"

// unprobedSaveRoutes ratifies the versioned save routes no HTTP probe sends a
// save of the stored values yet, keyed by `METHOD path`. The follow-up names
// them all, so the register shrinks as each is probed or ruled out.
var unprobedSaveRoutes = gatekit.Waive(map[string]string{
	"PATCH /activities/{id}/audience":                 "margince#8133 probes it",
	"PATCH /companies/{id}/facts/{factKey}":           "margince#8133 probes it",
	"PATCH /companies/{id}/profile-fields/{field}":    "margince#8133 probes it",
	"PATCH /custom-fields/{id}":                       "margince#8133 probes it",
	"PATCH /deal-rooms/{id}":                          "margince#8133 probes it",
	"PATCH /deal-rooms/{id}/documents/{documentId}":   "margince#8133 probes it",
	"PATCH /provider-connections/{provider}":          "held by TestAConfigurationSaveOfTheStoredSettingsChangesNothing at the store, which needs a registered provider the HTTP harness lacks",
	"PATCH /roles/{key}":                              "margince#8133 probes it",
	"PATCH /roles/{key}/objects/{object}":             "margince#8133 probes it",
	"PATCH /scheduled-sends/{id}":                     "margince#8133 probes it",
	"PATCH /signals/{id}":                             "margince#8133 probes it",
	"PATCH /stages/{id}":                              "margince#8133 probes it",
	"PATCH /stages/{id}/exit-criteria/{criterion_id}": "margince#8133 probes it",
	"PATCH /views/{id}":                               "margince#8133 probes it",
	"PATCH /voice-profiles/{id}":                      "margince#8133 probes it",
	"PATCH /voice-profiles/{id}/sources/{sourceId}":   "margince#8133 probes it",
	"PATCH /automations/{id}":                         "held by the automation store tests at the store; margince#8133 adds the HTTP probe",
	"PATCH /offers/{id}":                              "margince#8133 probes it",
	"PUT /mail-drafts":                                "margince#8133 probes it",
	"PUT /pipelines/{id}/stage-order":                 "margince#8133 probes it",
})

// TestEveryVersionedSaveRouteIsProbedForANoOp derives the client save routes
// from the contract: every PATCH or PUT taking If-Match. Each is probed by an
// HTTP test that re-sends its record's own values, or is ratified above. The
// probe table is read from the integration file so a probe cannot be listed
// without a test of that name existing.
func TestEveryVersionedSaveRouteIsProbedForANoOp(t *testing.T) {
	t.Parallel()
	defer unprobedSaveRoutes.AssertAllMatched(t)

	probed := noOpProbedRoutesFromSource(t)
	contract := map[string]bool{}
	for _, route := range conditionalOperations(t, ifMatchContractDocument(t)) {
		if strings.HasPrefix(route, "PATCH ") || strings.HasPrefix(route, "PUT ") {
			contract[route] = true
		}
	}
	if len(contract) < 30 {
		t.Fatalf("found %d versioned save route(s) in the contract, want at least 30: the census has stopped "+
			"finding them", len(contract))
	}
	var routes []string
	for route := range contract {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		if _, ok := probed[route]; ok {
			continue
		}
		if !unprobedSaveRoutes.Waived(t, route) {
			t.Errorf("%s: no HTTP probe sends it a save of the values its record holds.\n"+
				"\tAdd it to noOpProbedRoutes in %s with the test that probes it, or ratify it in "+
				"unprobedSaveRoutes with why a repeat save cannot move the version.", route, noOpProbeFile)
		}
	}
	for route := range probed {
		if !contract[route] {
			t.Errorf("%s is probed but the contract has no such versioned route: correct the table", route)
		}
	}
}

// noOpProbedRoutesFromSource reads the probe table out of the integration test
// file, and checks each value names a test function declared beside it.
func noOpProbedRoutesFromSource(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), noOpProbeFile, nil, 0)
	if err != nil {
		t.Fatalf("reading the probe table: %v", err)
	}
	declared := map[string]bool{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			declared[fn.Name.Name] = true
		}
	}
	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != "noOpProbedRoutes" || len(spec.Values) != 1 {
			return true
		}
		table, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, element := range table.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			route, routeErr := stringLiteralOf(pair.Key)
			test, testErr := stringLiteralOf(pair.Value)
			if routeErr != nil || testErr != nil {
				t.Errorf("the probe table holds an entry that is not a pair of string literals")
				continue
			}
			if !declared[test] {
				t.Errorf("%s names %s, which %s does not declare", route, test, noOpProbeFile)
			}
			out[route] = test
		}
		return false
	})
	if len(out) == 0 {
		t.Fatalf("%s declares no noOpProbedRoutes table, or it is empty", noOpProbeFile)
	}
	return out
}

func stringLiteralOf(expr ast.Expr) (string, error) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", strconv.ErrSyntax
	}
	return strconv.Unquote(lit.Value)
}
