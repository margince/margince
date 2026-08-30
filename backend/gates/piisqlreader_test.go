// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Reading the SQL the PII census judges: where one statement ends and the next
// begins, which table a write lands on, which columns its SET clause names, and
// which of those the reader could not read at all.
//
// Its own file because piicoverage_test.go had crossed the size cap, and
// because this half is a different subject from the one next door. That file
// declares WHAT must be true of every PII table — who erases it, who exports
// it, what a sweep may destroy. This one is how a statement is read well enough
// to answer, and every function here exists because a naive reading of it was
// wrong: a semicolon inside a value, a clause word inside a comment, a column
// assembled at runtime, an assignment after a subquery.
//
// The cases that hold it are retainedcolumncases_test.go, which plants each of
// those shapes rather than trusting that the tree passes.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// sqlWhitespaceRe collapses the indentation of a raw-string SQL literal so an
// assignment can be matched as the one-line clause it reads as in the registry.
var sqlWhitespaceRe = regexp.MustCompile(`\s+`)

func collapsedSQL(literal string) string {
	return strings.ToLower(strings.TrimSpace(sqlWhitespaceRe.ReplaceAllString(withoutComments(literal), " ")))
}

// withoutComments replaces each comment run with a space, keeping everything
// else — quoted values included — exactly as written.
//
// Before the whitespace collapse, and that ORDER is the whole of it. A line
// comment ends at its newline; collapsing newlines first turns `SET body =
// NULL, -- note\n counterparty_email = NULL` into one line where the comment
// runs to the end of the statement, so every assignment after it disappears and
// the check reports a clean sweep. The comment is not part of the statement, so
// it comes out rather than being scanned around later.
func withoutComments(literal string) string {
	var out strings.Builder
	for i := 0; i < len(literal); i++ {
		skip, _ := gatekit.SQLSpanAt(literal, i)
		if skip == 0 {
			out.WriteByte(literal[i])
			continue
		}
		if literal[i] == '-' || literal[i] == '/' {
			out.WriteByte(' ')
		} else {
			out.WriteString(literal[i : i+skip])
		}
		i += skip - 1
	}
	return out.String()
}

// sqlLiterals returns every Go string literal in one source file. Both the
// write-target and read-target scans run over these.
func sqlLiterals(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil {
			out = append(out, s)
		}
		return true
	})
	return out
}

// erasureCascadeFiles are the sources that make up the Art. 17 cascade — the
// files ErasePerson's own transaction executes SQL from. It is a LIST because
// the cascade spans more than one file, and a gate pinned to a single path
// silently stops covering a table the moment its scrub is extracted to a
// neighbour. It is deliberately NOT the whole privacy
// package: retention.go also writes subject tables, and letting a retention
// sweep satisfy "Art. 17 reaches this table" is exactly the confusion this test
// exists to prevent.
var erasureCascadeFiles = []string{
	"internal/modules/privacy/erasure.go",
	// The subject's TIMELINE and everything derived from it — split out of
	// erasure.go when that file crossed the size cap. It is the same Art. 17
	// transaction, so it counts here; leaving it off would let a table look
	// uncovered the moment its purge moved file.
	"internal/modules/privacy/erasuretimeline.go",
	// The subject's traces in the relationship graph — the interaction
	// participants, the imported LinkedIn ghosts, and the projection folded out
	// of both. Same Art. 17 transaction, its own file for the same size reason
	// the timeline has one.
	"internal/modules/privacy/erasure_graph.go",
	// Retention’s graph invalidation — same Art. 17/retention transaction.
	"internal/modules/privacy/erasure_attachments.go",
	"internal/modules/privacy/erasure_channels.go",
	// The live capabilities over the subject's consent record — the
	// preference-center token and the double-opt-in token. Split out of
	// erasure.go for the same size reason as the timeline, and named here for
	// the reason this list's own header gives: leaving it off would make
	// preference_token look uncovered the moment its DELETE moved file.
	"internal/modules/privacy/erasure_consent.go",
	"internal/modules/privacy/erasure_rivals.go",
	// What a licensed data provider was PAID to tell us about the subject,
	// and the runs that bought it (ADR-0101). Same Art. 17 transaction, its
	// own file for the same size reason the timeline has one.
	"internal/modules/privacy/erasure_provider.go",
	// The messages nobody has decided yet, and the quotations they carry.
	// Same Art. 17 transaction; its own file because it belongs to neither
	// destructive engine and both reach it.
	"internal/modules/privacy/erasure_approvals.go",
	// The readings of the transcripts the timeline scrub just emptied — same
	// transaction, its own file for the same both-engines reason.
	"internal/modules/privacy/transcriptreadings.go",
	"internal/modules/privacy/deliveries.go",
}

// retentionSweepFiles are the nightly time-based evaluator — the only eraser a
// subject-unlinked PII table has. Kept apart from the cascade above so a
// retention sweep can never be mistaken for an answer to an Art. 17 request.
//
// A LIST rather than one path, because the evaluator has already outgrown one
// file twice. Splitting the AI-store sweeps out made three tables look
// unswept; then the per-action executors moved to retentionactions.go and the
// list was not extended, so every assignment the sweep's OWN actions make —
// activity/erase among them — was invisible to this gate. A census keyed to a
// filename reports a refactor as a compliance regression, and worse reports
// nothing at all when the refactor moves code OUT of the names it knows.
//
// It is still a list and not a glob over retention*.go, and that is the part
// worth reading before "fixing" it. retentionrestricted.go is the RESTRICTION
// LIFT — a different trigger — and it clears `counterparty_email`, which the
// sweep deliberately keeps. Folding it in would let the lift's assignments
// satisfy the sweep's declarations below, so a retentionErasures entry would
// pass whether or not the sweep still made it: the gate would go quietly green
// over exactly the divergence it exists to catch. Over-recognition is the
// failure mode a glob buys here, and it is the one with no failing assertion
// to notice it.
var retentionSweepFiles = []string{
	"internal/modules/privacy/retention.go",
	"internal/modules/privacy/retentionai.go",
	"internal/modules/privacy/retention_graph.go",
	"internal/modules/privacy/retentionactions.go",
	// Everything an activity's TEXT leaves behind, which every arm destroys
	// through one function. It arrived here when that function was extracted,
	// and its absence was the regression this list's header describes twice
	// already: a delete leaving the named files makes the census report the
	// sweep as having stopped making it.
	//
	// retentionrestricted.go is deliberately NOT here. It holds the restriction
	// LIFT, which answers to a different contract — completing a suspended
	// erasure rather than applying a window — and destroys counterparty_email
	// where the retention action registers it as deliberately KEPT. Reading the
	// two through one registry reports the lift as a violation of a rule it was
	// never under.
	"internal/modules/privacy/activitycontenterasure.go",
}

// sqlStatements splits one Go string literal into the statements it holds. A
// literal is not a statement: several in this tree carry two, and a check that
// read the literal whole would let a second statement's SET clause hide behind
// the first one's.
//
// The split skips SEMICOLONS INSIDE STRINGS, which is the case gate-patterns.md
// §D names. `SET body = 'a;b', counterparty_email = NULL` splits naively into
// two fragments, and the second names no table — so sqlWriteTargets drops it and
// the destruction of a retained column in it goes unseen. Quoting is the only
// escape the split has to understand: a doubled ” inside a string is still
// inside it, which falls out of the scan without a case of its own.
//
// The scan is gatekit's, shared with the trigger-written-column census, because
// "what here is not SQL" is one question: a quoted value, a dollar-quoted body,
// a line comment and a block comment all carry semicolons that are not
// separators, and a split landing inside one leaves a fragment naming no table.
// Comments matter as much as quotes here — `SET body = NULL /* ; */ ,
// counterparty_email = NULL` splits at the comment's semicolon and the
// destruction rides out in a fragment sqlWriteTargets drops.
//
// ONE form it still does not understand, and does not guess at: the E” escape,
// where a backslashed apostrophe leaves the scan believing it has closed a
// string it has not. quotingBeyondTheSplit answers for it at the collection
// site, so its arrival is a failure naming the literal rather than a green run
// over a split nobody can trust — and it looks for it OUTSIDE the spans the
// scan does understand, or an E-string MENTIONED inside a value or a comment
// would refuse a statement that reads perfectly well.
func sqlStatements(literal string) []string {
	var out []string
	start := 0
	for i := 0; i < len(literal); i++ {
		if skip, _ := gatekit.SQLSpanAt(literal, i); skip > 0 {
			i += skip - 1
			continue
		}
		if literal[i] != ';' {
			continue
		}
		if trimmed := strings.TrimSpace(literal[start:i]); trimmed != "" {
			out = append(out, collapsedSQL(trimmed))
		}
		start = i + 1
	}
	if trimmed := strings.TrimSpace(literal[start:]); trimmed != "" {
		out = append(out, collapsedSQL(trimmed))
	}
	return out
}

// escapeStringRe is the one form the split cannot track: E'…' takes a
// BACKSLASH escape, so `E'a\';b'` leaves the scan believing it has closed a
// string it has not, and an inverted scan splits inside one.
var escapeStringRe = regexp.MustCompile(`(?i)\bE'`)

// quotingBeyondTheSplit names the form in a literal that sqlStatements would
// mis-scan, or "" when there is none. Nothing in the swept files uses it today;
// this is the tripwire for the day one does, and it fails CLOSED — the caller
// reports the literal — because the alternative is a split landing inside a
// string and a destructive assignment riding out in a fragment that names no
// table.
//
// Looked for OUTSIDE the spans the scan does understand, which is the half a
// bare regex gets wrong in the expensive direction: `SET body = 'E”s note'` or
// a comment mentioning one would refuse a statement that is read perfectly
// well, and a tripwire that fires on correct SQL is one somebody turns off.
func quotingBeyondTheSplit(literal string) string {
	for i := 0; i < len(literal); i++ {
		if skip, _ := gatekit.SQLSpanAt(literal, i); skip > 0 {
			i += skip - 1
			continue
		}
		if m := escapeStringRe.FindString(literal[i:]); m != "" && strings.HasPrefix(strings.ToLower(literal[i:]), "e'") {
			return m
		}
	}
	return ""
}

// setAssignments returns the assignment list of one UPDATE — what sits between
// SET and the clause that ends it — or "" when the statement has none.
//
// Scanned at PAREN DEPTH ZERO rather than matched with a lazy regex, and that
// is the whole point of it being a scan. `SET redacted_fields = ARRAY(SELECT c
// FROM unnest(…) WHERE c IS NOT NULL), counterparty_email = NULL` is a shape
// this tree already writes (retentionrestricted.go), and a regex ending at the
// first WHERE stops inside that subquery — so every assignment after it becomes
// invisible and reordering the list, which nobody reviews as a compliance
// change, turns the check off.
func setAssignments(statement string) string {
	low := strings.ToLower(statement)
	i := strings.Index(low, " set ")
	if i < 0 {
		return ""
	}
	rest := statement[i+len(" set "):]
	depth := 0
	for pos := range rest {
		switch rest[pos] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth != 0 {
			continue
		}
		for _, end := range []string{" where ", " from ", " returning "} {
			if strings.HasPrefix(strings.ToLower(rest[pos:]), end) {
				return rest[:pos]
			}
		}
	}
	return rest
}

// oneStatementCarries reports whether any single statement makes every declared
// assignment. Declared assignments describe ONE erasure, so they are proven
// against one statement — see the call site for why the union is not enough.
func oneStatementCarries(statements []string, assignments []string) bool {
	for _, stmt := range statements {
		all := true
		for _, assignment := range assignments {
			all = all && strings.Contains(stmt, strings.ToLower(assignment))
		}
		if all {
			return true
		}
	}
	return false
}

// sweepPurges reports whether the sweep deletes from table under every declared
// predicate — one delete per predicate, so a declaration names the acts rather
// than the table.
//
// Through deleteRe, the matcher tableownership_test.go already derives its own
// answer from, rather than a second spelling here. The hand-written one this
// replaced looked for "delete from <table> " and its end-of-string twin, and
// went blind on a trailing `;` or `)` — both of which end a statement in this
// tree, and both of which would have reported a purge that exists as missing.
func sweepPurges(statements []string, table string, predicates []string) bool {
	for _, predicate := range predicates {
		found := false
		for _, stmt := range statements {
			if !strings.Contains(stmt, strings.ToLower(predicate)) {
				continue
			}
			for _, m := range deleteRe.FindAllStringSubmatch(stmt, -1) {
				if m[1] == table {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// statementDestroying returns the sweep statement that would destroy column on
// table, or "" if none does.
//
// It asks whether the column is ASSIGNED AT ALL rather than whether it is
// assigned NULL: `col = NULL`, `col=NULL`, `col = CAST(NULL AS text)` and
// `col = ”` are one act in four spellings, and a check that recognised one
// would go quietly green on the other three. A retained column is one the sweep
// does not write, so any write to it is the finding, and a DELETE of the row
// counts because it takes the column with it.
//
// WHAT IT CANNOT SEE, and the tripwire that makes that loud. The column is
// matched as a LITERAL name, so a statement whose SET target is assembled —
// `"UPDATE activity SET " + column + " = NULL"` — carries no name to match and
// would pass silently. gate-patterns.md §D names that as the way a shape gate
// goes green. It cannot be read from a string literal at all, so it is not
// matched but REPORTED: assembledSetTarget below answers for a swept UPDATE
// whose SET clause names nothing, and the caller fails on it rather than
// reading it as a statement that destroys no column.
//
// Held by TestTheRetainedColumnCheckSeesEveryDestructiveShape, which plants the
// shapes an earlier version of this missed rather than trusting that the one
// statement in the tree passes.
func statementDestroying(statements []string, table, column string) string {
	needle := regexp.MustCompile(`(?is)(?:\bSET\b|,)\s*` + regexp.QuoteMeta(strings.ToLower(column)) + `\s*=`)
	for _, stmt := range statements {
		// The statement has to write THIS table. The caller already groups
		// statements by write target, but a helper that only answered
		// correctly because its caller filtered first is one the next caller
		// gets wrong — and `activity` and `activity_participant` both carry a
		// counterparty column, so the confusion is available here today.
		writesTable := false
		for _, target := range sqlWriteTargets(stmt) {
			writesTable = writesTable || target == table
		}
		if !writesTable {
			continue
		}
		if deleteRe.MatchString(stmt) {
			return stmt
		}
		if sets := setAssignments(stmt); sets != "" && needle.MatchString("SET "+strings.ToLower(sets)) {
			return stmt
		}
	}
	return ""
}

// assembledSetTarget answers the swept UPDATE whose SET clause names no column,
// or "" when every one of them does.
//
// A statement built by concatenation reaches a gate that reads string literals
// as its literal half alone — `UPDATE activity SET ` — and the column it writes
// is never in the text. statementDestroying would answer "destroys nothing",
// which is indistinguishable from a statement that really destroys nothing, and
// that is the shape gate-patterns.md §D says a shape check silently passes.
//
// A SET clause is unreadable in two shapes, and reading only the first was the
// same quiet pass one level in. `UPDATE provider_run SET` — which the sweep
// really does assemble today, in retentionactions.go, joined to
// storekit.ScrubProviderRunColumns — leaves NO assignments.
// `UPDATE activity SET body = NULL, ` + column + ` = NULL` leaves one, so a
// check asking only "is it empty" reads that as an ordinary statement and the
// assembled column goes unseen exactly as before. The trailing comma is the
// tell: an assignment list ending on a separator has an assignment after it
// that is not in the text.
//
// Both answers are read off the literal alone, which is why
// assembledSweepTargets below reads the concatenation instead: a fragment can
// also be a COMPLETE statement — `"UPDATE activity SET body = NULL" + more +
// " WHERE id = $1"` — and no amount of looking at that text says more follows.
//
// Neither reports provider_run, and that is the scoping and not an oversight.
// The caller asks these only of a table registering columns the sweep KEEPS,
// because the question they serve — "is the KEEPS registration still being
// checked against this statement" — has no meaning for a table with no such
// registration. provider_run declares none; its assembly is a named constant a
// reader can follow, not a column hidden from one.
func assembledSetTarget(statements []string) string {
	for _, stmt := range statements {
		if !updateRe.MatchString(stmt) {
			continue
		}
		assignments := strings.TrimSpace(setAssignments(stmt))
		if assignments == "" || strings.HasSuffix(assignments, ",") || formatVerbTargetRe.MatchString(assignments) {
			return stmt
		}
	}
	return ""
}

// formatVerbTargetRe is a fmt verb standing where a COLUMN should be — at the
// head of the assignment list or just after a comma, and followed by the `=`
// that makes it a target.
//
// `fmt.Sprintf("UPDATE activity SET %s = NULL", column)` reaches the reader as
// a literal that looks complete and names a column that is not a column, so
// neither the empty-clause nor the dangling-comma tell fires — and no `+` node
// exists for assembledSweepTargets to read either. The verb is the tell.
//
// The POSITION is what makes it a tell rather than a nuisance. A bare `%` is
// ordinary SQL text: `SET body = 'template %s'`, `SET query = 'foo%bar'`, a
// LIKE pattern `'100%'`. Matching those would report a statement that is
// entirely readable, and this gate's report is an instruction to go rewrite it
// — a tripwire that fires on correct code is one somebody turns off.
//
// What it still reads wrongly, stated rather than implied: a comma AND a verb
// AND an equals inside one quoted value — `SET body = 'a, %s = b'` — puts a
// target position inside a string. That is over-recognition on a shape nothing
// writes, which costs a false finding rather than a missed destruction.
var formatVerbTargetRe = regexp.MustCompile(`(?:^|,)\s*%[-+# 0-9.*]*[a-z]\s*=`)

// unreadableWriteOn answers the swept statement on one table that this gate
// can only half read, or "" when it can read them all.
//
// It is a function rather than two lines at the call site because the ANSWER is
// two answers and dropping either is silent. The text says a fragment is
// unfinished (assembledSetTarget); the syntax says a finished-looking fragment
// has more joined onto it (the caller's assembledSweepTargets). A regression
// that kept only the first would keep passing every case written for it.
func unreadableWriteOn(statements []string, assembledFragment string) string {
	if unreadable := assembledSetTarget(statements); unreadable != "" {
		return unreadable
	}
	return assembledFragment
}

// assembledSweepTargets returns, per table, a swept SQL literal that is joined
// to a runtime value — `"UPDATE provider_run SET" + storekit.Scrub…`. The table
// comes from the literal's own text, which is where the write target is even
// when the assignments are not.
//
// This reads the CONCATENATION, not the string, and that is the whole point.
// assembledSetTarget can only judge the text it is handed, and a fragment that
// reads as a finished statement is indistinguishable from one that is; the `+`
// node is the only place the fact that more follows is written down.
func assembledSweepTargets(t *testing.T, path string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	// Parentheses are not part of the expression, and reading them as if they
	// were fails in BOTH directions: a parenthesized literal on the joined side
	// hides the fragment, and one on the other side reads as a runtime value
	// and reports a statement that is entirely in the text.
	text := func(e ast.Expr) (string, bool) {
		for {
			paren, ok := e.(*ast.ParenExpr)
			if !ok {
				break
			}
			e = paren.X
		}
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		unquoted, err := strconv.Unquote(lit.Value)
		return unquoted, err == nil
	}
	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		// A SQL fragment handed to an ASSEMBLER — fmt.Sprintf, a
		// strings.Builder's WriteString, strings.Replace — is half a statement
		// with no `+` node to say so. tx.Exec's own literal is not: the whole
		// statement is the argument, which is why the call names are listed
		// rather than "any call".
		if call, isCall := n.(*ast.CallExpr); isCall && assemblesSQL(call) {
			// The whole SUBTREE, not the direct arguments: strings.Join takes
			// its pieces inside a slice literal, and a Sprintf format string
			// can itself be a join of literals.
			ast.Inspect(call, func(inner ast.Node) bool {
				expr, isExpr := inner.(ast.Expr)
				if !isExpr {
					return true
				}
				fragment, ok := text(expr)
				if !ok {
					return true
				}
				for _, table := range sqlWriteTargets(collapsedSQL(fragment)) {
					out[table] = collapsedSQL(fragment)
				}
				return true
			})
			return true
		}
		join, ok := n.(*ast.BinaryExpr)
		if !ok || join.Op != token.ADD {
			return true
		}
		// One side a literal and the other not. Two literals joined are still
		// one literal as far as the text is concerned — sqlLiterals reads both
		// halves — and reporting those would fire on every wrapped statement in
		// the tree.
		for _, side := range [2][2]ast.Expr{{join.X, join.Y}, {join.Y, join.X}} {
			fragment, ok := text(side[0])
			if !ok {
				continue
			}
			if _, alsoLiteral := text(side[1]); alsoLiteral {
				continue
			}
			for _, table := range sqlWriteTargets(collapsedSQL(fragment)) {
				out[table] = collapsedSQL(fragment)
			}
		}
		return true
	})
	return out
}

// sqlAssemblers are the calls that build a statement out of pieces. Named
// rather than inferred, because "a literal inside a call" is every statement in
// the tree: tx.Exec takes the whole thing.
var sqlAssemblers = map[string]bool{
	"Sprintf": true, "Sprint": true, "Sprintln": true,
	"Fprintf": true, "WriteString": true, "Replace": true, "ReplaceAll": true,
	"Join": true,
}

func assemblesSQL(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return sqlAssemblers[fn.Sel.Name]
	case *ast.Ident:
		return sqlAssemblers[fn.Name]
	}
	return false
}
