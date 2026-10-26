// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H2

package gates

// A LIKE whose pattern carries a caller's text escapes the wildcards in it.
//
// `%` and `_` are pattern syntax, not characters.
//
// A search for "50%" with an unescaped operand matches every row whose column
// starts "50". Nobody sees an error: the query succeeds and answers about rows
// the caller never asked for.
//
// In an erasure or an exclusion predicate that is a row destroyed or withheld
// on a match nobody intended.
//
// storekit.EscapeLike neutralizes a value on the Go side, and
// storekit.EscapeLikeSQL does it to an expression inside the statement.
//
// One unit test covered the shared quick-find clause and nothing held the
// rest, so a new LIKE with a bound operand joined the tree unescaped.
//
// A spliced column is judged per statement and a bound parameter per file.
//
// A function-scoped census would read a package-level query as unescaped and
// the function that binds its arguments as escaping nothing.

import (
	"go/ast"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// A runtime LIKE operand comes in two shapes, and each can only be escaped in
// one place.
//
// A COLUMN spliced into the pattern inside the SQL can only be escaped there,
// by storekit.EscapeLikeSQL.
//
// gatekit renders a spliced Go call as a gap. An escaped operand is therefore
// absent from the statement text, while an unescaped column is spelled out.
//
// That makes this shape judgeable per statement, which catches a second
// unescaped LIKE in a file that already escapes one.
//
// A BOUND parameter carries whatever the caller bound, so only the Go side can
// escape it.
//
// The statement cannot say whether anybody did, and a query is often a
// package-level string whose arguments are bound elsewhere in the same file.
// So the file is the smallest unit that can answer for that shape.
var (
	// anyRuntimeLike is the broad reader, used by the corpus floor: every LIKE
	// whose pattern is assembled at runtime, escaped or not.
	anyRuntimeLike = regexp.MustCompile(`(?i)\bI?LIKE\s+(\$[0-9]+|'[^']*'\s*\|\||[a-z_][a-z0-9_.]*\s*\|\|)`)

	// Same-line only, and spaces rather than \s.
	//
	// A gap left by a spliced call is followed by whatever the next line of SQL
	// begins with. Crossing a newline read that keyword as the operand.
	splicedColumnAfterLiteral = regexp.MustCompile(`(?i)\bI?LIKE[ \t]*\(?[ \t]*'[^']*'[ \t]*\|\|[ \t]*\(?[ \t]*([a-z_][a-z0-9_.]*)`)
	splicedColumnBeforeSuffix = regexp.MustCompile(`(?i)\bI?LIKE[ \t]*\(?[ \t]*([a-z_][a-z0-9_.]*)[ \t]*\)?[ \t]*\|\|`)
	boundOperand              = regexp.MustCompile(`(?i)\bI?LIKE\s+(?:'[^']*'\s*\|\|\s*)?\$[0-9]+`)
)

// sqlKeywordsAfterAGap are the words that follow a spliced call often enough
// to be mistaken for its operand.
//
// The gap renders as whitespace, so the reader would otherwise take the next
// word in the statement as a column.
var sqlKeywordsAfterAGap = map[string]bool{
	"escape": true, "or": true, "and": true, "then": true, "else": true,
	"end": true, "when": true, "order": true, "group": true, "limit": true,
	"union": true, "select": true, "from": true, "where": true, "as": true,
}

// splicesAnUnescapedColumn reports a statement that concatenates a column into
// a LIKE pattern without the in-statement escape.
func splicesAnUnescapedColumn(stmt string) bool {
	for _, re := range []*regexp.Regexp{splicedColumnAfterLiteral, splicedColumnBeforeSuffix} {
		for _, m := range re.FindAllStringSubmatch(stmt, -1) {
			if !sqlKeywordsAfterAGap[strings.ToLower(m[1])] {
				return true
			}
		}
	}
	return false
}

// bindsALikeOperand reports a statement whose LIKE pattern carries a bound
// parameter, which only the Go side can have escaped.
func bindsALikeOperand(stmt string) bool { return boundOperand.MatchString(stmt) }

// likeEscapeWaived are the files whose runtime LIKE operand cannot carry a
// wildcard, each with the reason. Keyed by path so the entry names the file a
// reader opens.
//
// An entry without a rationale is a finding rather than a pass, the same bar
// the other registers in this package hold.
var likeEscapeWaived = gatekit.Waive(map[string]string{
	"internal/platform/auth/audienceorphan.go": "the operand is `u.id`, a uuid column: " +
		"a uuid has no wildcard to escape and the pattern around it is a literal prefix.",
	"internal/compose/introadvance.go": "the operand is a captured_by prefix the server stamps " +
		"(`human:`, `connector:`), built from an actor id rather than from anything a caller typed.",
	"internal/modules/capture/seatstanding.go":         "a server-stamped captured_by prefix, as introadvance.",
	"internal/modules/activities/lasttouch.go":         "a server-stamped captured_by prefix, as introadvance.",
	"internal/modules/activities/followupownership.go": "a server-stamped captured_by prefix, as introadvance.",
	"internal/modules/activities/emailpresentation.go": "the operand is a uuid argument cast to text " +
		"inside a literal `%:` prefix.",
})

func TestEveryRuntimeLikeOperandIsEscaped(t *testing.T) {
	t.Parallel()
	var unescaped []string
	eachGoFileInTheModule(t, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return
		}
		stmts := gatekit.SQLStatementsOf(file)
		spliced := slices.ContainsFunc(stmts, splicesAnUnescapedColumn)
		bound := slices.ContainsFunc(stmts, bindsALikeOperand)
		if !spliced && !bound {
			return
		}
		// A waiver is read first, because each reason in the register names the
		// operand rather than the file.
		//
		// Two of them are a uuid column spliced into a literal prefix, which
		// carries no wildcard to escape.
		if likeEscapeWaived.Waived(t, path) {
			return
		}
		// A spliced column is otherwise judged on its own, because its escape
		// would be in the statement. Another escaped LIKE in the file says
		// nothing about this one.
		if spliced {
			unescaped = append(unescaped, path)
			return
		}
		if !escapesSomewhere(file) {
			unescaped = append(unescaped, path)
		}
	})
	for _, path := range unescaped {
		t.Errorf("%s builds a LIKE pattern at runtime and never reaches storekit.EscapeLike "+
			"or storekit.EscapeLikeSQL. An unescaped `%%` or `_` in the operand is pattern "+
			"syntax, so the query answers about rows the caller did not ask for and reports "+
			"no error. Escape the operand, or waive this file in likeEscapeWaived with the "+
			"reason its operand cannot carry a wildcard", path)
	}
	likeEscapeWaived.AssertAllMatched(t)
}

// escapesSomewhere reports whether the file calls either escape helper.
//
// Both spellings count: storekit's own files call them unqualified, and a
// selector-only reader named every one of them as a finding.
func escapesSomewhere(file *ast.File) bool {
	escaped := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !escaped
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			escaped = escaped || isEscapeHelper(fn.Sel.Name)
		case *ast.Ident:
			escaped = escaped || isEscapeHelper(fn.Name)
		}
		return !escaped
	})
	return escaped
}

func isEscapeHelper(name string) bool {
	return name == "EscapeLike" || name == "EscapeLikeSQL"
}

// fewestRuntimeLikeFiles is the corpus floor.
//
// The census is only as good as the files it reaches.
//
// Under-recognition is the one way it fails without saying so: a narrowed regex
// or a walk that stops short reports PASS over a smaller tree.
const fewestRuntimeLikeFiles = 16

func TestTheLikeCensusStillReachesTheTree(t *testing.T) {
	t.Parallel()
	reached := 0
	eachGoFileInTheModule(t, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return
		}
		if slices.ContainsFunc(gatekit.SQLStatementsOf(file), anyRuntimeLike.MatchString) {
			reached++
		}
	})
	if reached < fewestRuntimeLikeFiles {
		t.Errorf("the census reaches %d file(s) with a runtime LIKE operand and the floor is %d. "+
			"Fewer means the reader stopped seeing sites rather than that the tree stopped "+
			"having them: lower the floor only alongside the change that removed them", reached, fewestRuntimeLikeFiles)
	}
}
