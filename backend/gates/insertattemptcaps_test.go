// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// An insert that names no MaxAttempts does not run without a retry ladder — it
// runs on River's default of 25, on attempt-to-the-fourth backoff, which
// reaches days. Nobody chooses that ladder; a site simply omits the field and
// gets it, which is how vcard_ingest shipped riding it for hours on a
// deterministic failure.
//
// The contract cannot close this. api/jobs.yaml publishes max_attempts for the
// two owners that can APPLY one — a fan-out child, whose helper reads it off
// the spec, and an args-owned kind, held equal to its InsertOpts by
// TestArgsOwnedAttemptCapsMatchTheirDeclaration — and refuses it from a
// caller-owned kind, because a published number nothing applies is the
// declared-versus-actual drift that file exists to remove. So a caller-owned
// kind's cap lives in Go, at the insert, and this is what keeps one from being
// forgotten there.
//
// The corpus is every river.InsertOpts literal rather than the caller-owned
// kind list, for two reasons: a kind is inserted from more than one site and
// each site's opts are its own, and several sites build the literal inline
// rather than in a *InsertOpts() constructor a kind-keyed walk could find.

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// insertSite is a literal's address as this gate reports and waives it: the
// file, the function it is built in, and — for the second and later literal in
// one function — which one.
//
// Not a line number, which moves under an edit that changes nothing about the
// obligation. But not the function alone either: a waiver keyed on the function
// is inherited by every literal added to it later, and Waived records a match
// without refusing a repeat, so a second uncapped literal inside a ratified
// helper would be exempted by a reason written about its neighbour. An ordinal
// is stable under every edit except adding a literal, which is exactly the edit
// that must not inherit.
type insertSite string

// capExemptSites ratifies the literals that must NOT name an attempt cap. Both
// are helpers that hand the number's ownership somewhere else, and in both the
// omission is load-bearing rather than forgotten.
var capExemptSites = gatekit.Waive(map[insertSite]string{
	"internal/compose/jobs.go:sweepInsertOpts":       "the uniqueness half of the periodic insert and nothing else. River reads the explicit opts BEFORE the args type's, so a cap here would outrank every max_attempts api/jobs.yaml publishes for an opts_owner: args kind and make the declared ladder a number nothing runs at; periodicInsertOpts adds one only where no args type owns it",
	"internal/compose/dispatch.go:markedAsFleetPass": "it copies the caller's opts to add the sweep tag, so the cap it carries is whichever one the caller already chose. Naming one here would silently retune every fleet pass, and do it inside a function whose subject is a tag",
})

// TestEveryInsertDeclaresAnAttemptCap is the census the caller-owned tier had
// no equivalent of.
func TestEveryInsertDeclaresAnAttemptCap(t *testing.T) {
	t.Parallel()
	defer capExemptSites.AssertAllMatched(t)
	scope := gatekit.Scope{
		Roots:   []string{"internal/compose"},
		Subject: func(_ string, file *ast.File) bool { return len(insertOptsLiterals(file)) > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	seen, bounded := 0, 0
	for _, src := range scope.Files(t) {
		consts := attemptCapConstants(src.File)
		for _, lit := range insertOptsLiterals(src.File) {
			seen++
			site := insertSite(fmt.Sprintf("%s:%s", src.Path, lit.fn))
			if lit.nth > 1 {
				site = insertSite(fmt.Sprintf("%s:%s#%d", src.Path, lit.fn, lit.nth))
			}
			capExpr, named := lit.attemptCap()
			if !named {
				if !capExemptSites.Waived(t, site) {
					t.Errorf("%s builds a river.InsertOpts naming no MaxAttempts, so its rows ride River's "+
						"default ladder of %d on attempt⁴ backoff — days of retries nobody chose. Name one of "+
						"the caps in internal/compose/jobattempts.go, read the number off the declaration, or "+
						"ratify the site in capExemptSites with the reason the omission is load-bearing",
						site, jobs.DefaultMaxAttempts)
				}
				continue
			}
			bounded++
			if capExemptSites.Waived(t, site) {
				t.Errorf("%s names a MaxAttempts and is also ratified in capExemptSites as a site that must "+
					"not: drop the waiver, which now describes code that is gone", site)
			}
			if namesTheDefaultLadder(capExpr) {
				t.Errorf("%s names River's own default as its MaxAttempts, which is not a cap but the ladder "+
					"this gate exists to take rows off: %d attempts on attempt⁴ backoff, reaching days. A "+
					"kind that genuinely runs on the default says so in api/jobs.yaml, where the number "+
					"carries its reason, and the insert reads it off the declaration",
					site, jobs.DefaultMaxAttempts)
				continue
			}
			value, resolved := attemptCapValue(capExpr, consts)
			if !resolved {
				// The number came off the declaration or off a runtime value —
				// spec.MaxAttempts, decl.MaxAttempts — which is a BETTER source
				// than a Go constant, not a worse one: api/jobs.yaml carries
				// the number and the reason for it, and this walk deliberately
				// does not re-derive what that file already holds.
				continue
			}
			if value <= 0 || value >= jobs.DefaultMaxAttempts {
				t.Errorf("%s inserts with MaxAttempts %d: a cap must be positive and below River's own "+
					"default of %d, or it is not a choice — zero and above-default both leave the row on "+
					"the ladder this gate exists to take it off",
					site, value, jobs.DefaultMaxAttempts)
			}
		}
	}
	// Rule 8: a census that reads a smaller tree than it thinks reports PASS,
	// and there is no failing assertion to notice. These floors are what such a
	// walk trips over — one for the corpus, one for each disposition, so a
	// spelling change that hides the literals, the caps, or the waived helpers
	// fails here rather than certifying an empty sweep.
	const fewestLiterals = 30
	if seen < fewestLiterals {
		t.Errorf("the walk found %d river.InsertOpts literals under internal/compose, fewer than the %d that "+
			"were there when this gate was written: a census that reads less than it thinks reports PASS, so "+
			"either the inserts moved to a spelling this walk cannot see or the floor is stale",
			seen, fewestLiterals)
	}
	if bounded == 0 || bounded == seen {
		t.Errorf("of %d literals %d name a cap: this gate distinguishes the two dispositions, and finding "+
			"only one of them means the walk is reading the field's presence wrong rather than that the tree "+
			"is uniform", seen, bounded)
	}
}

// insertOptsLiteral is one river.InsertOpts composite literal and the function
// it is built in.
type insertOptsLiteral struct {
	fn string
	// nth is which literal this is inside fn, counting from one. It is what
	// keeps a waiver from spreading to a literal added beside the one it was
	// written about.
	nth int
	lit *ast.CompositeLit
}

// attemptCap returns the expression the literal sets MaxAttempts to, and
// whether it sets it at all.
func (l insertOptsLiteral) attemptCap() (ast.Expr, bool) {
	for _, elt := range l.lit.Elts {
		kv, keyed := elt.(*ast.KeyValueExpr)
		if !keyed {
			continue
		}
		if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "MaxAttempts" {
			return kv.Value, true
		}
	}
	return nil, false
}

// insertOptsLiterals finds every river.InsertOpts composite literal in the
// file, each tagged with the function it is built in.
//
// Both spellings of the type are matched. A composite literal written
// `river.InsertOpts{…}` reads as a SelectorExpr, and `&river.InsertOpts{…}`
// wraps the same node in a unary — the walk sees the literal either way,
// because ast.Inspect descends into the unary.
func insertOptsLiterals(file *ast.File) []insertOptsLiteral {
	var found []insertOptsLiteral
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}
		nth := 0
		ast.Inspect(fn, func(node ast.Node) bool {
			lit, isLit := node.(*ast.CompositeLit)
			if !isLit {
				return true
			}
			if sel, isSel := lit.Type.(*ast.SelectorExpr); isSel && sel.Sel.Name == "InsertOpts" {
				if pkg, isIdent := sel.X.(*ast.Ident); isIdent && pkg.Name == "river" {
					nth++
					found = append(found, insertOptsLiteral{fn: fn.Name.Name, nth: nth, lit: lit})
				}
			}
			return true
		})
	}
	return found
}

// attemptCapConstants collects the file's package-level integer constants, so a cap
// written as the named constant it should be can still be read as a number.
//
// One file's constants, not the package's: a cap and the insert that applies it
// belong beside each other, and the caps this tree shares live in
// jobattempts.go, whose own literals are read from the same file.
func attemptCapConstants(file *ast.File) map[string]int {
	values := map[string]int{}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if literal, ok := untypedIntLiteral(value.Values[i]); ok {
					values[name.Name] = literal
				}
			}
		}
	}
	return values
}

// attemptCapValue resolves a cap expression to the number it will insert with,
// reporting false for one this walk cannot read without a type checker — a
// field read off the declaration, or arithmetic.
func attemptCapValue(expr ast.Expr, consts map[string]int) (int, bool) {
	if literal, ok := untypedIntLiteral(expr); ok {
		return literal, true
	}
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		value, known := consts[ident.Name]
		return value, known
	}
	return 0, false
}

// untypedIntLiteral reads an untyped integer literal.
func untypedIntLiteral(expr ast.Expr) (int, bool) {
	basic, isBasic := expr.(*ast.BasicLit)
	if !isBasic || basic.Kind != token.INT {
		return 0, false
	}
	value, err := strconv.Atoi(basic.Value)
	if err != nil {
		return 0, false
	}
	return value, true
}

// namesTheDefaultLadder reports whether a cap expression IS River's default,
// however it is spelled — jobs.DefaultMaxAttempts, or the bare name inside the
// package that mirrors it.
//
// Named separately from attemptCapValue because it is a different refusal.
// attemptCapValue admits an expression it cannot evaluate, on the ground that
// the number came off api/jobs.yaml, where it carries its reason — and a
// declared cap may legitimately BE 25, as capture_sync's is. What may not
// happen is a Go insert site naming the default itself: that is a site
// declining to choose while satisfying a gate that asks it to.
func namesTheDefaultLadder(expr ast.Expr) bool {
	const defaultName = "DefaultMaxAttempts"
	switch named := expr.(type) {
	case *ast.SelectorExpr:
		return named.Sel.Name == defaultName
	case *ast.Ident:
		return named.Name == defaultName
	}
	return false
}
