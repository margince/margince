// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The lock-order walk judged on planted bodies. A child row taken before the
// company hold is found however the code is arranged. A body that holds first
// passes however its writes are arranged.
func TestTheLockOrderWalkJudgesWhenCodeRunsNotWhereItIsWritten(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		body     string
		inverted bool
	}{
		{"a child write above the hold", `
			tx.Exec(ctx, "UPDATE company_fact SET value = $1")
			auth.HoldWritableLive(ctx, tx, "company", id)`, true},
		{"the hold above the child write", `
			auth.HoldWritableLive(ctx, tx, "company", id)
			tx.Exec(ctx, "DELETE FROM company_fact WHERE id = $1")`, false},
		{"a hold written early in a closure that runs after the write", `
			hold := func() error { return auth.HoldWritableLive(ctx, tx, "company", id) }
			tx.Exec(ctx, "INSERT INTO company_fact (id) VALUES ($1)")
			hold()`, true},
		{"a write written early in a closure that runs after the hold", `
			write := func() { tx.Exec(ctx, "UPDATE company_fact SET value = $1") }
			auth.HoldWritableLive(ctx, tx, "company", id)
			write()`, false},
		{"a hold deferred to the end of a body that writes", `
			defer auth.HoldWritableLive(ctx, tx, "company", id)
			tx.Exec(ctx, "UPDATE company_fact SET value = $1")`, true},
		{"a closure that writes and then holds", `
			work := func() {
				tx.Exec(ctx, "UPDATE company_fact SET value = $1")
				auth.HoldWritableLive(ctx, tx, "company", id)
			}
			work()`, true},
		{"a closure that holds and then writes", `
			work := func() {
				auth.HoldWritableLive(ctx, tx, "company", id)
				tx.Exec(ctx, "UPDATE company_fact SET value = $1")
			}
			work()`, false},
		{"an inner closure that shadows a holding one with a write", `
			work := func() { auth.HoldWritableLive(ctx, tx, "company", id) }
			{
				work := func() { tx.Exec(ctx, "UPDATE company_fact SET value = $1") }
				work()
			}
			work()`, true},
		{"a callback kept in a struct for the holding callee to run", `
			w := writer{locate: func() { tx.Exec(ctx, "UPDATE company_fact SET value = $1") }}
			auth.HoldWritableLive(ctx, tx, "company", id)
			tx.Exec(ctx, "UPDATE company_fact SET value = $1")
			w.canonical = func() { tx.Exec(ctx, "UPDATE company SET name = $1") }
			write(w)`, false},
		{"a transaction callback that holds before it writes", `
			s.tx(ctx, func(tx pgx.Tx) error {
				auth.HoldWritableLive(ctx, tx, "company", id)
				return tx.Exec(ctx, "UPDATE company_fact SET value = $1")
			})`, false},
	}
	never := func(string) bool { return false }
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "planted.go", "package planted\nfunc planted() {"+c.body+"\n}", 0)
			if err != nil {
				t.Fatal(err)
			}
			fn, ok := file.Decls[0].(*ast.FuncDecl)
			if !ok {
				t.Fatal("the planted source holds no function")
			}
			hold, lock, lockFirst := holdAndFirstLock(fn, "", "", never, never)
			if hold == token.NoPos || lock == token.NoPos {
				t.Fatalf("the walk found hold=%v lock=%v, want both", hold, lock)
			}
			if lockFirst != c.inverted {
				t.Fatalf("the walk read the order as inverted=%t, want %t", lockFirst, c.inverted)
			}
		})
	}
}
