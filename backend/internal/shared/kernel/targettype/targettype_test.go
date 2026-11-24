// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package targettype

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

func TestKnownIsTheDeclaredConstants(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "targettype.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if typ, ok := spec.Type.(*ast.Ident); !ok || typ.Name != "Type" {
			return true
		}
		for _, v := range spec.Values {
			value, err := strconv.Unquote(v.(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			declared[value] = true
		}
		return true
	})
	if len(declared) != len(known) {
		t.Errorf("Type declares %d values and known holds %d", len(declared), len(known))
	}
	for value := range declared {
		if _, ok := known[value]; !ok {
			t.Errorf("Type declares %q and known does not", value)
		}
	}
}

func TestKnownAcceptsTheListAndAUnitTableOnly(t *testing.T) {
	t.Parallel()
	for _, accepted := range []string{"deal", "stage", "ext_visit_log_visit"} {
		if !Known(accepted) {
			t.Errorf("%q was refused", accepted)
		}
	}
	for _, refused := range []string{"", "widget", "Deal", "ext:visit-log:visit", "ext_Bad"} {
		if Known(refused) {
			t.Errorf("%q was accepted", refused)
		}
	}
}
