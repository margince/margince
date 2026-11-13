// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A shared mailbox is a capture_connection with no user, and its import rows
// name the connection instead of a seat. A statement that hands either user_id
// back to Go says what it does with such a row: `user_id IS NOT NULL` drops it,
// `user_id IS NULL` takes it. A NULL scanned into an id fails the whole read.
//
// A comparison of that column elsewhere in the statement drops the row too,
// and counts as the rule. The lists judged are those whose values reach Go:
// the statement's own, a `WITH` query's own, and an `ARRAY(SELECT ...)`.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	seatlessTables = regexp.MustCompile(`(?i)\b(capture_import|capture_connection)\b(?:\s+(?:AS\s+)?([a-z_]+))?`)
	sqlWord        = regexp.MustCompile(`(?i)\b(SELECT|FROM)\b|\(|\)`)
	valueOpen      = regexp.MustCompile(`(?i)(\bARRAY|\bAS(\s+(NOT\s+)?MATERIALIZED)?)\s*$`)
	distinctPrefix = regexp.MustCompile(`(?is)^\s*DISTINCT\s+(?:ON\s*\([^)]*\)\s*)?`)
	fromTable      = regexp.MustCompile(`^\s+([a-z_]+)`)
	selectedAlias  = regexp.MustCompile(`(?i)\bAS\s+\w+`)
	seatRule       = regexp.MustCompile(`(?i)user_id\s+IS\s+(NOT\s+)?NULL`)
	userIDRef      = regexp.MustCompile(`(?i)(?:\b([a-z_]+)\.)?\buser_id\b`)
)

// seatlessReaderFloor is how many statements the walk judged when this gate
// landed. Fewer means the walk stopped reaching them.
const seatlessReaderFloor = 28

func TestEveryReaderOfACaptureSeatSaysWhatASharedMailboxRowDoes(t *testing.T) {
	t.Parallel()
	judged := 0
	constants := map[string]map[string]string{}
	eachGoFileUnder(t, []string{"internal"}, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return
		}
		dir := filepath.Dir(path)
		if constants[dir] == nil {
			constants[dir] = gatekit.PackageStringConstants(t, dir)
		}
		// A statement is judged whole: a concatenation is folded with its
		// package's constants, and a part it cannot read stays a hole.
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.BasicLit:
				if typed.Kind != token.STRING {
					return true
				}
			case *ast.BinaryExpr:
				if typed.Op != token.ADD {
					return true
				}
			default:
				return true
			}
			sql, ok := gatekit.StringExpr(node, constants[dir], gatekit.FoldTotal)
			if !ok {
				return true
			}
			column, selects := selectedCaptureSeat(sql)
			if !selects {
				return false
			}
			judged++
			if !seatRule.MatchString(sql) && !comparesSeat(sql, column) {
				t.Errorf("%s:%d selects a capture seat with no rule for a shared mailbox row; add `user_id IS NOT NULL` (or handle `user_id IS NULL`)",
					path, gatekit.SourceFileSet().Position(node.Pos()).Line)
			}
			return false
		})
	})
	if judged < seatlessReaderFloor {
		t.Fatalf("judged %d statements, floor %d: the walk no longer reaches the readers", judged, seatlessReaderFloor)
	}
}

// selectedCaptureSeat answers the user_id column, as the statement spells it,
// that its outermost SELECT list returns from capture_import or
// capture_connection.
func selectedCaptureSeat(sql string) (string, bool) {
	aliases := map[string]bool{}
	for _, m := range seatlessTables.FindAllStringSubmatch(sql, -1) {
		aliases[strings.ToLower(m[1])] = true
		if m[2] != "" && !sqlKeyword(m[2]) {
			aliases[strings.ToLower(m[2])] = true
		}
	}
	if len(aliases) == 0 {
		return "", false
	}
	for _, list := range valueSelectLists(sql) {
		columns := selectedAlias.ReplaceAllString(distinctPrefix.ReplaceAllString(list.columns, ""), "")
		for _, ref := range userIDRef.FindAllStringSubmatch(columns, -1) {
			if (ref[1] == "" && aliases[list.table]) || (ref[1] != "" && aliases[strings.ToLower(ref[1])]) {
				return ref[0], true
			}
		}
	}
	return "", false
}

// valueList is one select list's columns and the first table its from names.
type valueList struct{ columns, table string }

// valueSelectLists answers the select lists at depth 0 and those opening a
// `WITH` query or an `ARRAY(` subquery. Each pairs with the from at its depth.
func valueSelectLists(sql string) []valueList {
	type open struct {
		at    int
		depth int
	}
	var pending []open
	var lists []valueList
	depth := 0
	for _, loc := range sqlWord.FindAllStringIndex(sql, -1) {
		word := strings.ToUpper(sql[loc[0]:loc[1]])
		switch word {
		case "(":
			depth++
		case ")":
			depth--
		case "SELECT":
			if depth == 0 || valueOpen.MatchString(strings.TrimRight(sql[:loc[0]], " \t\n(")) {
				pending = append(pending, open{at: loc[1], depth: depth})
			}
		case "FROM":
			for i, candidate := range slices.Backward(pending) {
				if candidate.depth != depth {
					continue
				}
				table := ""
				if m := fromTable.FindStringSubmatch(sql[loc[1]:]); m != nil {
					table = strings.ToLower(m[1])
				}
				lists = append(lists, valueList{columns: sql[candidate.at:loc[0]], table: table})
				pending = append(pending[:i], pending[i+1:]...)
				break
			}
		}
	}
	return lists
}

// comparesSeat reports whether the statement compares column, cast or not.
// Each of `=`, `<>`, `IN` and `= ANY` is false for a NULL.
func comparesSeat(sql, column string) bool {
	// A bare column must not match another alias's column of the same name.
	quoted := `(?:^|[^.\w])` + regexp.QuoteMeta(column)
	if strings.Contains(column, ".") {
		quoted = `\b` + regexp.QuoteMeta(column)
	}
	compared := regexp.MustCompile(`(?i)(` + quoted + `(::\w+)?\s*(=|<>|!=|\bIN\b)|(=|<>|!=)\s*` + quoted + `\b)`)
	return compared.MatchString(sql)
}

func sqlKeyword(word string) bool {
	switch strings.ToUpper(word) {
	case "WHERE", "JOIN", "ON", "LEFT", "INNER", "USING", "SET", "VALUES", "GROUP", "ORDER", "LIMIT", "FOR", "UNION", "AND", "OR":
		return true
	}
	return false
}
