// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit_test

// The reader, from both ends. Every row here is a case one of the two readers
// this replaced could see and the other could not — which is the defect: the
// blast radius of a census was decided by which file its author had read most
// recently, and picking the narrower reader reports a clean tree rather than
// failing.

import (
	"go/parser"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheStringReaderAnswersBothQuestionsItIsAsked(t *testing.T) {
	t.Parallel()
	consts := map[string]string{"known": "value"}
	cases := []struct {
		name       string
		expr       string
		strictText string
		strictIs   bool
		totalText  string
		totalIs    bool
	}{
		{
			name: "a plain literal", expr: `"gmail.com"`,
			strictText: "gmail.com", strictIs: true,
			totalText: "gmail.com", totalIs: true,
		},
		{
			// The escapes are the point: the reader hands back what the program
			// holds, not what the author typed.
			name: "an interpreted literal", expr: `"a\nb"`,
			strictText: "a\nb", strictIs: true,
			totalText: "a\nb", totalIs: true,
		},
		{
			name: "a resolved identifier", expr: `known`,
			strictText: "value", strictIs: true,
			totalText: "value", totalIs: true,
		},
		{
			// Strict says "not a string" so its census never judges invented
			// text; total says "a hole" so its census keeps descending.
			name: "an unresolvable identifier", expr: `whatever`,
			strictText: "", strictIs: false,
			totalText: gatekit.ComputedFragment, totalIs: false,
		},
		{
			name: "a parenthesised literal", expr: `("x")`,
			strictText: "x", strictIs: true,
			totalText: "x", totalIs: true,
		},
		{
			// The row the total reader used to miss entirely: it had no call
			// arm, so a constant conversion fell through as unreadable.
			name: "a string conversion", expr: `string("v1")`,
			strictText: "v1", strictIs: true,
			totalText: "v1", totalIs: true,
		},
		{
			name: "a call that is not a conversion", expr: `strings.ToLower("x")`,
			strictText: "", strictIs: false,
			totalText: gatekit.ComputedFragment, totalIs: false,
		},
		{
			name: "a concatenation of literals", expr: `"a" + "b"`,
			strictText: "ab", strictIs: true,
			totalText: "ab", totalIs: true,
		},
		{
			// The row the strict reader misses: half a statement is still the
			// half a census's words are in.
			name: "a concatenation with an unreadable half", expr: `"SELECT " + column`,
			strictText: "", strictIs: false,
			totalText: "SELECT " + gatekit.ComputedFragment, totalIs: true,
		},
		{
			name: "a non-string literal", expr: `42`,
			strictText: "", strictIs: false,
			totalText: "", totalIs: false,
		},
		{
			name: "an expression that is not a string at all", expr: `x - y`,
			strictText: "", strictIs: false,
			totalText: "", totalIs: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parsing %s: %v", tc.expr, err)
			}
			text, is := gatekit.StringExpr(expr, consts, gatekit.FoldStrict)
			if text != tc.strictText || is != tc.strictIs {
				t.Errorf("strict read %q as (%q, %t), want (%q, %t)",
					tc.expr, text, is, tc.strictText, tc.strictIs)
			}
			text, is = gatekit.StringExpr(expr, consts, gatekit.FoldTotal)
			if text != tc.totalText || is != tc.totalIs {
				t.Errorf("total read %q as (%q, %t), want (%q, %t)",
					tc.expr, text, is, tc.totalText, tc.totalIs)
			}
		})
	}
}
