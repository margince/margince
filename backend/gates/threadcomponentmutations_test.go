// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestThreadWriterCensusRejectsPlantedOrderingViolations(t *testing.T) {
	t.Parallel()
	const write = `tx.Exec(ctx, "INSERT INTO activity (thread_key) VALUES (@key)")`
	const lock = `storekit.LockThreadKeysTx(ctx, tx, key)`
	cases := []struct {
		name, code string
		guarded    bool
	}{
		{"wait before write", `func writer(){ storekit.WaitThreadKeysTx(ctx,tx,keys); ` + write + ` }`, true},
		{"lock before write", `func writer(){ ` + lock + `; ` + write + ` }`, true},
		{"lock after write", `func writer(){ ` + write + `; ` + lock + ` }`, false},
		{"unused closure", `func writer(){ unused:=func(){ ` + lock + ` }; ` + write + ` }`, false},
		{"helper before write", `func writer(){ acquire(); ` + write + ` }; func acquire(){ ` + lock + ` }`, true},
		{"helper after write", `func writer(){ ` + write + `; acquire() }; func acquire(){ ` + lock + ` }`, false},
		{"parent before writer", `func writer(){ ` + write + ` }; func entry(){ ` + lock + `; writer() }`, true},
		{"parent after writer", `func writer(){ ` + write + ` }; func entry(){ writer(); ` + lock + ` }`, false},
		{"unguarded sibling caller", `func writer(){ ` + write + ` }; func safe(){ ` + lock + `; writer() }; func unsafe(){ writer() }`, false},
		{"cycle without lock", `func writer(){ helper(); ` + write + ` }; func helper(){ writer() }`, false},
		{"cycle before late lock", `func writer(){ helper(); ` + write + `; ` + lock + ` }; func helper(){ writer() }`, false},
		{"public entry cannot borrow caller lock", `func writer(){ Public() }; func Public(){ ` + write + ` }; func caller(){ ` + lock + `; Public() }`, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source := "package fixture\nimport storekit \"github.com/margince/margince/backend/internal/platform/database/storekit\"\n" + test.code
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			order := threadOrderOf([]*ast.File{file})
			var position token.Pos
			name := "writer"
			if strings.Contains(test.code, "func Public") {
				name = "Public"
			}
			ast.Inspect(order.functions[name].Body, func(n ast.Node) bool {
				literal, ok := n.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				sql, err := strconv.Unquote(literal.Value)
				if err == nil && strings.HasPrefix(sql, "INSERT INTO activity") {
					position = literal.Pos()
				}
				return true
			})
			if position == token.NoPos {
				t.Fatal("planted writer was not discovered")
			}
			if got := order.guardedBefore(name, position, map[string]bool{}); got != test.guarded {
				t.Fatalf("guarded=%v, want %v", got, test.guarded)
			}
		})
	}
}
