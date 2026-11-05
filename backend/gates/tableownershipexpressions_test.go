// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Complete concatenations keep one statement identity. Unresolved expressions
// retain the literal scan so an unknown fragment cannot hide a known write.
func ownershipSQLExpression(node ast.Node, constants map[string]string, consumed map[ast.Node]bool) (string, bool) {
	if consumed[node] {
		return "", false
	}
	switch node.(type) {
	case *ast.BasicLit, *ast.BinaryExpr, *ast.ParenExpr:
	default:
		return "", false
	}
	text, ok := gatekit.StringExpr(node, constants, gatekit.FoldStrict)
	if !ok || len(sqlWrites(text)) == 0 {
		return "", false
	}
	ast.Inspect(node, func(child ast.Node) bool {
		if child != node {
			consumed[child] = true
		}
		return true
	})
	return text, true
}

func TestOwnershipReadsCompleteExpressionsWithoutLosingUnresolvedWrites(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, expr         string
		constants          map[string]string
		statements, tables []string
	}{
		{"split keyword", "`UP`+`DATE rival SET active = true`", nil, []string{"UPDATE rival SET active = true"}, []string{"rival"}},
		{"shared predicate", "`DELETE FROM rival WHERE `+scope", map[string]string{"scope": "owner_id = @id"}, []string{"DELETE FROM rival WHERE owner_id = @id"}, []string{"rival"}},
		{"cte spans fragments", "`WITH gone AS (DELETE FROM rival WHERE `+scope+` RETURNING id) INSERT INTO history SELECT id FROM gone`", map[string]string{"scope": "owner_id = @id"}, []string{"WITH gone AS (DELETE FROM rival WHERE owner_id = @id RETURNING id) INSERT INTO history SELECT id FROM gone"}, []string{"history", "rival"}},
		{"unknown fragment retains both writes", "`DELETE FROM rival WHERE `+unknown()+`; INSERT INTO history(id) VALUES (1)`", nil, []string{"DELETE FROM rival WHERE ", "; INSERT INTO history(id) VALUES (1)"}, []string{"rival", "history"}},
		{"nested joins count once", "(`UPDATE rival `+(`SET active = true`))", nil, []string{"UPDATE rival SET active = true"}, []string{"rival"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatal(err)
			}
			consumed := map[ast.Node]bool{}
			var statements, tables []string
			ast.Inspect(expr, func(node ast.Node) bool {
				if text, ok := ownershipSQLExpression(node, tc.constants, consumed); ok {
					statements = append(statements, text)
					for _, target := range sqlWrites(text) {
						tables = append(tables, target.table)
					}
				}
				return true
			})
			if !slices.Equal(statements, tc.statements) || !slices.Equal(tables, tc.tables) {
				t.Fatalf("statements=%q tables=%v", statements, tables)
			}
		})
	}
}

func TestOwnershipKeepsSeparateCopiesOfTheSameComposedStatement(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func write() { exec(`DELETE FROM rival WHERE `+scope); exec(`DELETE FROM rival WHERE `+scope) }", 0)
	if err != nil {
		t.Fatal(err)
	}
	consumed := map[ast.Node]bool{}
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if _, ok := ownershipSQLExpression(node, map[string]string{"scope": "owner_id = @id"}, consumed); ok {
			count++
		}
		return true
	})
	if count != 2 {
		t.Fatalf("found %d writes, want both copies", count)
	}
}
