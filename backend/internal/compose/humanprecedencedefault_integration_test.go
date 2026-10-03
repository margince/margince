// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// Only a quoted text or enum literal is a default nobody chose. A number or a
// boolean is read as a value a human may have meant — product.active = true is
// a decision to sell it — so the tie goes to asking them.
func TestOnlyAQuotedLiteralDefaultIsNobodysEdit(t *testing.T) {
	e := integration.Setup(t)
	for _, tc := range []struct {
		expr string
		want *string
	}{
		{`'unknown'::text`, ptrTo("unknown")},
		{`'unknown'::company_lifecycle`, ptrTo("unknown")},
		{`'unknown'::public.company_lifecycle`, ptrTo("unknown")},
		{`'it''s'::character varying`, ptrTo("it's")},
		{`true`, nil},
		{`false`, nil},
		{`0`, nil},
		{`'0'::numeric`, nil},
		{`'0'::bigint`, nil},
		{`'f'::boolean`, nil},
		{`now()`, nil},
		{`'{}'::text[]`, nil},
	} {
		var got *string
		if err := e.Pool.QueryRow(context.Background(),
			`SELECT `+literalOfDefault+` FROM (SELECT $1::text AS expr) d`, tc.expr).Scan(&got); err != nil {
			t.Fatalf("%s: %v", tc.expr, err)
		}
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("default %s reads as %q (absent: %v), want %q (absent: %v)",
				tc.expr, deref(got), got == nil, deref(tc.want), tc.want == nil)
		}
	}
}
