// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"testing"
)

func TestUnknownAdmissionActionsCannotDisappearFromWriteAuthority(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		action   string
		mutating bool
	}{
		{"principal.ActionRead", false},
		{"principal.ActionCreate", false},
		{"principal.ActionUpdate", true},
		{"principal.ActionDelete", true},
		{"action", true},
		{"grant.Action", true},
	} {
		expression, err := parser.ParseExpr(`auth.Require(ctx,"contact",` + scenario.action + `)`)
		if err != nil {
			t.Fatal(err)
		}
		call, ok := expression.(*ast.CallExpr)
		if !ok {
			t.Fatal("the fixture is not a call")
		}
		info := &writeAuthorityFn{requires: map[string]bool{}}
		recordAuthCall("Require", call, info, map[string]bool{"contact": true}, map[string]string{}, probeSite{}, tierFile{})
		if info.requires["contact"] != scenario.mutating {
			t.Errorf("%s write admission=%v", scenario.action, info.requires["contact"])
		}
	}
}
