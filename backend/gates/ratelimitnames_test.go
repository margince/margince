// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A rate limiter's NAME is its bucket. Every limiter counts into a store the
// replicas of a role share, keyed by `ratelimit:<name>:<key>`, so two limiters
// that name the same thing are not two ceilings of the configured size — they
// are one ceiling, spent by both, and the login budget an installation thinks
// it has may be being spent by a webhook's refusals.
//
// Nothing about that failure is visible. Both limiters still admit and refuse;
// the numbers are just wrong, in the direction that locks real callers out
// early and in the direction that lets an attacker spend somebody else's
// allowance. It cannot be caught in a unit test either, because a test binary
// binds no shared store and each limiter counts alone.
//
// So the invariant is the one thing a reader cannot check by reading one file:
// across the whole tree, no two construction sites claim a name that can
// collide. A site claims either an exact name (a literal, or a constant two
// sites deliberately share) or a PREFIX, when the name is built per subject —
// `"extension-inbound/"+unit+"/"+slug` is one claim over everything beneath
// it. Equal claims collide, and so does a claim that lies underneath another.
//
// WHAT IT CANNOT SEE. A name assembled somewhere other than the call — passed
// in as a parameter, or read from config — would arrive here unreadable, and
// the gate reports that rather than passing over it, because a census that
// cannot judge its subject and says nothing has already failed.

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// rateLimiterNameScope claims every limiter a deployment runs is constructed
// under internal/. Tests are outside the sweep by construction (gatekit sweeps
// what ships), which is right here rather than merely convenient: a test
// binary never binds a shared store, so a name it reuses costs nothing.
var rateLimiterNameScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: buildsARateLimiter,
	Exempt:  gatekit.Waive(map[string]string{}),
}

// nameClaim is one construction site's hold on the shared key space.
type nameClaim struct {
	claim  string // the exact name, or the literal prefix of a computed one
	exact  bool   // false when the site builds a name beneath this prefix
	spelt  string // how the site wrote it, so two uses of one constant are one claim
	source string // the file it was written in, so a finding names both halves
}

func TestNoTwoRateLimitersClaimOneName(t *testing.T) {
	t.Parallel()
	var claims []nameClaim
	for _, f := range rateLimiterNameScope.Files(t) {
		claims = append(claims, rateLimiterNamesIn(t, f)...)
	}
	if len(claims) == 0 {
		t.Fatal("no rate limiter construction was found under internal/ — this census reads a smaller tree than it thinks it does, and an empty one reports PASS")
	}
	sort.Slice(claims, func(i, j int) bool { return claims[i].claim < claims[j].claim })

	for i, held := range claims {
		for _, other := range claims[i+1:] {
			if !collides(held, other) {
				continue
			}
			t.Errorf("two rate limiters claim one name in the store their replicas share:\n\t%s\t%s\n\t%s\t%s\n\n"+
				"They are one ceiling, spent by both, and nothing about that is visible at either site. Name what each one bounds.",
				held.source, held.describe(), other.source, other.describe())
		}
	}
}

func (c nameClaim) describe() string {
	if c.exact {
		return fmt.Sprintf("%q", c.claim)
	}
	return fmt.Sprintf("%q and everything beneath it", c.claim)
}

// collides reports whether two claims can produce one name. Equal claims
// always can; a prefix claim swallows anything written beneath it. Two sites
// spelling one shared constant are the SAME claim on purpose — the OIDC edge
// has two constructors for one ceiling — so they are judged by what they wrote
// rather than by what it resolves to.
func collides(a, b nameClaim) bool {
	if a.spelt != "" && a.spelt == b.spelt {
		return false
	}
	if a.exact && b.exact {
		return a.claim == b.claim
	}
	return strings.HasPrefix(a.claim, b.claim) || strings.HasPrefix(b.claim, a.claim)
}

func buildsARateLimiter(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if isRateLimiterConstruction(n) {
			found = true
		}
		return !found
	})
	return found
}

// isRateLimiterConstruction reports whether the node is a call to one of the
// two constructors. Both are judged: NewWithClock differs only in where the
// clock comes from, and a limiter built through it claims a name exactly as
// hard as one built through New.
func isRateLimiterConstruction(n ast.Node) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "ratelimit" &&
		(selector.Sel.Name == "New" || selector.Sel.Name == "NewWithClock")
}

func rateLimiterNamesIn(t *testing.T, f gatekit.ParsedFile) []nameClaim {
	t.Helper()
	// Resolved lazily: most swept files hold no constant-spelt name, and
	// parsing a package's constants for every one of them is work the census
	// does not need to do.
	var constants map[string]string
	var out []nameClaim
	ast.Inspect(f.File, func(n ast.Node) bool {
		if !isRateLimiterConstruction(n) {
			return true
		}
		call := n.(*ast.CallExpr)
		if len(call.Args) == 0 {
			t.Errorf("%s builds a rate limiter with no arguments; this census cannot tell what it claims", f.Path)
			return true
		}
		if constants == nil {
			constants = gatekit.PackageStringConstants(t, filepath.Dir(f.Path))
		}
		claim, ok := claimOf(call.Args[0], constants)
		if !ok {
			t.Errorf("%s builds a rate limiter whose name this census cannot read. A name it cannot read is a "+
				"bucket it cannot prove distinct, and saying nothing about it reads exactly like agreement. "+
				"Write the name at the call, as a literal or a package constant.", f.Path)
			return true
		}
		claim.source = f.Path
		out = append(out, claim)
		return true
	})
	return out
}

// claimOf reduces a name expression to what it holds in the key space: a
// literal or a resolved constant is an exact name, and a concatenation is the
// prefix its leading literal claims over everything the rest can produce.
func claimOf(expr ast.Expr, constants map[string]string) (nameClaim, bool) {
	if text, ok := gatekit.LiteralText(expr); ok {
		return nameClaim{claim: text, exact: true}, true
	}
	if ident, ok := expr.(*ast.Ident); ok {
		text, known := constants[ident.Name]
		if !known {
			return nameClaim{}, false
		}
		return nameClaim{claim: text, exact: true, spelt: ident.Name}, true
	}
	sum, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return nameClaim{}, false
	}
	prefix, ok := leadingLiteral(sum)
	if !ok || prefix == "" {
		return nameClaim{}, false
	}
	return nameClaim{claim: prefix}, true
}

// leadingLiteral walks to the left of a concatenation, where Go's left
// associativity puts the part that is fixed at the call.
func leadingLiteral(expr ast.Expr) (string, bool) {
	for {
		sum, ok := expr.(*ast.BinaryExpr)
		if !ok {
			return gatekit.LiteralText(expr)
		}
		expr = sum.X
	}
}
