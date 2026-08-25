// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// A deal's idle base — its newest activity when one is recorded, else the day
// it was written down — is spelled once, in the deals module that owns the
// table.
//
// It was spelled five times. Two of the four copies outside the owner carried
// a COMMENT saying they matched the rule ("the same base IsStalled measures
// from"; "the same idle base IsStalled uses"), which is exactly the shape this
// repo's rulebook forbids: a comment may not claim to be the only
// implementation unless a test holds it. Nothing held either claim, and the
// census filed against them was wrong twice in three days — once counting the
// owner's own two halves as copies, once naming a file the rule had already
// moved out of. That is the argument for DERIVING the site list instead of
// writing it down, which is what gatekit.Scope does below.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It recognises two shapes: the SQL
// coalesce over the two columns, in any string literal, and the Go fallback —
// a value defaulted from a creation instant and overridden inside a
// `lastActivityAt != nil` branch. It cannot see a third spelling that reaches
// the same answer some other way (a SQL CASE, a `GREATEST`, a fallback written
// through a helper of its own), so it is a net under the two known shapes
// rather than a proof of uniqueness.
//
// The REVERSED coalesce is a subject too, deliberately. `coalesce(created_at,
// last_activity_at)` reads to the naked eye like the same clause and is not:
// created_at is NOT NULL, so the fallback never fires and the expression is a
// long way of writing created_at. A gate that only knew the correct order
// would stay green over the one respelling that is already a bug.

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// idleBaseSQL finds a two-argument coalesce over either column, across the
// line breaks a formatted query puts inside it. The qualifier is anything
// attached to the column name rather than an identifier followed by a dot: a
// caller that parameterises the alias writes `%[1]slast_activity_at`, and a
// pattern that only knew `d.` would read those straight past.
//
// It matches either column in either position, so it also finds
// `coalesce(created_at, created_at)`-shaped nonsense; spellsIdleBaseSQL below
// keeps only the matches naming BOTH columns. RE2 has no backreference to say
// "and the second is not the first", which is why the pairing is checked in Go
// rather than smuggled into the pattern.
var idleBaseSQL = regexp.MustCompile(
	`(?is)coalesce\(\s*[^,()\s]*(?:last_activity_at|created_at)\s*,\s*[^,()\s]*(?:last_activity_at|created_at)\s*\)`)

// spellsIdleBaseSQL reports whether a string holds a coalesce over BOTH
// columns — the idle base, or its reversed spelling.
func spellsIdleBaseSQL(text string) bool {
	for _, match := range idleBaseSQL.FindAllString(text, -1) {
		lower := strings.ToLower(match)
		if strings.Contains(lower, "last_activity_at") && strings.Contains(lower, "created_at") {
			return true
		}
	}
	return false
}

// lastActivityName and creationName recognise the two columns in Go spelling —
// the field, the local, and the snake_case a scan target sometimes keeps.
var (
	lastActivityName = regexp.MustCompile(`(?i)^last_?activity_?at$`)
	creationName     = regexp.MustCompile(`(?i)^created_?at$`)
)

// idleBaseScope claims the derivation lives in the deals module and nowhere
// else, and proves it by sweeping the negative space rather than by listing
// the sites the rule used to have. Nothing is exempt: every caller outside the
// owner can reach deals.IdleSince or deals.IdleSinceSQL, because compose may
// import a module and a module gets the value handed across its seam.
var idleBaseScope = gatekit.Scope{
	Roots:   []string{"internal/shared/kernel/idlebase"},
	Subject: spellsTheIdleBase,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestTheIdleBaseIsSpelledOnce(t *testing.T) {
	inside := idleBaseScope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("the idle base is spelled in %d files inside the module that owns it:\n\t%s\n\n"+
			"One derivation, in Go and in SQL, so a surface cannot agree with the stalled rule by "+
			"inspection and then drift. Call deals.IdleSince or deals.IdleSinceSQL", len(inside),
			strings.Join(where, "\n\t"))
	}
}

// spellsTheIdleBase reports whether a file derives the idle base itself, in
// either the SQL or the Go shape.
func spellsTheIdleBase(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind == token.STRING && spellsIdleBaseSQL(node.Value) {
				found = true
			}
		case *ast.FuncDecl:
			if node.Body != nil && rederivesIdleBase(node.Body) {
				found = true
			}
		case *ast.FuncLit:
			if rederivesIdleBase(node.Body) {
				found = true
			}
		}
		return !found
	})
	return found
}

// rederivesIdleBase reports whether a function body defaults a value from the
// creation instant and overrides it inside a `lastActivityAt != nil` branch.
//
// BOTH halves are required. The override alone is every ordinary nil check on
// the column — reading it, formatting it, deciding which column grounded an
// evidence line — and a gate that fired on those would be waived into
// uselessness within a week. It is the pairing with the creation fallback that
// makes the code a second copy of the rule.
func rederivesIdleBase(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	overrides := false
	ast.Inspect(body, func(n ast.Node) bool {
		branch, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		cond, ok := branch.Cond.(*ast.BinaryExpr)
		if !ok || cond.Op != token.NEQ || !isNilIdent(cond.Y) || !namesColumn(cond.X, lastActivityName) {
			return true
		}
		if writesValueNaming(branch.Body, lastActivityName) {
			overrides = true
		}
		return !overrides
	})
	return overrides && writesValueNaming(body, creationName)
}

// writesValueNaming reports whether some assignment, declaration or return in
// the node takes its value from an expression naming the column.
func writesValueNaming(node ast.Node, column *regexp.Regexp) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		var values []ast.Expr
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			values = stmt.Rhs
		case *ast.ReturnStmt:
			values = stmt.Results
		case *ast.ValueSpec:
			values = stmt.Values
		default:
			return true
		}
		for _, v := range values {
			if namesColumn(v, column) {
				found = true
			}
		}
		return !found
	})
	return found
}

// namesColumn reports whether any identifier reachable from the expression is
// one of the two columns — the field of a selector, a local, or a bare name.
func namesColumn(expr ast.Expr, column *regexp.Regexp) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && column.MatchString(ident.Name) {
			found = true
		}
		return !found
	})
	return found
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// TestTheGateStillRecognisesEverySpellingItRemoved is the falsification half.
//
// A census passes by finding nothing, which is also what it does when the
// pattern stops matching, the walk reads the wrong tree, or a later edit
// loosens the detector into uselessness — and none of those has a failing
// assertion to notice. So it is asked the four spellings this change deleted,
// verbatim, plus the near misses it must NOT claim: an ordinary nil check on
// the column is not a second copy of the rule, and neither is a coalesce onto
// something that is not the creation instant.
func TestTheGateStillRecognisesEverySpellingItRemoved(t *testing.T) {
	removed := map[string]string{
		"org360/pipelineread.go's ORDER BY": "" +
			"func f() { q(`ORDER BY coalesce(d.last_activity_at, d.created_at), d.id`) }",
		"network/coveragefacts.go's projection": "" +
			"func f() { q(`\n\t\tSELECT status, organization_id, coalesce(last_activity_at, created_at),\n" +
			"\t\t       last_activity_at IS NOT NULL\n\t\t  FROM deal WHERE id = $1`) }",
		"org360/pipelineread.go's row scan": "" +
			"func f() {\n\tr.idleSince = createdAt\n\tif lastActivityAt != nil {\n\t\tr.idleSince = *lastActivityAt\n\t}\n}",
		"agents/tools_slipping.go's idleSince": "" +
			"func idleSince(d SlippingDeal) *time.Time {\n\tif d.LastActivityAt != nil {\n\t\treturn d.LastActivityAt\n\t}\n" +
			"\tif !d.CreatedAt.IsZero() {\n\t\tcreated := d.CreatedAt\n\t\treturn &created\n\t}\n\treturn nil\n}",
		"deals/formulas.go's IsStalled, before it delegated": "" +
			"func f() {\n\tbase := createdAt\n\tif lastActivityAt != nil {\n\t\tbase = *lastActivityAt\n\t}\n\t_ = base\n}",
		"the reversed order, which never falls back at all": "" +
			"func f() { q(`SELECT coalesce(created_at, last_activity_at) FROM deal`) }",
		"an alias passed as a format verb rather than written out": "" +
			"func f() { q(fmt.Sprintf(`coalesce(%[1]slast_activity_at, %[1]screated_at)`, p)) }",
	}
	for name, body := range removed {
		if !spellsTheIdleBase("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the detector no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"an ordinary nil check that never falls back to the creation instant": "" +
			"func f() {\n\tif d.LastActivityAt != nil {\n\t\tout.Touched = *d.LastActivityAt\n\t}\n}",
		"a creation read beside a nil check that writes no value": "" +
			"func f() {\n\tbase := d.CreatedAt\n\tif d.LastActivityAt != nil {\n\t\tsource = \"activity\"\n\t}\n\t_ = base\n}",
		"a coalesce onto something that is not the creation instant": "" +
			"func f() { q(`coalesce(last_activity_at, now())`) }",
		"a coalesce naming one column twice, which pairs with nothing": "" +
			"func f() { q(`coalesce(a.created_at, b.created_at)`) }",
	}
	for name, body := range nearMisses {
		if spellsTheIdleBase("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the detector claims %s is a second spelling of the idle base; it will be waived into "+
				"uselessness if it fires on every read of the column", name)
		}
	}
}
