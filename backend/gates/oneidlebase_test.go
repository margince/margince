// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The idle base — a record's newest activity when one is recorded, else the
// instant it was created — is spelled once, in shared/kernel/idlebase.
//
// It was spelled five times, across the stalled-deal rule, an account's
// pipeline read, a coverage read, the what's-slipping ranker and the
// gone-quiet project predicate. Two of them carried a COMMENT saying they
// matched the stalled rule, which is the shape this repository's rulebook
// forbids: a comment may not claim to be the only implementation unless a test
// holds it. Nothing held either, and the two had already been edited apart in
// the ORDER BY they sat in.
//
// The site list is DERIVED rather than written down. A hand-written one is
// what this gate exists because of — the sites are in five packages under
// three tiers, they do not share a noun, and each spelling hid the next from
// whoever went looking. gatekit.Scope sweeps the negative space instead: any
// file outside the owner that derives the base fails, and the owner must hold
// one or the gate is reading nothing.
//
// WHAT THIS GATE CAN AND CANNOT SEE. It recognises two shapes: the SQL
// coalesce over the two columns, in any string literal, and the Go fallback —
// a value defaulted from a creation instant and overridden inside a
// `lastActivityAt != nil` branch. It cannot see a third spelling that reaches
// the same answer some other way (a SQL CASE, a GREATEST, a fallback written
// through a helper of its own), so it is a net under the two known shapes
// rather than a proof of uniqueness.
//
// A THIRD ARGUMENT does not make it a different rule. created_at is NOT NULL,
// so nothing after it can ever fire — `coalesce(last_activity_at, created_at,
// now())` is this rule with a decoration on the end, and a census that required
// exactly two arguments could be evaded by one dead word.
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

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// coalesceCall finds where a coalesce begins. Only the CALL is matched by
// pattern; its arguments are read by scanning the balanced parentheses, because
// a pattern over the argument text is where this detector went blind twice.
// `%[1]slast_activity_at` has no word boundary in front of the column, and
// `coalesce((d.created_at), d.last_activity_at)` puts a parenthesis where a
// character class excluding parentheses cannot follow — and a reader comparing
// the two spellings by eye sees no difference at all.
var coalesceCall = regexp.MustCompile(`(?i)\bcoalesce\s*\(`)

// idleBaseColumns are the two columns whose fallback IS the rule.
var idleBaseColumns = []string{"last_activity_at", "created_at"}

// spellsIdleBaseSQL reports whether a string holds a two-argument coalesce
// naming BOTH columns — the idle base, or its reversed spelling.
//
// The reversed order matters as much as the correct one: created_at is NOT
// NULL, so `coalesce(created_at, last_activity_at)` never falls back and is a
// long way of writing created_at, while reading identically to a glance.
func spellsIdleBaseSQL(text string) bool {
	for _, at := range coalesceCall.FindAllStringIndex(text, -1) {
		args, ok := balancedArguments(text[at[1]:])
		if !ok || len(args) < len(idleBaseColumns) {
			continue
		}
		// The FIRST TWO arguments, and only those. A third arm is not a
		// different rule: created_at is NOT NULL, so nothing after it can ever
		// fire and `coalesce(last_activity_at, created_at, now())` is the idle
		// base with a decoration on the end. Requiring exactly two arguments
		// made one dead trailing argument a complete evasion. But the pair has
		// to come first — `coalesce(now(), last_activity_at, created_at)` asks
		// a different question, and its answer is usually now().
		named := map[string]bool{}
		for _, arg := range args[:len(idleBaseColumns)] {
			for _, column := range idleBaseColumns {
				if namesIdleColumn(arg, column) {
					named[column] = true
				}
			}
		}
		if len(named) == len(idleBaseColumns) {
			return true
		}
	}
	return false
}

// balancedArguments splits the argument list that starts just past an opening
// parenthesis, at top-level commas, and reports whether the list closed. Each
// argument is trimmed of space and of the parentheses a writer may wrap it in,
// so a qualifier — an alias, a format verb, a redundant bracket — cannot hide
// the column name behind it.
func balancedArguments(tail string) ([]string, bool) {
	var args []string
	depth, start := 0, 0
	for i, r := range tail {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return append(args, trimArgument(tail[start:i])), true
			}
			depth--
		case ',':
			if depth == 0 {
				args = append(args, trimArgument(tail[start:i]))
				start = i + 1
			}
		}
	}
	return nil, false
}

// trimArgument strips the space and the wrapping parentheses a writer may put
// around one argument.
func trimArgument(arg string) string {
	return strings.Trim(strings.TrimSpace(arg), "() \t\n\r")
}

// namesIdleColumn reports whether one coalesce argument reads the column.
//
// Both ends of the name have to be a boundary, and the two ends are asked
// differently. After it comes a cast, a space or nothing, and requiring that
// much keeps `last_activity_at_utc` from counting as this column. Before it
// comes whatever qualifies the column — `d.`, or a `%[1]s` format verb whose
// final letter is itself an identifier byte, which is why a plain
// word-boundary match read the parameterised spelling straight past.
func namesIdleColumn(arg, column string) bool {
	lower := strings.ToLower(arg)
	for at := strings.Index(lower, column); at >= 0; {
		after := at + len(column)
		if qualifiedBefore(lower[:at]) && (after == len(lower) || !isIdentifierByte(lower[after])) {
			return true
		}
		next := strings.Index(lower[after:], column)
		if next < 0 {
			return false
		}
		at = after + next
	}
	return false
}

// formatVerb is a format verb standing where a table alias would — `%s`,
// `%[1]s` — and it is the reason the leading boundary cannot simply be "not an
// identifier byte": the verb's final letter is one.
var formatVerb = regexp.MustCompile(`%(\[\d+\])?[a-z]$`)

// qualifiedBefore reports whether what precedes the match leaves the column a
// name of its own rather than the tail of a longer one. Without it
// `previous_created_at` — a different column, and a snapshot of the wrong
// instant — reads as the creation instant, and a coalesce over the two
// previous_ columns is claimed as a second copy of a rule it does not spell.
func qualifiedBefore(before string) bool {
	if before == "" || !isIdentifierByte(before[len(before)-1]) {
		return true
	}
	return formatVerb.MatchString(before)
}

// isIdentifierByte is what may CONTINUE an unquoted Postgres identifier.
//
// `$` is one of them, which is not obvious and matters here: without it
// `last_activity_at$old` and `previous$last_activity_at` are two different
// columns that this detector would read as the canonical pair, and a coalesce
// over them would be claimed as a second copy of a rule it does not spell.
// A `$` in a statement is far more often a placeholder, which is why it is easy
// to leave out of a boundary rule.
func isIdentifierByte(b byte) bool {
	return b == '_' || b == '$' || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// lastActivityName and creationName recognise the two columns in Go spelling —
// the field, the local, and the snake_case a scan target sometimes keeps.
var (
	lastActivityName = regexp.MustCompile(`(?i)^last_?activity_?at$`)
	creationName     = regexp.MustCompile(`(?i)^created_?at$`)
)

// idleBaseScope claims the derivation lives in shared/kernel/idlebase and
// nowhere else, and proves it by sweeping the negative space rather than by
// listing sites. Nothing is exempt: kernel is below every tier that asks the
// question, so every caller can reach idlebase.Since or idlebase.SQL.
var idleBaseScope = gatekit.Scope{
	Roots:   []string{"internal/shared/kernel/idlebase"},
	Subject: spellsTheIdleBase,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestTheIdleBaseIsSpelledOnce(t *testing.T) {
	t.Parallel()
	inside := idleBaseScope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("the idle base is spelled in %d files inside the package that owns it:\n\t%s\n\n"+
			"One derivation, in Go and in SQL, so a surface cannot agree with the stalled rule by "+
			"inspection and then drift. Call idlebase.Since or idlebase.SQL", len(inside),
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
			if node.Kind == token.STRING && spellsIdleBaseSQL(gatekit.TextOf(node)) {
				found = true
			}
		case *ast.BinaryExpr:
			// SQL assembled by concatenation, where no single literal holds
			// the whole expression. Joining the chain's literal parts is what
			// keeps `"coalesce(" + p + "last_activity_at, " + p +
			// "created_at)"` from being a respelling the census cannot see.
			if node.Op == token.ADD && spellsIdleBaseSQL(joinedLiterals(node)) {
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
// creation instant and overrides it with the activity, in either polarity:
//
//	base := createdAt; if lastActivityAt != nil { base = *lastActivityAt }
//	if lastActivityAt == nil { return createdAt }; return *lastActivityAt
//
// BOTH halves are required. The nil check alone is every ordinary read of the
// column — formatting it, deciding which column grounded an evidence line —
// and a gate that fired on those would be waived into uselessness within a
// week. It is the pairing with the other column that makes the code a second
// copy of the rule.
//
// Which arm carries which half follows the polarity: a `!= nil` branch holds
// the override and the default sits outside it, an `== nil` branch holds the
// default and the override sits outside. Checking only one polarity would
// leave the guard-clause spelling — the one a small function reaches for
// first — invisible.
func rederivesIdleBase(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		branch, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		subject, comparison, ok := nilComparison(branch.Cond)
		if !ok || !namesColumn(subject, lastActivityName) {
			return true
		}
		inBranch, outside := lastActivityName, creationName
		if comparison == token.EQL {
			inBranch, outside = creationName, lastActivityName
		}
		if writesValueNaming(branch.Body, inBranch) && writesValueOutside(body, branch, outside) {
			found = true
		}
		return !found
	})
	return found
}

// nilComparison reads `x != nil`, `x == nil` and both written the other way
// round, returning the compared expression and the operator. The yoda form is
// admitted because a detector that only knew one word order would be blind to
// a respelling the compiler treats as identical.
func nilComparison(cond ast.Expr) (subject ast.Expr, op token.Token, ok bool) {
	binary, isBinary := cond.(*ast.BinaryExpr)
	if !isBinary || (binary.Op != token.NEQ && binary.Op != token.EQL) {
		return nil, token.ILLEGAL, false
	}
	switch {
	case isNilIdent(binary.Y):
		return binary.X, binary.Op, true
	case isNilIdent(binary.X):
		return binary.Y, binary.Op, true
	}
	return nil, token.ILLEGAL, false
}

// writesValueOutside is writesValueNaming over everything in the function
// EXCEPT the arm the nil check matched.
//
// The two halves have to come from opposite sides of the check, or a function
// that nil-checks the activity and happens to read the creation instant in the
// SAME branch is read as a fallback it never wrote. That is the
// false-POSITIVE direction, and it is the one that gets a gate waived: a
// finding that is wrong on inspection teaches the next author to stop reading
// the findings.
//
// Only the matched arm is skipped, not the whole `if`. Skipping the statement
// entire also skipped its `else`, which is where the fallback lives in the
// most literal spelling of one — so the detector read a smaller tree and
// reported the clean word over it, which is the direction a census must not
// fail in.
func writesValueOutside(body *ast.BlockStmt, branch *ast.IfStmt, column *regexp.Regexp) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found || n == branch.Body {
			return false
		}
		switch n.(type) {
		case *ast.AssignStmt, *ast.ReturnStmt, *ast.ValueSpec:
			if writesValueNaming(n, column) {
				found = true
			}
		}
		return !found
	})
	return found
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

// joinedLiterals concatenates the string literals of a `+` chain, dropping the
// non-literal operands. What is dropped is a qualifier — an alias variable, a
// format verb — and the column names on either side of it are what the
// detector reads.
func joinedLiterals(expr ast.Expr) string {
	var parts []string
	ast.Inspect(expr, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			parts = append(parts, gatekit.TextOf(lit))
		}
		return true
	})
	return strings.Join(parts, "")
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// TestTheGateRecognisesEverySpellingOfTheIdleBase is the falsification half.
//
// A census passes by finding nothing, which is also what it does when the
// pattern stops matching, the walk reads the wrong tree, or a later edit
// loosens the detector into uselessness — and none of those has a failing
// assertion to notice. So it is asked each spelling the tree is known to reach
// for, verbatim, plus the near misses it must NOT claim: an ordinary nil check
// on the column is not a second copy of the rule, and neither is a coalesce
// onto something that is not the creation instant.
func TestTheGateRecognisesEverySpellingOfTheIdleBase(t *testing.T) {
	t.Parallel()
	spellings := map[string]string{
		"an ORDER BY over the base": "" +
			"func f() { q(`ORDER BY coalesce(d.last_activity_at, d.created_at), d.id`) }",
		"a projection selecting the base": "" +
			"func f() { q(`\n\t\tSELECT status, organization_id, coalesce(last_activity_at, created_at),\n" +
			"\t\t       last_activity_at IS NOT NULL\n\t\t  FROM deal WHERE id = $1`) }",
		"a row scan folding the base into a field": "" +
			"func f() {\n\tr.idleSince = createdAt\n\tif lastActivityAt != nil {\n\t\tr.idleSince = *lastActivityAt\n\t}\n}",
		"a function returning the base as a pointer": "" +
			"func idleSince(d SlippingDeal) *time.Time {\n\tif d.LastActivityAt != nil {\n\t\treturn d.LastActivityAt\n\t}\n" +
			"\tif !d.CreatedAt.IsZero() {\n\t\tcreated := d.CreatedAt\n\t\treturn &created\n\t}\n\treturn nil\n}",
		"an idle threshold measured from a hand-rolled base": "" +
			"func f() {\n\tbase := createdAt\n\tif lastActivityAt != nil {\n\t\tbase = *lastActivityAt\n\t}\n\t_ = base\n}",
		"the guard-clause spelling, where the nil check is the other way round": "" +
			"func f() time.Time {\n\tif d.LastActivityAt == nil {\n\t\treturn d.CreatedAt\n\t}\n\treturn *d.LastActivityAt\n}",
		"the same guard written with nil on the left": "" +
			"func f() time.Time {\n\tif nil == d.LastActivityAt {\n\t\treturn d.CreatedAt\n\t}\n\treturn *d.LastActivityAt\n}",
		"SQL assembled by concatenation, held by no single literal": "" +
			"func f() { q(`coalesce(` + p + `last_activity_at, ` + p + `created_at)`) }",
		"the reversed order, which never falls back at all": "" +
			"func f() { q(`SELECT coalesce(created_at, last_activity_at) FROM deal`) }",
		"a trailing arm that can never fire, which decorates the rule rather than changing it": "" +
			"func f() { q(`coalesce(last_activity_at, created_at, now())`) }",
		"the explicit else form, where the fallback lives in the branch's other arm": "" +
			"func f() time.Time {\n\tif d.LastActivityAt != nil {\n\t\treturn *d.LastActivityAt\n" +
			"\t} else {\n\t\treturn d.CreatedAt\n\t}\n}",
		"the same else, assigning rather than returning": "" +
			"func f() {\n\tvar base time.Time\n\tif d.LastActivityAt != nil {\n\t\tbase = *d.LastActivityAt\n" +
			"\t} else {\n\t\tbase = d.CreatedAt\n\t}\n\t_ = base\n}",
		"an alias passed as a format verb rather than written out": "" +
			"func f() { q(fmt.Sprintf(`coalesce(%[1]slast_activity_at, %[1]screated_at)`, p)) }",
		"an argument a writer wrapped in redundant parentheses": "" +
			"func f() { q(`coalesce((d.created_at), d.last_activity_at)`) }",
		"an argument carrying a cast, which puts a parenthesis in the way": "" +
			"func f() { q(`coalesce(d.last_activity_at::timestamptz, (d.created_at)::timestamptz)`) }",
	}
	for name, body := range spellings {
		if !spellsTheIdleBase("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the detector no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"an ordinary nil check that never falls back to the creation instant": "" +
			"func f() {\n\tif d.LastActivityAt != nil {\n\t\tout.Touched = *d.LastActivityAt\n\t}\n}",
		"a creation read beside a nil check that writes no value": "" +
			"func f() {\n\tbase := d.CreatedAt\n\tif d.LastActivityAt != nil {\n\t\tsource = \"activity\"\n\t}\n\t_ = base\n}",
		"both reads inside the SAME branch, which is not a fallback": "" +
			"func f() {\n\tif d.LastActivityAt != nil {\n\t\ttouched = *d.LastActivityAt\n\t\tage = now.Sub(d.CreatedAt)\n\t}\n}",
		"a coalesce onto something that is not the creation instant": "" +
			"func f() { q(`coalesce(last_activity_at, now())`) }",
		"a coalesce naming one column twice, which pairs with nothing": "" +
			"func f() { q(`coalesce(a.created_at, b.created_at)`) }",
		"a coalesce whose first arm is something else entirely": "" +
			"func f() { q(`coalesce(now(), last_activity_at, created_at)`) }",
		"a longer column that merely starts the same way": "" +
			"func f() { q(`coalesce(last_activity_at_utc, created_at_utc)`) }",
		"a prefixed column that merely ends the same way": "" +
			"func f() { q(`coalesce(previous_last_activity_at, previous_created_at)`) }",
		"columns suffixed with a dollar, which Postgres allows inside a name": "" +
			"func f() { q(`coalesce(last_activity_at$old, created_at$old)`) }",
		"and prefixed with one": "" +
			"func f() { q(`coalesce(previous$last_activity_at, previous$created_at)`) }",
	}
	for name, body := range nearMisses {
		if spellsTheIdleBase("x.go", parseGateFixture(t, "package p\n"+body)) {
			t.Errorf("the detector claims %s is a second spelling of the idle base; it will be waived into "+
				"uselessness if it fires on every read of the column", name)
		}
	}
}
