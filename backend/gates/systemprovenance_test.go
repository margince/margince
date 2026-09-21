// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A scheduled pass binds its provenance through ONE helper.
//
// The system principal and a fresh correlation id are two calls that belong
// together: every audit row and outbox event such a pass leaves carries both,
// and a pass that binds only one writes a trail nobody can read back — rows
// with no actor, or rows nobody can replay as one story. Neither omission fails
// anything at the call site, and neither is visible until somebody goes looking
// for what the machine did.
//
// They were spelled together in forty-odd places, and the shape had already
// cost something: internal/compose/integration/scope.go built the retention
// pass's provenance BY HAND, which about thirty integration suites then ran the
// retention engine under. Deleting the WithActor line from the production
// helper left every one of them green, because each was building the context it
// was checking for — AGENTS.md's review-loop rule 6, exactly.
//
// So principal.SystemActing is the one spelling, and this is what keeps it one:
// a system actor bound beside a freshly minted correlation id, anywhere but
// inside the helper, is a second answer to the question the helper answers.
//
// It does NOT forbid principal.WithActor. A bus consumer re-binds the
// triggering event's correlation id rather than minting one, and a human
// request's actor comes from authentication; neither is this shape. The
// subject is the PAIR, and only where the id is `ids.NewV7()`.

import (
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// principalPath is the package that owns both halves and the helper that binds
// them together.
const principalPath = "github.com/margince/margince/backend/internal/shared/kernel/principal"

// systemActingHelper names the helper this gate exempts, being the place the
// pair is spelled.
//
// Held by: TestASystemPassBindsItsProvenanceThroughOneHelper (this file)
const systemActingHelper = "SystemActing"

// inheritedCorrelationBindings ratifies a site that binds the system actor and
// mints an id CONDITIONALLY. SystemActing always mints, so pushing one of these
// through it would overwrite a trace the caller already opened — the opposite
// of what the site is for.
var inheritedCorrelationBindings = gatekit.Waive(map[string]string{
	"internal/modules/integrations/runactor.go:actingForProvider": "a connector run inherits the correlation id of whatever opened it — a webhook " +
		"delivery, a rep's request — and mints one only when nothing did, so the purchase and the claim rows it writes replay as one story with the call that asked for them",
})

func TestASystemPassBindsItsProvenanceThroughOneHelper(t *testing.T) {
	t.Parallel()
	defer inheritedCorrelationBindings.AssertAllMatched(t)
	// internal/ alone, because cmd/ holds no such binding any more — the three
	// binaries wire and start, and every pass that acts is behind compose. A
	// subject appearing there is not missed: Scope sweeps the negative space
	// and reports one outside every root.
	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileBindsASystemActor,
	}.Files(t)

	judged := 0
	for _, parsed := range files {
		qualifier, dotImported := gatekit.ImportedAs(parsed.File, principalPath)
		if dotImported {
			t.Errorf("%s dot-imports the principal package, which leaves both halves of this pair bare and unreadable here", parsed.Path)
			continue
		}
		for _, fn := range functionsIn(parsed.File) {
			if fn.Name.Name == systemActingHelper {
				// The helper itself. It is the one place the pair is spelled,
				// which is the whole subject.
				continue
			}
			judged++
			for _, actor := range mintedProvenancePairsIn(fn.Body, qualifier) {
				if inheritedCorrelationBindings.Waived(t, parsed.Path+":"+fn.Name.Name) {
					continue
				}
				t.Errorf("%s:%s binds %s as the actor and mints a correlation id onto the same context, which is what principal.SystemActing is.\n"+
					"\tCall it with this pass's own name — the name is the caller's, so nothing is flattened — rather than spelling the pair again.",
					parsed.Path, fn.Name.Name, actor)
			}
		}
	}
	if judged < systemProvenanceFunctionFloor {
		t.Fatalf("read only %d function(s) in files that bind a system actor, expected at least %d — a walk this short reports nothing and reads exactly like a tree with one spelling",
			judged, systemProvenanceFunctionFloor)
	}
}

// systemProvenanceFunctionFloor guards against a vacuous pass. The files that
// bind a system actor hold roughly two hundred functions between them; the
// floor sits low enough that retiring a pass does not drag it along, and high
// enough that a sweep that found nothing is reported rather than read as clean.
const systemProvenanceFunctionFloor = 60

// fileBindsASystemActor is the subject predicate, asked the same way inside the
// walk roots and outside them.
func fileBindsASystemActor(_ string, file *ast.File) bool {
	qualifier, dotImported := gatekit.ImportedAs(file, principalPath)
	if dotImported {
		return true
	}
	if qualifier == "" && (file.Name == nil || file.Name.Name != "principal") {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if selects(n, qualifier, "PrincipalSystem") {
			found = true
		}
		return !found
	})
	return found
}

// mintedProvenancePairsIn reports each place a body binds a system actor and
// mints a correlation id ONTO THE SAME CONTEXT, naming what was bound.
//
// Same context, not merely the same body: runnerresume.go binds the system
// actor onto a context carrying the triggering EVENT's correlation id, and
// separately mints a fresh one for an agent's own principal. A rule that paired
// anything with anything inside one function reported that as a second
// spelling, which it is not — and a gate that cries wolf over correct code is
// one the next author waives rather than reads.
//
// Two shapes, because both appear: the pair nested in one expression, and the
// pair spread over two statements through a named context.
func mintedProvenancePairsIn(body *ast.BlockStmt, qualifier string) []string {
	mintedOnto := map[string]bool{}
	actorInto := map[string]string{}
	var findings []string

	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if isCall && selects(call.Fun, qualifier, "WithCorrelationID") && mintsAFreshID(call) {
			if inner, isInner := call.Args[0].(*ast.CallExpr); isInner &&
				selects(inner.Fun, qualifier, "WithActor") && bindsSystemPrincipal(inner, qualifier) {
				findings = append(findings, actorNameIn(inner, qualifier))
				return true
			}
			if named, isNamed := call.Args[0].(*ast.Ident); isNamed {
				mintedOnto[named.Name] = true
			}
			return true
		}
		assigned, into := assignedContext(n)
		if assigned == nil || !selects(assigned.Fun, qualifier, "WithActor") || !bindsSystemPrincipal(assigned, qualifier) {
			return true
		}
		if into != "" {
			actorInto[into] = actorNameIn(assigned, qualifier)
		}
		return true
	})

	for name, actor := range actorInto {
		if mintedOnto[name] {
			findings = append(findings, actor)
		}
	}
	sort.Strings(findings)
	return findings
}

// assignedContext reads a call whose result continues one named context:
// `v = call(v, …)`, `v := call(v, …)`, or `return call(v, …)`.
//
// The RETURN form is not a nicety. jobs_aiactivity.go mints the id and then
// returns the actor binding in one statement, which is the same pair written
// the other way round — and a reader that saw only assignments would report it
// as absent, which is the one direction a census must not fail in.
func assignedContext(n ast.Node) (*ast.CallExpr, string) {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		// The context continues into the NAME assigned, which is where the
		// mint finds it. Not the argument: the binder is commonly handed
		// `principal.WithWorkspaceID(ctx, ws)` rather than a bare name, and
		// keying on that would leave every such site unread.
		if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
			return nil, ""
		}
		name, isIdent := stmt.Lhs[0].(*ast.Ident)
		call, isCall := stmt.Rhs[0].(*ast.CallExpr)
		if !isIdent || !isCall {
			return nil, ""
		}
		return call, name.Name
	case *ast.ReturnStmt:
		if len(stmt.Results) != 1 {
			return nil, ""
		}
		call, isCall := stmt.Results[0].(*ast.CallExpr)
		if !isCall {
			return nil, ""
		}
		return call, contextArgName(call)
	}
	return nil, ""
}

// contextArgName is the name of the context a binder was handed. A return
// carries its result nowhere this walk can name, so the argument is what ties
// it to the mint on the statement before.
func contextArgName(call *ast.CallExpr) string {
	if len(call.Args) == 0 {
		return ""
	}
	named, isNamed := call.Args[0].(*ast.Ident)
	if !isNamed {
		return ""
	}
	return named.Name
}

// actorNameIn is the id the principal literal names, for the finding to quote.
func actorNameIn(call *ast.CallExpr, qualifier string) string {
	for _, arg := range call.Args {
		literal, isLiteral := arg.(*ast.CompositeLit)
		if !isLiteral || !selects(literal.Type, qualifier, "Principal") {
			continue
		}
		for _, element := range literal.Elts {
			pair, isPair := element.(*ast.KeyValueExpr)
			if !isPair {
				continue
			}
			if key, isIdent := pair.Key.(*ast.Ident); isIdent && key.Name == "ID" {
				return types.ExprString(pair.Value)
			}
		}
	}
	return "a system principal"
}

// bindsSystemPrincipal answers whether this WithActor call names a system
// principal acting on NOBODY's behalf — Type and ID and nothing else, which is
// exactly what the helper expresses.
//
// A principal carrying UserID or OnBehalfOf states a further fact: the pass is
// acting for a named human, which is what puts that human on the rows it
// writes (the rate refresh a rep asked for, a claimed deep read). The helper
// takes a name and cannot carry that, so pushing those through it would drop
// the attribution — which is why they are not this shape rather than why they
// are exempt from it.
func bindsSystemPrincipal(call *ast.CallExpr, qualifier string) bool {
	for _, arg := range call.Args {
		literal, isLiteral := arg.(*ast.CompositeLit)
		if !isLiteral || !selects(literal.Type, qualifier, "Principal") {
			continue
		}
		if principalFieldsOf(literal) == "ID,Type" && namesTheSystemType(literal, qualifier) {
			return true
		}
	}
	return false
}

// principalFieldsOf is the literal's field names in the order written, joined — so the
// plain shape is one string to compare and any further field makes it another.
func principalFieldsOf(literal *ast.CompositeLit) string {
	var names []string
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			return ""
		}
		key, isIdent := pair.Key.(*ast.Ident)
		if !isIdent {
			return ""
		}
		names = append(names, key.Name)
	}
	sort.Strings(names)
	// Sorted, then compared against the sorted plain shape: the two fields are
	// written in either order across this tree and the order says nothing.
	return strings.Join(names, ",")
}

// namesTheSystemType answers whether the literal's Type is PrincipalSystem.
func namesTheSystemType(literal *ast.CompositeLit, qualifier string) bool {
	for _, element := range literal.Elts {
		pair, isPair := element.(*ast.KeyValueExpr)
		if !isPair {
			continue
		}
		if key, isIdent := pair.Key.(*ast.Ident); isIdent && key.Name == "Type" {
			return selects(pair.Value, qualifier, "PrincipalSystem")
		}
	}
	return false
}

// mintsAFreshID answers whether the correlation id is minted here rather than
// carried in. A bus consumer re-binds the triggering event's, which is not this
// shape and must not be reported as one.
func mintsAFreshID(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	minted, isCall := call.Args[1].(*ast.CallExpr)
	return isCall && selects(minted.Fun, "ids", "NewV7")
}

// selects answers whether a node is `qualifier.name`, or a bare `name` when the
// package owns the identifier itself.
func selects(n ast.Node, qualifier, name string) bool {
	if qualifier == "" {
		ident, isIdent := n.(*ast.Ident)
		return isIdent && ident.Name == name
	}
	selector, isSelector := n.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != name {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	return isIdent && pkg.Name == qualifier
}
